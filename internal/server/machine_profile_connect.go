package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type machineProfileConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

var profileNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

const maxProfileStartupScriptBytes = 8 * 1024

func newMachineProfileConnectService(store *db.Store, authenticator Authenticator) *machineProfileConnectService {
	return &machineProfileConnectService{store: store, authenticator: authenticator}
}

func (s *machineProfileConnectService) ListMachineProfiles(ctx context.Context, req *connect.Request[arcav1.ListMachineProfilesRequest]) (*connect.Response[arcav1.ListMachineProfilesResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	profiles, err := s.store.ListMachineProfiles(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list profiles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list profiles"))
	}

	items := make([]*arcav1.MachineProfile, 0, len(profiles))
	for _, profile := range profiles {
		message, convErr := toProfileMessage(profile)
		if convErr != nil {
			slog.ErrorContext(ctx, "invalid profile row", "profile_id", profile.ID, "error", convErr)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode profile config"))
		}
		items = append(items, message)
	}

	return connect.NewResponse(&arcav1.ListMachineProfilesResponse{Profiles: items}), nil
}

func (s *machineProfileConnectService) CreateMachineProfile(ctx context.Context, req *connect.Request[arcav1.CreateMachineProfileRequest]) (*connect.Response[arcav1.CreateMachineProfileResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	validated, err := validateProfileRequest(req.Msg.GetName(), req.Msg.GetType(), req.Msg.GetConfig())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	configJSON, err := marshalProfileConfigJSON(validated.config)
	if err != nil {
		slog.ErrorContext(ctx, "marshal profile config failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save profile"))
	}

	profile, err := s.store.CreateMachineProfile(ctx, validated.name, validated.profileType, configJSON)
	if err != nil {
		if errors.Is(err, db.ErrProfileNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("profile name already exists"))
		}
		slog.ErrorContext(ctx, "create profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create profile"))
	}

	message, err := toProfileMessage(profile)
	if err != nil {
		slog.ErrorContext(ctx, "decode created profile config failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode profile config"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "profile.create", "profile", profile.ID, fmt.Sprintf(`{"name":%q,"type":%q}`, validated.name, validated.profileType))

	return connect.NewResponse(&arcav1.CreateMachineProfileResponse{Profile: message}), nil
}

func (s *machineProfileConnectService) UpdateMachineProfile(ctx context.Context, req *connect.Request[arcav1.UpdateMachineProfileRequest]) (*connect.Response[arcav1.UpdateMachineProfileResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	profileID := strings.TrimSpace(req.Msg.GetProfileId())
	if profileID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile id is required"))
	}

	validated, err := validateProfileRequest(req.Msg.GetName(), req.Msg.GetType(), req.Msg.GetConfig())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	configJSON, err := marshalProfileConfigJSON(validated.config)
	if err != nil {
		slog.ErrorContext(ctx, "marshal profile config failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update profile"))
	}

	profile, updated, err := s.store.UpdateMachineProfileByID(ctx, profileID, validated.name, validated.profileType, configJSON)
	if err != nil {
		if errors.Is(err, db.ErrProfileNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("profile name already exists"))
		}
		slog.ErrorContext(ctx, "update profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update profile"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("profile not found"))
	}

	message, err := toProfileMessage(profile)
	if err != nil {
		slog.ErrorContext(ctx, "decode updated profile config failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to decode profile config"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "profile.update", "profile", profileID, fmt.Sprintf(`{"name":%q}`, validated.name))

	return connect.NewResponse(&arcav1.UpdateMachineProfileResponse{Profile: message}), nil
}

func (s *machineProfileConnectService) DeleteMachineProfile(ctx context.Context, req *connect.Request[arcav1.DeleteMachineProfileRequest]) (*connect.Response[arcav1.DeleteMachineProfileResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	profileID := strings.TrimSpace(req.Msg.GetProfileId())
	if profileID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile id is required"))
	}

	// Fetch name before deletion for audit log
	var profileName string
	if p, pErr := s.store.GetMachineProfileByID(ctx, profileID); pErr == nil {
		profileName = p.Name
	}

	deleted, err := s.store.DeleteMachineProfileByID(ctx, profileID)
	if err != nil {
		if errors.Is(err, db.ErrProfileInUse) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("profile is used by existing machines"))
		}
		slog.ErrorContext(ctx, "delete profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete profile"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("profile not found"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "profile.delete", "profile", profileID, fmt.Sprintf(`{"name":%q}`, profileName))

	return connect.NewResponse(&arcav1.DeleteMachineProfileResponse{}), nil
}

