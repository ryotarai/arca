package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/cloudflare"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type tunnelConnectService struct {
	store         *db.Store
	authenticator Authenticator
	cf            *cloudflare.Client
}

func newTunnelConnectService(store *db.Store, authenticator Authenticator, cf *cloudflare.Client) *tunnelConnectService {
	return &tunnelConnectService{store: store, authenticator: authenticator, cf: cf}
}

func (s *tunnelConnectService) CreateMachineTunnel(ctx context.Context, req *connect.Request[arcav1.CreateMachineTunnelRequest]) (*connect.Response[arcav1.CreateMachineTunnelResponse], error) {
	if s.store == nil || s.authenticator == nil || s.cf == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
	}

	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	accountID := strings.TrimSpace(req.Msg.GetAccountId())
	tunnelName := strings.TrimSpace(req.Msg.GetTunnelName())
	if machineID == "" || accountID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id and account id are required"))
	}
	if tunnelName == "" {
		tunnelName = fmt.Sprintf("machine-%s", machineID)
	}

	if _, err := s.store.GetMachineByIDForUser(ctx, userID, machineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("authorize machine for tunnel create failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
	}

	setupState, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("load setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if strings.TrimSpace(setupState.CloudflareAPIToken) == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cloudflare token is not configured"))
	}

	tunnel, err := s.cf.CreateTunnel(ctx, setupState.CloudflareAPIToken, accountID, tunnelName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create cloudflare tunnel"))
	}
	tunnelToken, err := s.cf.CreateTunnelToken(ctx, setupState.CloudflareAPIToken, accountID, tunnel.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create tunnel token"))
	}

	tunnelMeta := db.MachineTunnel{
		MachineID:   machineID,
		AccountID:   accountID,
		TunnelID:    tunnel.ID,
		TunnelName:  tunnel.Name,
		TunnelToken: tunnelToken,
	}
	if err := s.store.UpsertMachineTunnel(ctx, tunnelMeta); err != nil {
		log.Printf("persist machine tunnel failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist tunnel metadata"))
	}

	return connect.NewResponse(&arcav1.CreateMachineTunnelResponse{Tunnel: toMachineTunnelMessage(tunnelMeta)}), nil
}

func (s *tunnelConnectService) UpsertMachineExposure(ctx context.Context, req *connect.Request[arcav1.UpsertMachineExposureRequest]) (*connect.Response[arcav1.UpsertMachineExposureResponse], error) {
	if s.store == nil || s.authenticator == nil || s.cf == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
	}

	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	zoneID := strings.TrimSpace(req.Msg.GetZoneId())
	exposureName := strings.TrimSpace(req.Msg.GetName())
	if machineID == "" || exposureName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id and exposure name are required"))
	}

	if _, err := s.store.GetMachineByIDForUser(ctx, userID, machineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("authorize machine for exposure upsert failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
	}

	setupState, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("load setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if strings.TrimSpace(setupState.CloudflareAPIToken) == "" || strings.TrimSpace(setupState.BaseDomain) == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("setup is not complete"))
	}
	if zoneID == "" {
		zoneID = strings.TrimSpace(setupState.CloudflareZoneID)
	}

	hostname := buildExposureHostname(exposureName, setupState.BaseDomain)
	service := "http://localhost:80"
	exposure, err := s.store.UpsertMachineExposure(ctx, machineID, exposureName, hostname, service, req.Msg.GetPublic())
	if err != nil {
		log.Printf("persist machine exposure failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist exposure metadata"))
	}

	// Cloudflare DNS/ingress updates are best-effort in MVP.
	if zoneID != "" {
		tunnel, tunnelErr := s.store.GetMachineTunnelByMachineID(ctx, machineID)
		if tunnelErr == nil {
			tunnelTarget := tunnel.TunnelID + ".cfargotunnel.com"
			if err := s.cf.UpsertDNSCNAME(ctx, setupState.CloudflareAPIToken, zoneID, hostname, tunnelTarget, true); err != nil {
				log.Printf("upsert dns failed: %v", err)
			} else {
				exposures, err := s.store.ListMachineExposuresByMachineID(ctx, machineID)
				if err == nil {
					ingressRules := make([]cloudflare.IngressRule, 0, len(exposures))
					for _, item := range exposures {
						ingressRules = append(ingressRules, cloudflare.IngressRule{Hostname: item.Hostname, Service: item.Service})
					}
					if err := s.cf.UpdateTunnelIngress(ctx, setupState.CloudflareAPIToken, tunnel.AccountID, tunnel.TunnelID, ingressRules); err != nil {
						log.Printf("update tunnel ingress failed: %v", err)
					}
				}
			}
		} else if !errors.Is(tunnelErr, sql.ErrNoRows) {
			log.Printf("load machine tunnel failed: %v", tunnelErr)
		}
	}

	return connect.NewResponse(&arcav1.UpsertMachineExposureResponse{Exposure: toMachineExposureMessage(exposure)}), nil
}

