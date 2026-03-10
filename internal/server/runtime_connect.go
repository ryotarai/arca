package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type runtimeConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

var runtimeNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

const maxRuntimeStartupScriptBytes = 8 * 1024

func newRuntimeConnectService(store *db.Store, authenticator Authenticator) *runtimeConnectService {
	return &runtimeConnectService{store: store, authenticator: authenticator}
}

func (s *runtimeConnectService) ListRuntimes(ctx context.Context, req *connect.Request[arcav1.ListRuntimesRequest]) (*connect.Response[arcav1.ListRuntimesResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	runtimes, err := s.store.ListRuntimes(ctx)
	if err != nil {
		log.Printf("list runtimes failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list runtimes"))
	}

	items := make([]*arcav1.Runtime, 0, len(runtimes))
	for _, runtime := range runtimes {
		message, convErr := toRuntimeMessage(runtime)
		if convErr != nil {
			log.Printf("invalid runtime row id=%s: %v", runtime.ID, convErr)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode runtime config"))
		}
		items = append(items, message)
	}

	return connect.NewResponse(&arcav1.ListRuntimesResponse{Runtimes: items}), nil
}

func (s *runtimeConnectService) CreateRuntime(ctx context.Context, req *connect.Request[arcav1.CreateRuntimeRequest]) (*connect.Response[arcav1.CreateRuntimeResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	validated, err := validateRuntimeRequest(req.Msg.GetName(), req.Msg.GetType(), req.Msg.GetConfig())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	configJSON, err := marshalRuntimeConfigJSON(validated.config)
	if err != nil {
		log.Printf("marshal runtime config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save runtime"))
	}

	runtime, err := s.store.CreateRuntime(ctx, validated.name, validated.runtimeType, configJSON)
	if err != nil {
		if errors.Is(err, db.ErrRuntimeNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("runtime name already exists"))
		}
		log.Printf("create runtime failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create runtime"))
	}

	message, err := toRuntimeMessage(runtime)
	if err != nil {
		log.Printf("decode created runtime config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode runtime config"))
	}

	return connect.NewResponse(&arcav1.CreateRuntimeResponse{Runtime: message}), nil
}

func (s *runtimeConnectService) UpdateRuntime(ctx context.Context, req *connect.Request[arcav1.UpdateRuntimeRequest]) (*connect.Response[arcav1.UpdateRuntimeResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	runtimeID := strings.TrimSpace(req.Msg.GetRuntimeId())
	if runtimeID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime id is required"))
	}

	validated, err := validateRuntimeRequest(req.Msg.GetName(), req.Msg.GetType(), req.Msg.GetConfig())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	configJSON, err := marshalRuntimeConfigJSON(validated.config)
	if err != nil {
		log.Printf("marshal runtime config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update runtime"))
	}

	runtime, updated, err := s.store.UpdateRuntimeByID(ctx, runtimeID, validated.name, validated.runtimeType, configJSON)
	if err != nil {
		if errors.Is(err, db.ErrRuntimeNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("runtime name already exists"))
		}
		log.Printf("update runtime failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update runtime"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("runtime not found"))
	}

	message, err := toRuntimeMessage(runtime)
	if err != nil {
		log.Printf("decode updated runtime config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode runtime config"))
	}
	return connect.NewResponse(&arcav1.UpdateRuntimeResponse{Runtime: message}), nil
}

func (s *runtimeConnectService) DeleteRuntime(ctx context.Context, req *connect.Request[arcav1.DeleteRuntimeRequest]) (*connect.Response[arcav1.DeleteRuntimeResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	runtimeID := strings.TrimSpace(req.Msg.GetRuntimeId())
	if runtimeID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime id is required"))
	}

	deleted, err := s.store.DeleteRuntimeByID(ctx, runtimeID)
	if err != nil {
		if errors.Is(err, db.ErrRuntimeInUse) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("runtime is used by existing machines"))
		}
		log.Printf("delete runtime failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete runtime"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("runtime not found"))
	}
	return connect.NewResponse(&arcav1.DeleteRuntimeResponse{}), nil
}

func (s *runtimeConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("runtime management unavailable"))
	}

	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	userID, _, role, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage runtimes"))
	}
	return userID, nil
}

