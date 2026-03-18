package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type imageConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newImageConnectService(store *db.Store, authenticator Authenticator) *imageConnectService {
	return &imageConnectService{store: store, authenticator: authenticator}
}

func (s *imageConnectService) ListCustomImages(ctx context.Context, req *connect.Request[arcav1.ListCustomImagesRequest]) (*connect.Response[arcav1.ListCustomImagesResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	images, err := s.store.ListCustomImages(ctx)
	if err != nil {
		log.Printf("list custom images failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list custom images"))
	}

	items := make([]*arcav1.CustomImage, 0, len(images))
	for _, img := range images {
		runtimeIDs, _ := s.store.ListTemplateIDsByCustomImageID(ctx, img.ID)
		items = append(items, toCustomImageMessage(img, runtimeIDs))
	}

	return connect.NewResponse(&arcav1.ListCustomImagesResponse{Images: items}), nil
}

func (s *imageConnectService) CreateCustomImage(ctx context.Context, req *connect.Request[arcav1.CreateCustomImageRequest]) (*connect.Response[arcav1.CreateCustomImageResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	runtimeType := strings.ToLower(strings.TrimSpace(req.Msg.GetTemplateType()))
	if runtimeType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_type is required"))
	}
	if runtimeType != "gce" && runtimeType != "lxd" && runtimeType != "libvirt" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_type must be gce, lxd, or libvirt"))
	}

	data := req.Msg.GetData()
	if err := validateCustomImageData(runtimeType, data); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encode data"))
	}

	img, err := s.store.CreateCustomImage(ctx, name, runtimeType, string(dataJSON), req.Msg.GetDescription())
	if err != nil {
		if errors.Is(err, db.ErrCustomImageNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("image with this name and runtime type already exists"))
		}
		log.Printf("create custom image failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create custom image"))
	}

	// Associate runtimes
	runtimeIDs := req.Msg.GetTemplateIds()
	for _, rid := range runtimeIDs {
		rid = strings.TrimSpace(rid)
		if rid == "" {
			continue
		}
		if err := s.validateTemplateTypeMatch(ctx, rid, runtimeType); err != nil {
			return nil, err
		}
		if err := s.store.AssociateTemplateCustomImage(ctx, rid, img.ID); err != nil {
			log.Printf("associate runtime custom image failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to associate runtime"))
		}
	}

	associatedIDs, _ := s.store.ListTemplateIDsByCustomImageID(ctx, img.ID)

	writeAuditLog(ctx, s.store, adminUserID, "", "image.create", "custom_image", img.ID, fmt.Sprintf(`{"name":%q}`, name))

	return connect.NewResponse(&arcav1.CreateCustomImageResponse{Image: toCustomImageMessage(img, associatedIDs)}), nil
}

func (s *imageConnectService) UpdateCustomImage(ctx context.Context, req *connect.Request[arcav1.UpdateCustomImageRequest]) (*connect.Response[arcav1.UpdateCustomImageResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	runtimeType := strings.ToLower(strings.TrimSpace(req.Msg.GetTemplateType()))
	if runtimeType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_type is required"))
	}
	if runtimeType != "gce" && runtimeType != "lxd" && runtimeType != "libvirt" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_type must be gce, lxd, or libvirt"))
	}

	data := req.Msg.GetData()
	if err := validateCustomImageData(runtimeType, data); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encode data"))
	}

	img, updated, err := s.store.UpdateCustomImage(ctx, id, name, runtimeType, string(dataJSON), req.Msg.GetDescription())
	if err != nil {
		if errors.Is(err, db.ErrCustomImageNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("image with this name and runtime type already exists"))
		}
		log.Printf("update custom image failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update custom image"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
	}

	// Re-sync runtime associations
	if err := s.store.DisassociateAllTemplatesFromCustomImage(ctx, id); err != nil {
		log.Printf("disassociate runtimes failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update runtime associations"))
	}
	for _, rid := range req.Msg.GetTemplateIds() {
		rid = strings.TrimSpace(rid)
		if rid == "" {
			continue
		}
		if err := s.validateTemplateTypeMatch(ctx, rid, runtimeType); err != nil {
			return nil, err
		}
		if err := s.store.AssociateTemplateCustomImage(ctx, rid, id); err != nil {
			log.Printf("associate runtime custom image failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to associate runtime"))
		}
	}

	associatedIDs, _ := s.store.ListTemplateIDsByCustomImageID(ctx, id)

	writeAuditLog(ctx, s.store, adminUserID, "", "image.update", "custom_image", id, fmt.Sprintf(`{"name":%q}`, name))

	return connect.NewResponse(&arcav1.UpdateCustomImageResponse{Image: toCustomImageMessage(img, associatedIDs)}), nil
}

