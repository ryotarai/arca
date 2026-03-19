package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type machineTemplateConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

var templateNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

const maxTemplateStartupScriptBytes = 8 * 1024

func newMachineTemplateConnectService(store *db.Store, authenticator Authenticator) *machineTemplateConnectService {
	return &machineTemplateConnectService{store: store, authenticator: authenticator}
}

func (s *machineTemplateConnectService) ListMachineTemplates(ctx context.Context, req *connect.Request[arcav1.ListMachineTemplatesRequest]) (*connect.Response[arcav1.ListMachineTemplatesResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	templates, err := s.store.ListMachineTemplates(ctx)
	if err != nil {
		log.Printf("list templates failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list templates"))
	}

	items := make([]*arcav1.MachineTemplate, 0, len(templates))
	for _, template := range templates {
		message, convErr := toTemplateMessage(template)
		if convErr != nil {
			log.Printf("invalid template row id=%s: %v", template.ID, convErr)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode template config"))
		}
		items = append(items, message)
	}

	return connect.NewResponse(&arcav1.ListMachineTemplatesResponse{Templates: items}), nil
}

func (s *machineTemplateConnectService) CreateMachineTemplate(ctx context.Context, req *connect.Request[arcav1.CreateMachineTemplateRequest]) (*connect.Response[arcav1.CreateMachineTemplateResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	validated, err := validateTemplateRequest(req.Msg.GetName(), req.Msg.GetType(), req.Msg.GetConfig())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	configJSON, err := marshalTemplateConfigJSON(validated.config)
	if err != nil {
		log.Printf("marshal template config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save template"))
	}

	template, err := s.store.CreateMachineTemplate(ctx, validated.name, validated.templateType, configJSON)
	if err != nil {
		if errors.Is(err, db.ErrTemplateNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("template name already exists"))
		}
		log.Printf("create template failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create template"))
	}

	message, err := toTemplateMessage(template)
	if err != nil {
		log.Printf("decode created template config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode template config"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "template.create", "template", template.ID, fmt.Sprintf(`{"name":%q,"type":%q}`, validated.name, validated.templateType))

	return connect.NewResponse(&arcav1.CreateMachineTemplateResponse{Template: message}), nil
}

func (s *machineTemplateConnectService) UpdateMachineTemplate(ctx context.Context, req *connect.Request[arcav1.UpdateMachineTemplateRequest]) (*connect.Response[arcav1.UpdateMachineTemplateResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	templateID := strings.TrimSpace(req.Msg.GetTemplateId())
	if templateID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("template id is required"))
	}

	validated, err := validateTemplateRequest(req.Msg.GetName(), req.Msg.GetType(), req.Msg.GetConfig())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	configJSON, err := marshalTemplateConfigJSON(validated.config)
	if err != nil {
		log.Printf("marshal template config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update template"))
	}

	template, updated, err := s.store.UpdateMachineTemplateByID(ctx, templateID, validated.name, validated.templateType, configJSON)
	if err != nil {
		if errors.Is(err, db.ErrTemplateNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("template name already exists"))
		}
		log.Printf("update template failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update template"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("template not found"))
	}

	message, err := toTemplateMessage(template)
	if err != nil {
		log.Printf("decode updated template config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode template config"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "template.update", "template", templateID, fmt.Sprintf(`{"name":%q}`, validated.name))

	return connect.NewResponse(&arcav1.UpdateMachineTemplateResponse{Template: message}), nil
}

func (s *machineTemplateConnectService) DeleteMachineTemplate(ctx context.Context, req *connect.Request[arcav1.DeleteMachineTemplateRequest]) (*connect.Response[arcav1.DeleteMachineTemplateResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	templateID := strings.TrimSpace(req.Msg.GetTemplateId())
	if templateID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("template id is required"))
	}

	// Fetch name before deletion for audit log
	var templateName string
	if rt, rtErr := s.store.GetMachineTemplateByID(ctx, templateID); rtErr == nil {
		templateName = rt.Name
	}

	deleted, err := s.store.DeleteMachineTemplateByID(ctx, templateID)
	if err != nil {
		if errors.Is(err, db.ErrTemplateInUse) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("template is used by existing machines"))
		}
		log.Printf("delete template failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete template"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("template not found"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "template.delete", "template", templateID, fmt.Sprintf(`{"name":%q}`, templateName))

	return connect.NewResponse(&arcav1.DeleteMachineTemplateResponse{}), nil
}