type validatedRuntimeRequest struct {
	name        string
	runtimeType string
	config      *arcav1.RuntimeConfig
}

func validateRuntimeRequest(name string, runtimeType arcav1.RuntimeType, config *arcav1.RuntimeConfig) (validatedRuntimeRequest, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return validatedRuntimeRequest{}, errors.New("name is required")
	}
	if len(normalizedName) < 3 {
		return validatedRuntimeRequest{}, errors.New("name must be at least 3 characters")
	}
	if len(normalizedName) > 63 {
		return validatedRuntimeRequest{}, errors.New("name must be 63 characters or less")
	}
	if !runtimeNamePattern.MatchString(normalizedName) {
		return validatedRuntimeRequest{}, errors.New("name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen")
	}
	if config == nil {
		return validatedRuntimeRequest{}, errors.New("runtime config is required")
	}

	// Validate and normalize the exposure config if present
	var exposureConfig *arcav1.MachineExposureConfig
	if exp := config.GetExposure(); exp != nil {
		exposureConfig = &arcav1.MachineExposureConfig{
			Method:              exp.GetMethod(),
			DomainPrefix:        strings.ToLower(strings.TrimSpace(exp.GetDomainPrefix())),
			BaseDomain:          strings.ToLower(strings.TrimSpace(exp.GetBaseDomain())),
			CloudflareApiToken:  strings.TrimSpace(exp.GetCloudflareApiToken()),
			CloudflareAccountId: strings.TrimSpace(exp.GetCloudflareAccountId()),
			CloudflareZoneId:    strings.TrimSpace(exp.GetCloudflareZoneId()),
			Connectivity:        exp.GetConnectivity(),
		}
	}

	serverApiUrl := strings.TrimSpace(config.GetServerApiUrl())

	switch runtimeType {
	case arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT:
		libvirt := config.GetLibvirt()
		if libvirt == nil || config.GetGce() != nil {
			return validatedRuntimeRequest{}, errors.New("libvirt runtime requires libvirt config only")
		}
		uri := strings.TrimSpace(libvirt.GetUri())
		network := strings.TrimSpace(libvirt.GetNetwork())
		storagePool := strings.TrimSpace(libvirt.GetStoragePool())
		startupScript, err := normalizeRuntimeStartupScript(libvirt.GetStartupScript(), "libvirt startup script")
		if err != nil {
			return validatedRuntimeRequest{}, err
		}
		if uri == "" || network == "" || storagePool == "" {
			return validatedRuntimeRequest{}, errors.New("libvirt config requires uri, network, and storage pool")
		}
		return validatedRuntimeRequest{
			name:        normalizedName,
			runtimeType: db.RuntimeTypeLibvirt,
			config: &arcav1.RuntimeConfig{
				Provider: &arcav1.RuntimeConfig_Libvirt{
					Libvirt: &arcav1.LibvirtRuntimeConfig{
						Uri:           uri,
						Network:       network,
						StoragePool:   storagePool,
						StartupScript: startupScript,
					},
				},
				Exposure:     exposureConfig,
				ServerApiUrl: serverApiUrl,
			},
		}, nil
	case arcav1.RuntimeType_RUNTIME_TYPE_GCE:
		gce := config.GetGce()
		if gce == nil || config.GetLibvirt() != nil {
			return validatedRuntimeRequest{}, errors.New("gce runtime requires gce config only")
		}
		project := strings.TrimSpace(gce.GetProject())
		zone := strings.TrimSpace(gce.GetZone())
		network := strings.TrimSpace(gce.GetNetwork())
		subnetwork := strings.TrimSpace(gce.GetSubnetwork())
		serviceAccountEmail := strings.TrimSpace(gce.GetServiceAccountEmail())
		startupScript, err := normalizeRuntimeStartupScript(gce.GetStartupScript(), "gce startup script")
		if err != nil {
			return validatedRuntimeRequest{}, err
		}
		if project == "" || zone == "" || network == "" || subnetwork == "" || serviceAccountEmail == "" {
			return validatedRuntimeRequest{}, errors.New("gce config requires project, zone, network, subnetwork, and service account email")
		}
		machineType := strings.TrimSpace(gce.GetMachineType())
		diskSizeGb := gce.GetDiskSizeGb()
		imageProject := strings.TrimSpace(gce.GetImageProject())
		imageFamily := strings.TrimSpace(gce.GetImageFamily())
		return validatedRuntimeRequest{
			name:        normalizedName,
			runtimeType: db.RuntimeTypeGCE,
			config: &arcav1.RuntimeConfig{
				Provider: &arcav1.RuntimeConfig_Gce{
					Gce: &arcav1.GceRuntimeConfig{
						Project:             project,
						Zone:                zone,
						Network:             network,
						Subnetwork:          subnetwork,
						ServiceAccountEmail: serviceAccountEmail,
						StartupScript:       startupScript,
						MachineType:         machineType,
						DiskSizeGb:          diskSizeGb,
						ImageProject:        imageProject,
						ImageFamily:         imageFamily,
					},
				},
				Exposure:     exposureConfig,
				ServerApiUrl: serverApiUrl,
			},
		}, nil
	case arcav1.RuntimeType_RUNTIME_TYPE_LXD:
		lxd := config.GetLxd()
		if lxd == nil || config.GetLibvirt() != nil || config.GetGce() != nil {
			return validatedRuntimeRequest{}, errors.New("lxd runtime requires lxd config only")
		}
		endpoint := strings.TrimSpace(lxd.GetEndpoint())
		startupScript, err := normalizeRuntimeStartupScript(lxd.GetStartupScript(), "lxd startup script")
		if err != nil {
			return validatedRuntimeRequest{}, err
		}
		if endpoint == "" {
			return validatedRuntimeRequest{}, errors.New("lxd config requires endpoint")
		}
		return validatedRuntimeRequest{
			name:        normalizedName,
			runtimeType: db.RuntimeTypeLXD,
			config: &arcav1.RuntimeConfig{
				Provider: &arcav1.RuntimeConfig_Lxd{
					Lxd: &arcav1.LxdRuntimeConfig{
						Endpoint:      endpoint,
						StartupScript: startupScript,
					},
				},
				Exposure:     exposureConfig,
				ServerApiUrl: serverApiUrl,
			},
		}, nil
	default:
		return validatedRuntimeRequest{}, errors.New("runtime type is unsupported")
	}
}