func (s *tunnelConnectService) ListMachineExposures(ctx context.Context, req *connect.Request[arcav1.ListMachineExposuresRequest]) (*connect.Response[arcav1.ListMachineExposuresResponse], error) {
	if s.store == nil || s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
	}

	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}
	if _, err := s.store.GetMachineByIDForUser(ctx, userID, machineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("authorize machine for exposure list failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
	}

	exposures, err := s.store.ListMachineExposuresByMachineID(ctx, machineID)
	if err != nil {
		log.Printf("list machine exposures failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list exposures"))
	}

	items := make([]*arcav1.MachineExposure, 0, len(exposures))
	for _, exposure := range exposures {
		items = append(items, toMachineExposureMessage(exposure))
	}
	return connect.NewResponse(&arcav1.ListMachineExposuresResponse{Exposures: items}), nil
}

func (s *tunnelConnectService) GetMachineExposureByHostname(ctx context.Context, req *connect.Request[arcav1.GetMachineExposureByHostnameRequest]) (*connect.Response[arcav1.GetMachineExposureByHostnameResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
	}

	hostname := strings.TrimSpace(req.Msg.GetHostname())
	if hostname == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hostname is required"))
	}
	machineToken := strings.TrimSpace(machineTokenFromHeader(req.Header()))
	if machineToken == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine token is required"))
	}
	machineID, err := s.store.GetMachineIDByMachineToken(ctx, machineToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid machine token"))
	}

	exposure, err := s.store.GetMachineExposureByHostname(ctx, hostname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
		}
		log.Printf("get machine exposure failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get exposure"))
	}
	if exposure.MachineID != machineID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
	}

	return connect.NewResponse(&arcav1.GetMachineExposureByHostnameResponse{Exposure: toMachineExposureMessage(exposure)}), nil
}

func toMachineTunnelMessage(tunnel db.MachineTunnel) *arcav1.MachineTunnel {
	return &arcav1.MachineTunnel{
		MachineId:   tunnel.MachineID,
		AccountId:   tunnel.AccountID,
		TunnelId:    tunnel.TunnelID,
		TunnelName:  tunnel.TunnelName,
		TunnelToken: tunnel.TunnelToken,
	}
}

func toMachineExposureMessage(exposure db.MachineExposure) *arcav1.MachineExposure {
	return &arcav1.MachineExposure{
		Id:        exposure.ID,
		MachineId: exposure.MachineID,
		Name:      exposure.Name,
		Hostname:  exposure.Hostname,
		Service:   exposure.Service,
		Public:    exposure.IsPublic,
	}
}

func buildExposureHostname(name, baseDomain string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	baseDomain = strings.ToLower(strings.TrimSpace(baseDomain))
	if strings.Contains(name, ".") {
		return name
	}
	return name + "." + baseDomain
}