func (s *machineTemplateConnectService) ListAvailableMachineTemplates(ctx context.Context, req *connect.Request[arcav1.ListAvailableMachineTemplatesRequest]) (*connect.Response[arcav1.ListAvailableMachineTemplatesResponse], error) {
	if _, err := s.authenticate(ctx, req.Header()); err != nil {
		return nil, err
	}

	templates, err := s.store.ListMachineTemplates(ctx)
	if err != nil {
		log.Printf("list available templates failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list templates"))
	}

	items := make([]*arcav1.MachineTemplateSummary, 0, len(templates))
	for _, template := range templates {
		templateType, err := templateTypeFromDB(template.Type)
		if err != nil {
			log.Printf("invalid template row id=%s: %v", template.ID, err)
			continue
		}
		items = append(items, &arcav1.MachineTemplateSummary{
			Id:   template.ID,
			Name: template.Name,
			Type: templateType,
		})
	}

	return connect.NewResponse(&arcav1.ListAvailableMachineTemplatesResponse{Templates: items}), nil
}

func (s *machineTemplateConnectService) authenticate(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("template service unavailable"))
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

func (s *machineTemplateConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("template management unavailable"))
	}

	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		userID, _, role, err := s.authenticator.Authenticate(ctx, sessionToken)
		if err == nil {
			if role != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage templates"))
			}
			return userID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, role, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			if role != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage templates"))
			}
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

type validatedTemplateRequest struct {
	name         string
	templateType string
	config       *arcav1.MachineTemplateConfig
}

func validateTemplateRequest(name string, templateType arcav1.MachineTemplateType, config *arcav1.MachineTemplateConfig) (validatedTemplateRequest, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return validatedTemplateRequest{}, errors.New("name is required")
	}
	if len(normalizedName) < 3 {
		return validatedTemplateRequest{}, errors.New("name must be at least 3 characters")
	}
	if len(normalizedName) > 63 {
		return validatedTemplateRequest{}, errors.New("name must be 63 characters or less")
	}
	if !templateNamePattern.MatchString(normalizedName) {
		return validatedTemplateRequest{}, errors.New("name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen")
	}
	if config == nil {
		return validatedTemplateRequest{}, errors.New("template config is required")
	}

	// Validate and normalize the exposure config if present
	var exposureConfig *arcav1.MachineExposureConfig
	if exp := config.GetExposure(); exp != nil {
		exposureConfig = &arcav1.MachineExposureConfig{
			Method:       exp.GetMethod(),
			DomainPrefix: strings.ToLower(strings.TrimSpace(exp.GetDomainPrefix())),
			BaseDomain:   strings.ToLower(strings.TrimSpace(exp.GetBaseDomain())),
			Connectivity: exp.GetConnectivity(),
		}
	}

	serverApiUrl := strings.TrimSpace(config.GetServerApiUrl())
	autoStopTimeoutSeconds := config.GetAutoStopTimeoutSeconds()
	if autoStopTimeoutSeconds < 0 {
		autoStopTimeoutSeconds = 0
	}

	switch templateType {
	case arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT:
		libvirt := config.GetLibvirt()
		if libvirt == nil || config.GetGce() != nil {
			return validatedTemplateRequest{}, errors.New("libvirt template requires libvirt config only")
		}
		uri := strings.TrimSpace(libvirt.GetUri())
		network := strings.TrimSpace(libvirt.GetNetwork())
		storagePool := strings.TrimSpace(libvirt.GetStoragePool())
		startupScript, err := normalizeTemplateStartupScript(libvirt.GetStartupScript(), "libvirt startup script")
		if err != nil {
			return validatedTemplateRequest{}, err
		}
		if uri == "" || network == "" || storagePool == "" {
			return validatedTemplateRequest{}, errors.New("libvirt config requires uri, network, and storage pool")
		}
		return validatedTemplateRequest{
			name:         normalizedName,
			templateType: db.TemplateTypeLibvirt,
			config: &arcav1.MachineTemplateConfig{
				Provider: &arcav1.MachineTemplateConfig_Libvirt{
					Libvirt: &arcav1.LibvirtTemplateConfig{
						Uri:           uri,
						Network:       network,
						StoragePool:   storagePool,
						StartupScript: startupScript,
					},
				},
				Exposure:               exposureConfig,
				ServerApiUrl:           serverApiUrl,
				AutoStopTimeoutSeconds: autoStopTimeoutSeconds,
			},
		}, nil
	case arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_GCE:
		gce := config.GetGce()
		if gce == nil || config.GetLibvirt() != nil {
			return validatedTemplateRequest{}, errors.New("gce template requires gce config only")
		}
		project := strings.TrimSpace(gce.GetProject())
		zone := strings.TrimSpace(gce.GetZone())
		network := strings.TrimSpace(gce.GetNetwork())
		subnetwork := strings.TrimSpace(gce.GetSubnetwork())
		serviceAccountEmail := strings.TrimSpace(gce.GetServiceAccountEmail())
		startupScript, err := normalizeTemplateStartupScript(gce.GetStartupScript(), "gce startup script")
		if err != nil {
			return validatedTemplateRequest{}, err
		}
		if project == "" || zone == "" || network == "" || subnetwork == "" || serviceAccountEmail == "" {
			return validatedTemplateRequest{}, errors.New("gce config requires project, zone, network, subnetwork, and service account email")
		}
		diskSizeGb := gce.GetDiskSizeGb()
		allowedMachineTypes := gce.GetAllowedMachineTypes()
		if len(allowedMachineTypes) == 0 {
			return validatedTemplateRequest{}, errors.New("gce config requires at least one allowed machine type")
		}
		return validatedTemplateRequest{
			name:         normalizedName,
			templateType: db.TemplateTypeGCE,
			config: &arcav1.MachineTemplateConfig{
				Provider: &arcav1.MachineTemplateConfig_Gce{
					Gce: &arcav1.GceTemplateConfig{
						Project:             project,
						Zone:                zone,
						Network:             network,
						Subnetwork:          subnetwork,
						ServiceAccountEmail: serviceAccountEmail,
						StartupScript:       startupScript,
						DiskSizeGb:          diskSizeGb,
						AllowedMachineTypes: allowedMachineTypes,
					},
				},
				Exposure:               exposureConfig,
				ServerApiUrl:           serverApiUrl,
				AutoStopTimeoutSeconds: autoStopTimeoutSeconds,
			},
		}, nil
	case arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LXD:
		lxd := config.GetLxd()
		if lxd == nil || config.GetLibvirt() != nil || config.GetGce() != nil {
			return validatedTemplateRequest{}, errors.New("lxd template requires lxd config only")
		}
		endpoint := strings.TrimSpace(lxd.GetEndpoint())
		startupScript, err := normalizeTemplateStartupScript(lxd.GetStartupScript(), "lxd startup script")
		if err != nil {
			return validatedTemplateRequest{}, err
		}
		if endpoint == "" {
			return validatedTemplateRequest{}, errors.New("lxd config requires endpoint")
		}
		return validatedTemplateRequest{
			name:         normalizedName,
			templateType: db.TemplateTypeLXD,
			config: &arcav1.MachineTemplateConfig{
				Provider: &arcav1.MachineTemplateConfig_Lxd{
					Lxd: &arcav1.LxdTemplateConfig{
						Endpoint:      endpoint,
						StartupScript: startupScript,
					},
				},
				Exposure:               exposureConfig,
				ServerApiUrl:           serverApiUrl,
				AutoStopTimeoutSeconds: autoStopTimeoutSeconds,
			},
		}, nil
	default:
		return validatedTemplateRequest{}, errors.New("template type is unsupported")
	}
}