func normalizeRuntimeStartupScript(startupScript string, label string) (string, error) {
	if strings.TrimSpace(startupScript) == "" {
		return "", nil
	}
	if len([]byte(startupScript)) > maxRuntimeStartupScriptBytes {
		return "", errors.New(label + " must be 8KB or less")
	}
	return startupScript, nil
}

func marshalRuntimeConfigJSON(config *arcav1.RuntimeConfig) (string, error) {
	data, err := protojson.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func toRuntimeMessage(runtime db.RuntimeCatalog) (*arcav1.Runtime, error) {
	runtimeType, err := runtimeTypeFromDB(runtime.Type)
	if err != nil {
		return nil, err
	}
	config, err := unmarshalRuntimeConfigJSON(runtime.ConfigJSON)
	if err != nil {
		return nil, err
	}
	if _, err := validateRuntimeRequest(runtime.Name, runtimeType, config); err != nil {
		return nil, err
	}

	return &arcav1.Runtime{
		Id:        runtime.ID,
		Name:      runtime.Name,
		Type:      runtimeType,
		Config:    config,
		CreatedAt: runtime.CreatedAt,
		UpdatedAt: runtime.UpdatedAt,
	}, nil
}

func runtimeTypeFromDB(runtimeType string) (arcav1.RuntimeType, error) {
	switch strings.ToLower(strings.TrimSpace(runtimeType)) {
	case db.RuntimeTypeLibvirt:
		return arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT, nil
	case db.RuntimeTypeGCE:
		return arcav1.RuntimeType_RUNTIME_TYPE_GCE, nil
	case db.RuntimeTypeLXD:
		return arcav1.RuntimeType_RUNTIME_TYPE_LXD, nil
	default:
		return arcav1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED, errors.New("unknown runtime type")
	}
}

func unmarshalRuntimeConfigJSON(raw string) (*arcav1.RuntimeConfig, error) {
	config := &arcav1.RuntimeConfig{}
	if err := protojson.Unmarshal([]byte(raw), config); err != nil {
		return nil, err
	}
	return config, nil
}
