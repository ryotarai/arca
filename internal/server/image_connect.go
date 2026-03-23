package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	var images []db.CustomImage
	if result.Role == db.UserRoleAdmin {
		images, err = s.store.ListCustomImages(ctx)
	} else {
		images, err = s.store.ListCustomImagesByUserOrShared(ctx, result.UserID)
	}
	if err != nil {
		slog.ErrorContext(ctx, "list custom images failed", "error", err)
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

	img, err := s.store.CreateCustomImage(ctx, name, runtimeType, string(dataJSON), req.Msg.GetDescription(), adminUserID)
	if err != nil {
		if errors.Is(err, db.ErrCustomImageNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("image with this name and runtime type already exists"))
		}
		slog.ErrorContext(ctx, "create custom image failed", "error", err)
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
			slog.ErrorContext(ctx, "associate runtime custom image failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to associate runtime"))
		}
	}

	associatedIDs, _ := s.store.ListTemplateIDsByCustomImageID(ctx, img.ID)

	writeAuditLog(ctx, s.store, adminUserID, "", "image.create", "custom_image", img.ID, fmt.Sprintf(`{"name":%q}`, name))

	return connect.NewResponse(&arcav1.CreateCustomImageResponse{Image: toCustomImageMessage(img, associatedIDs)}), nil
}

func (s *imageConnectService) UpdateCustomImage(ctx context.Context, req *connect.Request[arcav1.UpdateCustomImageRequest]) (*connect.Response[arcav1.UpdateCustomImageResponse], error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}
	isAdmin := result.Role == db.UserRoleAdmin

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	// Fetch existing image for ownership/field checks
	existing, err := s.store.GetCustomImage(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
		}
		slog.ErrorContext(ctx, "get custom image failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get custom image"))
	}

	var runtimeType string
	var dataJSON string
	var visibility string

	if isAdmin {
		// Admin can change all fields
		runtimeType = strings.ToLower(strings.TrimSpace(req.Msg.GetTemplateType()))
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

		dataJSONBytes, err := json.Marshal(data)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encode data"))
		}
		dataJSON = string(dataJSONBytes)

		visibility = req.Msg.GetVisibility()
		if visibility == "" {
			visibility = existing.Visibility
		}
		if visibility != "private" && visibility != "shared" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("visibility must be 'private' or 'shared'"))
		}
	} else {
		// Non-admin: verify ownership
		if existing.CreatedByUserID != result.UserID {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("you do not own this image"))
		}

		// Non-admin: reject forbidden field changes
		reqVisibility := req.Msg.GetVisibility()
		if reqVisibility != "" && reqVisibility != existing.Visibility {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admins can change visibility"))
		}

		if len(req.Msg.GetTemplateIds()) > 0 {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admins can change template associations"))
		}

		reqRuntimeType := strings.ToLower(strings.TrimSpace(req.Msg.GetTemplateType()))
		if reqRuntimeType != "" && reqRuntimeType != existing.ProviderType {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admins can change provider type"))
		}

		reqData := req.Msg.GetData()
		var existingData map[string]string
		if existing.DataJSON != "" && existing.DataJSON != "{}" {
			_ = json.Unmarshal([]byte(existing.DataJSON), &existingData)
		}
		if existingData == nil {
			existingData = make(map[string]string)
		}
		if len(reqData) > 0 && !mapsEqual(reqData, existingData) {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admins can change image data"))
		}

		// Non-admin: use existing values for restricted fields
		runtimeType = existing.ProviderType
		dataJSON = existing.DataJSON
		visibility = existing.Visibility
	}

	img, updated, err := s.store.UpdateCustomImage(ctx, id, name, runtimeType, dataJSON, req.Msg.GetDescription(), visibility)
	if err != nil {
		if errors.Is(err, db.ErrCustomImageNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("image with this name and runtime type already exists"))
		}
		slog.ErrorContext(ctx, "update custom image failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update custom image"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
	}

	// Re-sync runtime associations (admin only)
	if isAdmin {
		if err := s.store.DisassociateAllTemplatesFromCustomImage(ctx, id); err != nil {
			slog.ErrorContext(ctx, "disassociate runtimes failed", "error", err)
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
				slog.ErrorContext(ctx, "associate runtime custom image failed", "error", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to associate runtime"))
			}
		}
	}

	associatedIDs, _ := s.store.ListTemplateIDsByCustomImageID(ctx, id)

	writeAuditLog(ctx, s.store, result.UserID, "", "image.update", "custom_image", id, fmt.Sprintf(`{"name":%q}`, name))

	return connect.NewResponse(&arcav1.UpdateCustomImageResponse{Image: toCustomImageMessage(img, associatedIDs)}), nil
}

func (s *imageConnectService) DeleteCustomImage(ctx context.Context, req *connect.Request[arcav1.DeleteCustomImageRequest]) (*connect.Response[arcav1.DeleteCustomImageResponse], error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	// Fetch image for ownership check and audit log
	img, err := s.store.GetCustomImage(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
		}
		slog.ErrorContext(ctx, "get custom image failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get custom image"))
	}

	// Non-admin must own the image and it must be private
	if result.Role != db.UserRoleAdmin {
		if img.CreatedByUserID != result.UserID {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("you do not own this image"))
		}
		if img.Visibility == "shared" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("shared images cannot be deleted by non-admin users"))
		}
	}

	deleted, err := s.store.DeleteCustomImage(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "delete custom image failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete custom image"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
	}

	writeAuditLog(ctx, s.store, result.UserID, "", "image.delete", "custom_image", id, fmt.Sprintf(`{"name":%q}`, img.Name))

	return connect.NewResponse(&arcav1.DeleteCustomImageResponse{}), nil
}

func (s *imageConnectService) ListAvailableImages(ctx context.Context, req *connect.Request[arcav1.ListAvailableImagesRequest]) (*connect.Response[arcav1.ListAvailableImagesResponse], error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	runtimeID := strings.TrimSpace(req.Msg.GetTemplateId())
	if runtimeID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_id is required"))
	}

	// All users (including admin) see only their own private + shared images
	images, err := s.store.ListCustomImagesByUserOrSharedAndProfileID(ctx, result.UserID, runtimeID)
	if err != nil {
		slog.ErrorContext(ctx, "list available images failed", "error", err)
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
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return "", err
	}
	if result.Role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage images"))
	}
	return result.UserID, nil
}

func (s *imageConnectService) validateTemplateTypeMatch(ctx context.Context, templateID, expectedType string) error {
	tmpl, err := s.store.GetMachineProfileByID(ctx, templateID)
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
		TemplateType:          img.ProviderType,
		Data:                  data,
		Description:           img.Description,
		AssociatedTemplateIds: templateIDs,
		CreatedAt:             img.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		SourceMachineId:       img.SourceMachineID,
		CreatedByUserId:       img.CreatedByUserID,
		Visibility:            img.Visibility,
	}
}

func validateCustomImageData(runtimeType string, data map[string]string) error {
	switch runtimeType {
	case "gce":
		if strings.TrimSpace(data["image_project"]) == "" {
			return errors.New("GCE images require image_project")
		}
		if strings.TrimSpace(data["image_name"]) == "" && strings.TrimSpace(data["image_family"]) == "" {
			return errors.New("GCE images require image_name or image_family")
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

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