func normalizeTemplateStartupScript(startupScript string, label string) (string, error) {
	if strings.TrimSpace(startupScript) == "" {
		return "", nil
	}
	if len([]byte(startupScript)) > maxTemplateStartupScriptBytes {
		return "", errors.New(label + " must be 8KB or less")
	}
	return startupScript, nil
}

func marshalTemplateConfigJSON(config *arcav1.MachineTemplateConfig) (string, error) {
	data, err := protojson.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func toTemplateMessage(template db.MachineTemplate) (*arcav1.MachineTemplate, error) {
	templateType, err := templateTypeFromDB(template.Type)
	if err != nil {
		return nil, err
	}
	config, err := unmarshalTemplateConfigJSON(template.ConfigJSON)
	if err != nil {
		return nil, err
	}
	if _, err := validateTemplateRequest(template.Name, templateType, config); err != nil {
		return nil, err
	}

	return &arcav1.MachineTemplate{
		Id:        template.ID,
		Name:      template.Name,
		Type:      templateType,
		Config:    config,
		CreatedAt: template.CreatedAt,
		UpdatedAt: template.UpdatedAt,
	}, nil
}

func templateTypeFromDB(templateType string) (arcav1.MachineTemplateType, error) {
	switch strings.ToLower(strings.TrimSpace(templateType)) {
	case db.TemplateTypeLibvirt:
		return arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT, nil
	case db.TemplateTypeGCE:
		return arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_GCE, nil
	case db.TemplateTypeLXD:
		return arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LXD, nil
	default:
		return arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_UNSPECIFIED, errors.New("unknown template type")
	}
}

func unmarshalTemplateConfigJSON(raw string) (*arcav1.MachineTemplateConfig, error) {
	config := &arcav1.MachineTemplateConfig{}
	if err := protojson.Unmarshal([]byte(raw), config); err != nil {
		return nil, err
	}
	return config, nil
}