func (s *machineProfileConnectService) ListAvailableProfiles(ctx context.Context, req *connect.Request[arcav1.ListAvailableProfilesRequest]) (*connect.Response[arcav1.ListAvailableProfilesResponse], error) {
	if _, err := s.authenticate(ctx, req.Header()); err != nil {
		return nil, err
	}

	profiles, err := s.store.ListMachineProfiles(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list available profiles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list profiles"))
	}

	items := make([]*arcav1.MachineProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		profileType, err := profileTypeFromDB(profile.Type)
		if err != nil {
			slog.ErrorContext(ctx, "invalid profile row", "profile_id", profile.ID, "error", err)
			continue
		}
		summary := &arcav1.MachineProfileSummary{
			Id:   profile.ID,
			Name: profile.Name,
			Type: profileType,
		}
		if config, err := unmarshalProfileConfigJSON(profile.ConfigJSON); err == nil {
			if gce := config.GetGce(); gce != nil {
				summary.AllowedMachineTypes = gce.GetAllowedMachineTypes()
			}
		}
		items = append(items, summary)
	}

	return connect.NewResponse(&arcav1.ListAvailableProfilesResponse{Profiles: items}), nil
}

func (s *machineProfileConnectService) authenticate(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("profile service unavailable"))
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

func (s *machineProfileConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("profile management unavailable"))
	}
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return "", err
	}
	if result.Role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage profiles"))
	}
	return result.UserID, nil
}

type validatedProfileRequest struct {
	name        string
	profileType string
	config      *arcav1.MachineProfileConfig
}