func (s *imageConnectService) DeleteCustomImage(ctx context.Context, req *connect.Request[arcav1.DeleteCustomImageRequest]) (*connect.Response[arcav1.DeleteCustomImageResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	// Fetch name before deletion for audit log
	var imageName string
	if img, imgErr := s.store.GetCustomImage(ctx, id); imgErr == nil {
		imageName = img.Name
	}

	deleted, err := s.store.DeleteCustomImage(ctx, id)
	if err != nil {
		log.Printf("delete custom image failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete custom image"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "image.delete", "custom_image", id, fmt.Sprintf(`{"name":%q}`, imageName))

	return connect.NewResponse(&arcav1.DeleteCustomImageResponse{}), nil
}

func (s *imageConnectService) ListAvailableImages(ctx context.Context, req *connect.Request[arcav1.ListAvailableImagesRequest]) (*connect.Response[arcav1.ListAvailableImagesResponse], error) {
	if _, err := s.authenticate(ctx, req.Header()); err != nil {
		return nil, err
	}

	runtimeID := strings.TrimSpace(req.Msg.GetTemplateId())
	if runtimeID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_id is required"))
	}

	images, err := s.store.ListCustomImagesByTemplateID(ctx, runtimeID)
	if err != nil {
		log.Printf("list available images failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list available images"))
	}

	items := make([]*arcav1.CustomImage, 0, len(images))
	for _, img := range images {
		items = append(items, toCustomImageMessage(img, nil))
	}

	return connect.NewResponse(&arcav1.ListAvailableImagesResponse{Images: items}), nil
}

func (s *imageConnectService) authenticate(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("image service unavailable"))
	}

	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		userID, _, _, err := s.authenticator.Authenticate(ctx, sessionToken)
		if err == nil {
			return userID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, _, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

func (s *imageConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("image management unavailable"))
	}

	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		userID, _, role, err := s.authenticator.Authenticate(ctx, sessionToken)
		if err == nil {
			if role != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage images"))
			}
			return userID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, role, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			if role != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage images"))
			}
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

func (s *imageConnectService) validateTemplateTypeMatch(ctx context.Context, templateID, expectedType string) error {
	tmpl, err := s.store.GetMachineTemplateByID(ctx, templateID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("template not found: "+templateID))
		}
		return connect.NewError(connect.CodeInternal, errors.New("failed to resolve template"))
	}
	if strings.ToLower(tmpl.Type) != expectedType {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("template type mismatch: template "+templateID+" is "+tmpl.Type+", expected "+expectedType))
	}
	return nil
}

func toCustomImageMessage(img db.CustomImage, templateIDs []string) *arcav1.CustomImage {
	data := make(map[string]string)
	if img.DataJSON != "" && img.DataJSON != "{}" {
		_ = json.Unmarshal([]byte(img.DataJSON), &data)
	}
	return &arcav1.CustomImage{
		Id:                    img.ID,
		Name:                  img.Name,
		TemplateType:          img.TemplateType,
		Data:                  data,
		Description:           img.Description,
		AssociatedTemplateIds: templateIDs,
		CreatedAt:             img.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func validateCustomImageData(runtimeType string, data map[string]string) error {
	switch runtimeType {
	case "gce":
		if strings.TrimSpace(data["image_project"]) == "" || strings.TrimSpace(data["image_family"]) == "" {
			return errors.New("GCE images require image_project and image_family")
		}
	case "lxd":
		if strings.TrimSpace(data["image_alias"]) == "" && strings.TrimSpace(data["image_fingerprint"]) == "" {
			return errors.New("LXD images require image_alias or image_fingerprint")
		}
	case "libvirt":
		if strings.TrimSpace(data["volume_name"]) == "" {
			return errors.New("Libvirt images require volume_name")
		}
	}
	return nil
}