func validateProfileRequest(name string, profileType arcav1.MachineProfileType, config *arcav1.MachineProfileConfig) (validatedProfileRequest, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return validatedProfileRequest{}, errors.New("name is required")
	}
	if len(normalizedName) < 3 {
		return validatedProfileRequest{}, errors.New("name must be at least 3 characters")
	}
	if len(normalizedName) > 63 {
		return validatedProfileRequest{}, errors.New("name must be 63 characters or less")
	}
	if !profileNamePattern.MatchString(normalizedName) {
		return validatedProfileRequest{}, errors.New("name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen")
	}
	if config == nil {
		return validatedProfileRequest{}, errors.New("profile config is required")
	}

	// Validate and normalize the exposure config if present
	var exposureConfig *arcav1.MachineExposureConfig
	if exp := config.GetExposure(); exp != nil {
		exposureConfig = &arcav1.MachineExposureConfig{
			Method:       exp.GetMethod(),
			Connectivity: exp.GetConnectivity(),
		}
	}

	serverApiUrl := strings.TrimSpace(config.GetServerApiUrl())
	autoStopTimeoutSeconds := config.GetAutoStopTimeoutSeconds()
	if autoStopTimeoutSeconds < 0 {
		autoStopTimeoutSeconds = 0
	}

	switch profileType {
	case arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT:
		libvirt := config.GetLibvirt()
		if libvirt == nil || config.GetGce() != nil {
			return validatedProfileRequest{}, errors.New("libvirt profile requires libvirt config only")
		}
		uri := strings.TrimSpace(libvirt.GetUri())
		network := strings.TrimSpace(libvirt.GetNetwork())
		storagePool := strings.TrimSpace(libvirt.GetStoragePool())
		startupScript, err := normalizeProfileStartupScript(libvirt.GetStartupScript(), "libvirt startup script")
		if err != nil {
			return validatedProfileRequest{}, err
		}
		if uri == "" || network == "" || storagePool == "" {
			return validatedProfileRequest{}, errors.New("libvirt config requires uri, network, and storage pool")
		}
		return validatedProfileRequest{
			name:        normalizedName,
			profileType: db.ProviderTypeLibvirt,
			config: &arcav1.MachineProfileConfig{
				Provider: &arcav1.MachineProfileConfig_Libvirt{
					Libvirt: &arcav1.LibvirtProfileConfig{
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
	case arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_GCE:
		gce := config.GetGce()
		if gce == nil || config.GetLibvirt() != nil {
			return validatedProfileRequest{}, errors.New("gce profile requires gce config only")
		}
		project := strings.TrimSpace(gce.GetProject())
		zone := strings.TrimSpace(gce.GetZone())
		network := strings.TrimSpace(gce.GetNetwork())
		subnetwork := strings.TrimSpace(gce.GetSubnetwork())
		serviceAccountEmail := strings.TrimSpace(gce.GetServiceAccountEmail())
		startupScript, err := normalizeProfileStartupScript(gce.GetStartupScript(), "gce startup script")
		if err != nil {
			return validatedProfileRequest{}, err
		}
		if project == "" || zone == "" || network == "" || subnetwork == "" || serviceAccountEmail == "" {
			return validatedProfileRequest{}, errors.New("gce config requires project, zone, network, subnetwork, and service account email")
		}
		diskSizeGb := gce.GetDiskSizeGb()
		allowedMachineTypes := gce.GetAllowedMachineTypes()
		if len(allowedMachineTypes) == 0 {
			return validatedProfileRequest{}, errors.New("gce config requires at least one allowed machine type")
		}
		return validatedProfileRequest{
			name:        normalizedName,
			profileType: db.ProviderTypeGCE,
			config: &arcav1.MachineProfileConfig{
				Provider: &arcav1.MachineProfileConfig_Gce{
					Gce: &arcav1.GceProfileConfig{
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
	case arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LXD:
		lxd := config.GetLxd()
		if lxd == nil || config.GetLibvirt() != nil || config.GetGce() != nil {
			return validatedProfileRequest{}, errors.New("lxd profile requires lxd config only")
		}
		endpoint := strings.TrimSpace(lxd.GetEndpoint())
		startupScript, err := normalizeProfileStartupScript(lxd.GetStartupScript(), "lxd startup script")
		if err != nil {
			return validatedProfileRequest{}, err
		}
		if endpoint == "" {
			return validatedProfileRequest{}, errors.New("lxd config requires endpoint")
		}
		return validatedProfileRequest{
			name:        normalizedName,
			profileType: db.ProviderTypeLXD,
			config: &arcav1.MachineProfileConfig{
				Provider: &arcav1.MachineProfileConfig_Lxd{
					Lxd: &arcav1.LxdProfileConfig{
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
		return validatedProfileRequest{}, errors.New("profile type is unsupported")
	}
}

func normalizeProfileStartupScript(startupScript string, label string) (string, error) {
	if strings.TrimSpace(startupScript) == "" {
		return "", nil
	}
	if len([]byte(startupScript)) > maxProfileStartupScriptBytes {
		return "", errors.New(label + " must be 8KB or less")
	}
	return startupScript, nil
}

func marshalProfileConfigJSON(config *arcav1.MachineProfileConfig) (string, error) {
	data, err := protojson.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func toProfileMessage(profile db.MachineProfile) (*arcav1.MachineProfile, error) {
	profileType, err := profileTypeFromDB(profile.Type)
	if err != nil {
		return nil, err
	}
	config, err := unmarshalProfileConfigJSON(profile.ConfigJSON)
	if err != nil {
		return nil, err
	}
	if _, err := validateProfileRequest(profile.Name, profileType, config); err != nil {
		return nil, err
	}

	return &arcav1.MachineProfile{
		Id:             profile.ID,
		Name:           profile.Name,
		Type:           profileType,
		Config:         config,
		CreatedAt:      profile.CreatedAt,
		UpdatedAt:      profile.UpdatedAt,
		BootConfigHash: profile.BootConfigHash,
	}, nil
}

func profileTypeFromDB(profileType string) (arcav1.MachineProfileType, error) {
	switch strings.ToLower(strings.TrimSpace(profileType)) {
	case db.ProviderTypeLibvirt:
		return arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT, nil
	case db.ProviderTypeGCE:
		return arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_GCE, nil
	case db.ProviderTypeLXD:
		return arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LXD, nil
	default:
		return arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_UNSPECIFIED, errors.New("unknown profile type")
	}
}

func unmarshalProfileConfigJSON(raw string) (*arcav1.MachineProfileConfig, error) {
	config := &arcav1.MachineProfileConfig{}
	unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshaler.Unmarshal([]byte(raw), config); err != nil {
		return nil, err
	}
	return config, nil
}
