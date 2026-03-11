package machine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math"
	"runtime/debug"
	"strings"
	"time"
	"unicode"

	"github.com/ryotarai/arca/internal/cloudflare"
	"github.com/ryotarai/arca/internal/db"
)

var errStartCancelled = errors.New("machine start cancelled")

type RuntimeMachineInfo struct {
	PrivateIP string
	PublicIP  string
}

type Runtime interface {
	EnsureRunning(context.Context, db.Machine, RuntimeStartOptions) (string, error)
	EnsureStopped(context.Context, db.Machine) error
	EnsureDeleted(context.Context, db.Machine) error
	IsRunning(context.Context, db.Machine) (bool, string, error)
	GetMachineInfo(context.Context, db.Machine) (*RuntimeMachineInfo, error)
}

type RuntimeStartOptions struct {
	TunnelToken           string
	ControlPlaneURL       string
	AuthorizeURL          string
	MachineID             string
	MachineToken          string
	StartupScript         string
	InteractiveSSHPubKeys []string
}

type Worker struct {
	store        *db.Store
	runtime      Runtime
	cfClient     *cloudflare.Client
	ipCache      *MachineIPCache
	workerID     string
	pollInterval time.Duration
	leaseTTL     time.Duration
	reconcileTTL time.Duration
	startupTTL   time.Duration
	stopTTL      time.Duration
	lastSweep    time.Time
}

const (
	deleteTunnelMaxAttempts  = 5
	deleteTunnelRetryBackoff = 3 * time.Second
	readyPollInterval        = 2 * time.Second
	readyStaleAfter          = 30 * time.Second
)

func NewWorker(store *db.Store, runtime Runtime, cfClient *cloudflare.Client, workerID string, ipCache *MachineIPCache) *Worker {
	return &Worker{
		store:        store,
		runtime:      runtime,
		cfClient:     cfClient,
		ipCache:      ipCache,
		workerID:     workerID,
		pollInterval: 2 * time.Second,
		leaseTTL:     30 * time.Second,
		reconcileTTL: 15 * time.Second,
		startupTTL:   4 * time.Minute,
		stopTTL:      90 * time.Second,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		nowUnix := time.Now().Unix()
		w.maybeReconcile(ctx, nowUnix)
		w.maybeAutoStop(ctx, nowUnix)
		if err := w.store.RecoverExpiredMachineJobs(ctx, nowUnix); err != nil {
			log.Printf("machine worker recover failed: %v", err)
		}

		job, ok, err := w.store.ClaimNextMachineJob(ctx, w.workerID, nowUnix+int64(w.leaseTTL.Seconds()), nowUnix)
		if err != nil {
			log.Printf("machine worker claim failed: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}

		w.processJob(ctx, job)
	}
}

func (w *Worker) maybeReconcile(ctx context.Context, nowUnix int64) {
	if w.store == nil || w.runtime == nil {
		return
	}
	now := time.Now()
	if !w.lastSweep.IsZero() && now.Sub(w.lastSweep) < w.reconcileTTL {
		return
	}
	w.lastSweep = now

	machines, err := w.store.ListMachinesByDesiredStatus(ctx, db.MachineDesiredRunning, 200)
	if err != nil {
		slog.Error("machine reconcile list failed", "error", err)
		return
	}

	for _, machine := range machines {
		running, containerID, runErr := w.runtime.IsRunning(ctx, machine)
		if runErr != nil {
			slog.Warn("machine reconcile probe failed", "machine_id", machine.ID, "error", runErr)
			continue
		}

		if running {
			if machine.Status != db.MachineStatusRunning || machine.ContainerID != containerID {
				if err := w.store.UpdateMachineRuntimeStateByMachineID(ctx, machine.ID, db.MachineStatusRunning, db.MachineDesiredRunning, containerID, ""); err != nil {
					slog.Warn("machine reconcile runtime state update failed", "machine_id", machine.ID, "error", err)
				}
			}
			continue
		}

		active, err := w.store.HasActiveStartOrReconcileJob(ctx, machine.ID)
		if err != nil {
			slog.Warn("machine reconcile active-job check failed", "machine_id", machine.ID, "error", err)
			continue
		}
		if active {
			continue
		}

		if machine.Status == db.MachineStatusRunning {
			if err := w.store.UpdateMachineRuntimeStateByMachineID(ctx, machine.ID, db.MachineStatusPending, db.MachineDesiredRunning, containerID, "container is not running; reconcile scheduled"); err != nil {
				slog.Warn("machine reconcile pending-state update failed", "machine_id", machine.ID, "error", err)
			}
		}
		if err := w.store.EnqueueReconcileMachineJob(ctx, machine.ID, nowUnix); err != nil {
			slog.Warn("machine reconcile enqueue failed", "machine_id", machine.ID, "error", err)
			continue
		}
		w.emitEvent(ctx, machine.ID, "", "warn", "reconcile_scheduled", "container is not running; reconcile job enqueued")
		slog.Info("machine reconcile job enqueued", "machine_id", machine.ID)
	}
}

func (w *Worker) maybeAutoStop(ctx context.Context, nowUnix int64) {
	if w.store == nil || w.runtime == nil {
		return
	}
	now := time.Now()
	// Piggyback on the same cadence as reconcile (reuse reconcileTTL interval)
	if !w.lastSweep.IsZero() && now.Sub(w.lastSweep) < w.reconcileTTL {
		return
	}

	machines, err := w.store.ListMachinesByDesiredStatus(ctx, db.MachineDesiredRunning, 200)
	if err != nil {
		slog.Error("auto-stop list failed", "error", err)
		return
	}

	// Cache runtime configs per runtime_id to avoid N+1 queries
	runtimeConfigs := make(map[string]string) // runtime_id -> config_json

	for _, machine := range machines {
		if machine.Status != db.MachineStatusRunning {
			continue
		}
		if machine.LastActivityAt == 0 {
			continue
		}

		configJSON, ok := runtimeConfigs[machine.RuntimeID]
		if !ok {
			rt, rtErr := w.store.GetRuntimeByID(ctx, machine.RuntimeID)
			if rtErr != nil {
				slog.Warn("auto-stop runtime lookup failed", "machine_id", machine.ID, "runtime_id", machine.RuntimeID, "error", rtErr)
				continue
			}
			configJSON = rt.ConfigJSON
			runtimeConfigs[machine.RuntimeID] = configJSON
		}

		timeout := db.GetRuntimeAutoStopTimeoutSeconds(configJSON)
		if timeout <= 0 {
			continue
		}

		idleDuration := nowUnix - machine.LastActivityAt
		if idleDuration <= timeout {
			continue
		}

		stopped, stopErr := w.store.RequestSystemStopMachine(ctx, machine.ID)
		if stopErr != nil {
			slog.Warn("auto-stop request failed", "machine_id", machine.ID, "error", stopErr)
			continue
		}
		if stopped {
			idleMinutes := idleDuration / 60
			w.emitEvent(ctx, machine.ID, "", "info", "auto_stop", fmt.Sprintf("machine auto-stopped after %d minutes idle", idleMinutes))
			slog.Info("machine auto-stopped", "machine_id", machine.ID, "idle_minutes", idleMinutes)
		}
	}
}

func (w *Worker) processJob(ctx context.Context, job db.MachineJob) {
	nowUnix := time.Now().Unix()
	w.emitEvent(ctx, job.MachineID, job.ID, "info", "job_started", "processing "+job.Kind+" job")

	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("panic: %v", recovered)
			log.Printf("machine worker panic: %s\n%s", message, string(debug.Stack()))
			w.emitEvent(ctx, job.MachineID, job.ID, "error", "job_panic", message)
			_ = w.store.RequeueMachineJob(ctx, job.ID, nowUnix+retryDelaySeconds(job.Attempt), message, nowUnix)
		}
	}()

	machine, err := w.store.GetMachineByID(ctx, job.MachineID)
	if err != nil {
		if err == sql.ErrNoRows {
			w.emitEvent(ctx, job.MachineID, job.ID, "warn", "machine_missing", "machine no longer exists; marking job succeeded")
			_ = w.store.MarkMachineJobSucceeded(ctx, job.ID, nowUnix)
			return
		}
		w.emitEvent(ctx, job.MachineID, job.ID, "error", "load_machine_failed", err.Error())
		_ = w.store.RequeueMachineJob(ctx, job.ID, nowUnix+retryDelaySeconds(job.Attempt), err.Error(), nowUnix)
		return
	}

	switch job.Kind {
	case db.MachineJobStart, db.MachineJobReconcile:
		err = w.handleStart(ctx, machine, job.ID)
	case db.MachineJobStop:
		err = w.handleStop(ctx, machine, job.ID)
	case db.MachineJobDelete:
		err = w.handleDelete(ctx, machine, job.ID)
	default:
		err = fmt.Errorf("unknown machine job kind: %s", job.Kind)
	}

	if err != nil {
		slog.Error(
			"machine job failed",
			"worker_id", w.workerID,
			"job_id", job.ID,
			"machine_id", machine.ID,
			"job_kind", job.Kind,
			"attempt", job.Attempt,
			"error", err,
		)
		_ = w.store.UpdateMachineRuntimeStateByMachineID(ctx, machine.ID, db.MachineStatusFailed, machine.DesiredStatus, machine.ContainerID, err.Error())
		nextRunAt := nowUnix + retryDelaySeconds(job.Attempt)
		w.emitEvent(ctx, machine.ID, job.ID, "error", "job_failed", err.Error())
		w.emitEvent(ctx, machine.ID, job.ID, "info", "retry_scheduled", fmt.Sprintf("retry scheduled at unix=%d", nextRunAt))
		_ = w.store.RequeueMachineJob(ctx, job.ID, nextRunAt, err.Error(), nowUnix)
		return
	}

	w.emitEvent(ctx, machine.ID, job.ID, "info", "job_succeeded", "job completed")
	if err := w.store.MarkMachineJobSucceeded(ctx, job.ID, nowUnix); err != nil {
		log.Printf("machine worker mark success failed: %v", err)
	}
}

func (w *Worker) handleStart(ctx context.Context, machine db.Machine, jobID string) error {
	if machine.DesiredStatus == db.MachineDesiredDeleted {
		return w.handleDelete(ctx, machine, jobID)
	}
	if machine.DesiredStatus == db.MachineDesiredStopped {
		return w.handleStop(ctx, machine, jobID)
	}

	if err := w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusStarting,
		db.MachineDesiredRunning,
		machine.ContainerID,
		"",
	); err != nil {
		return err
	}
	w.emitEvent(ctx, machine.ID, jobID, "info", "runtime_starting", "starting machine runtime")

	exposureMethod := w.getMachineExposureMethod(ctx, machine)
	var tunnelToken string
	if exposureMethod == db.MachineExposureMethodCloudflareTunnel {
		var tunnelErr error
		tunnelToken, tunnelErr = w.ensureMachineTunnel(ctx, machine)
		if tunnelErr != nil {
			return tunnelErr
		}
		w.emitEvent(ctx, machine.ID, jobID, "info", "tunnel_ready", "machine tunnel is ready")
	} else {
		// proxy-via-server: register exposure without Cloudflare tunnel
		if err := w.ensureMachineExposureProxyViaServer(ctx, machine); err != nil {
			return err
		}
		w.emitEvent(ctx, machine.ID, jobID, "info", "exposure_ready", "machine exposure registered (proxy via server)")
	}

	setup, err := w.store.GetSetupState(ctx)
	if err != nil {
		return fmt.Errorf("load setup state: %w", err)
	}
	controlPlaneURL, err := controlPlaneURLForMachine(setup)
	if err != nil {
		return err
	}
	// Override control plane URL if the runtime has a server_api_url configured
	if runtimeCatalog, rtErr := w.store.GetRuntimeByID(ctx, machine.RuntimeID); rtErr == nil {
		if override := db.GetRuntimeServerAPIURL(runtimeCatalog.ConfigJSON); override != "" {
			controlPlaneURL = override
		}
	}
	ownerUserID, err := w.store.GetMachineOwnerUserID(ctx, machine.ID)
	if err != nil {
		return fmt.Errorf("load machine owner: %w", err)
	}
	userSettings, err := w.store.GetUserSettingsByUserID(ctx, ownerUserID)
	if err != nil {
		return fmt.Errorf("load user settings: %w", err)
	}

	// Compute the authorize URL from the public server domain so that arcad
	// can redirect browsers even when the control-plane URL is an internal IP.
	var authorizeURL string
	if serverDomain := strings.TrimSpace(setup.ServerDomain); serverDomain != "" {
		authorizeURL = "https://" + serverDomain + "/console/authorize"
	}

	containerID, err := w.runtime.EnsureRunning(ctx, machine, RuntimeStartOptions{
		TunnelToken:           tunnelToken,
		ControlPlaneURL:       controlPlaneURL,
		AuthorizeURL:          authorizeURL,
		MachineID:             machine.ID,
		MachineToken:          machine.MachineToken,
		InteractiveSSHPubKeys: userSettings.SSHPublicKeys,
	})
	if err != nil {
		return err
	}
	if err := w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusStarting,
		db.MachineDesiredRunning,
		containerID,
		"",
	); err != nil {
		return err
	}

	// Invalidate IP cache so next proxy request fetches fresh IPs
	if w.ipCache != nil {
		w.ipCache.Invalidate(machine.ID)
	}

	w.emitEvent(ctx, machine.ID, jobID, "info", "waiting_ready", "waiting for machine readiness")
	readyCtx, cancel := context.WithTimeout(ctx, w.startupTTL)
	defer cancel()
	if err := w.waitMachineReady(readyCtx, machine.ID); err != nil {
		if errors.Is(err, errStartCancelled) {
			w.emitEvent(ctx, machine.ID, jobID, "info", "ready_wait_cancelled", "machine desired state changed while waiting for readiness")
			return nil
		}
		return fmt.Errorf("wait machine ready: %w", err)
	}

	if err := w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusRunning,
		db.MachineDesiredRunning,
		containerID,
		"",
	); err != nil {
		return err
	}

	// Initialize last_activity_at so idle timer starts from readiness, not epoch 0
	if err := w.store.UpdateMachineLastActivityAt(ctx, machine.ID); err != nil {
		slog.Warn("update machine last activity on start failed", "machine_id", machine.ID, "error", err)
	}

	w.emitEvent(ctx, machine.ID, jobID, "info", "ready", "machine is ready")
	return nil
}

func (w *Worker) waitMachineReady(ctx context.Context, machineID string) error {
	ticker := time.NewTicker(readyPollInterval)
	defer ticker.Stop()

	var lastState db.MachineReadiness
	var hasState bool

	for {
		readiness, err := w.store.GetMachineReadinessByMachineID(ctx, machineID)
		if err != nil {
			return err
		}
		lastState = readiness
		hasState = true
		if readiness.DesiredStatus != db.MachineDesiredRunning {
			return fmt.Errorf("%w: desired status changed to %q", errStartCancelled, readiness.DesiredStatus)
		}
		if readiness.Ready && readiness.ReadyReportedAt > 0 {
			reportedAt := time.Unix(readiness.ReadyReportedAt, 0)
			if time.Since(reportedAt) <= readyStaleAfter {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			if hasState {
				return fmt.Errorf("%w (last readiness: ready=%t desired=%s reported_at=%d)", ctx.Err(), lastState.Ready, lastState.DesiredStatus, lastState.ReadyReportedAt)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) ensureMachineTunnel(ctx context.Context, machine db.Machine) (string, error) {
	if w.store == nil {
		return "", fmt.Errorf("store unavailable")
	}
	if w.cfClient == nil {
		return "", fmt.Errorf("cloudflare client unavailable")
	}

	slog.Info(
		"machine tunnel provisioning started",
		"machine_id", machine.ID,
		"machine_name", machine.Name,
	)

	runtimeCatalog, err := w.store.GetRuntimeByID(ctx, machine.RuntimeID)
	if err != nil {
		return "", fmt.Errorf("load runtime config: %w", err)
	}
	exposureCfg := db.GetRuntimeExposureConfig(runtimeCatalog.ConfigJSON)

	setup, err := w.store.GetSetupState(ctx)
	if err != nil {
		return "", fmt.Errorf("load setup state: %w", err)
	}

	// Resolve Cloudflare credentials: prefer runtime config, fallback to setup state
	cfAPIToken := strings.TrimSpace(exposureCfg.CloudflareAPIToken)
	if cfAPIToken == "" {
		cfAPIToken = strings.TrimSpace(setup.CloudflareAPIToken)
	}
	cfZoneID := strings.TrimSpace(exposureCfg.CloudflareZoneID)
	if cfZoneID == "" {
		cfZoneID = strings.TrimSpace(setup.CloudflareZoneID)
	}
	baseDomain := strings.TrimSpace(exposureCfg.BaseDomain)

	if cfAPIToken == "" {
		return "", fmt.Errorf("cloudflare api token is not configured")
	}
	if cfZoneID == "" {
		return "", fmt.Errorf("cloudflare zone id is not configured")
	}
	if baseDomain == "" {
		return "", fmt.Errorf("base domain is not configured in runtime exposure config")
	}

	zone, err := w.cfClient.GetZone(ctx, cfAPIToken, cfZoneID)
	if err != nil {
		return "", fmt.Errorf("fetch cloudflare zone: %w", err)
	}
	accountID := strings.TrimSpace(zone.Account.ID)
	if accountID == "" {
		return "", fmt.Errorf("cloudflare zone %q does not include account id", cfZoneID)
	}

	tunnel, err := w.store.GetMachineTunnelByMachineID(ctx, machine.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("load machine tunnel: %w", err)
		}

		tunnelName := "arca-machine-" + machine.ID[:12]
		slog.Info(
			"creating cloudflare tunnel",
			"machine_id", machine.ID,
			"account_id", accountID,
			"tunnel_name", tunnelName,
		)
		created, createErr := w.cfClient.CreateTunnel(ctx, cfAPIToken, accountID, tunnelName)
		if createErr != nil {
			var apiErr cloudflare.APIError
			if errors.As(createErr, &apiErr) && apiErr.Code == 1013 {
				slog.Warn(
					"cloudflare tunnel already exists, reusing existing tunnel",
					"machine_id", machine.ID,
					"account_id", accountID,
					"tunnel_name", tunnelName,
				)
				existing, findErr := w.cfClient.GetTunnelByName(ctx, cfAPIToken, accountID, tunnelName)
				if findErr != nil {
					return "", fmt.Errorf("find existing cloudflare tunnel: %w", findErr)
				}
				created = existing
			} else {
				return "", fmt.Errorf("create cloudflare tunnel: %w", createErr)
			}
		}
		tunnelToken, tokenErr := w.cfClient.CreateTunnelToken(ctx, cfAPIToken, accountID, created.ID)
		if tokenErr != nil {
			return "", fmt.Errorf("create cloudflare tunnel token: %w", tokenErr)
		}
		tunnel = db.MachineTunnel{
			MachineID:   machine.ID,
			AccountID:   accountID,
			TunnelID:    created.ID,
			TunnelName:  created.Name,
			TunnelToken: tunnelToken,
		}
		if upsertErr := w.store.UpsertMachineTunnel(ctx, tunnel); upsertErr != nil {
			return "", fmt.Errorf("save machine tunnel: %w", upsertErr)
		}
		slog.Info(
			"cloudflare tunnel prepared",
			"machine_id", machine.ID,
			"tunnel_id", tunnel.TunnelID,
			"tunnel_name", tunnel.TunnelName,
		)
	}

	hostname, err := w.resolveMachineHostname(ctx, machine, exposureCfg.DomainPrefix, baseDomain)
	if err != nil {
		return "", err
	}
	target := tunnel.TunnelID + ".cfargotunnel.com"
	if err := w.cfClient.UpsertDNSCNAME(ctx, cfAPIToken, cfZoneID, hostname, target, true); err != nil {
		return "", fmt.Errorf("upsert machine cname: %w", err)
	}

	ingressRules := []cloudflare.IngressRule{
		{Hostname: hostname, Service: "http://localhost:21030"},
	}
	if err := w.cfClient.UpdateTunnelIngress(ctx, cfAPIToken, tunnel.AccountID, tunnel.TunnelID, ingressRules); err != nil {
		return "", fmt.Errorf("update tunnel ingress: %w", err)
	}

	if _, err := w.store.UpsertMachineExposure(ctx, machine.ID, "default", hostname, "http://localhost:11030"); err != nil {
		return "", fmt.Errorf("upsert machine exposure: %w", err)
	}
	if err := w.store.UpdateMachineEndpointByID(ctx, machine.ID, hostname); err != nil {
		return "", fmt.Errorf("update machine endpoint: %w", err)
	}
	slog.Info(
		"machine tunnel provisioning completed",
		"machine_id", machine.ID,
		"hostname", hostname,
		"target", target,
		"tunnel_id", tunnel.TunnelID,
	)
	return tunnel.TunnelToken, nil
}

func (w *Worker) resolveMachineHostname(ctx context.Context, machine db.Machine, domainPrefix, baseDomain string) (string, error) {
	exposures, err := w.store.ListMachineExposuresByMachineID(ctx, machine.ID)
	if err != nil {
		return "", fmt.Errorf("list machine exposures: %w", err)
	}
	for _, exposure := range exposures {
		if exposure.Name == "default" && strings.TrimSpace(exposure.Hostname) != "" {
			return strings.TrimSpace(exposure.Hostname), nil
		}
	}

	hostname := machineSubdomain(domainPrefix, machine.Name)
	hostname = hostname + "." + strings.TrimSpace(baseDomain)
	return hostname, nil
}

func machineSubdomain(prefix, name string) string {
	prefix = sanitizeSubdomainPart(prefix)
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevDash := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if prevDash || b.Len() == 0 {
			continue
		}
		b.WriteByte('-')
		prevDash = true
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "machine"
	}
	out = strings.Trim(prefix+out, "-")
	if out == "" {
		return "machine"
	}
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
		if out == "" {
			return "machine"
		}
	}
	return out
}

func sanitizeSubdomainPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func controlPlaneURLForMachine(setup db.SetupState) (string, error) {
	if serverDomain := strings.TrimSpace(setup.ServerDomain); serverDomain != "" {
		return "https://" + serverDomain, nil
	}
	return "", fmt.Errorf("server domain is not configured")
}

func (w *Worker) getMachineExposureMethod(ctx context.Context, machine db.Machine) string {
	runtimeCatalog, err := w.store.GetRuntimeByID(ctx, machine.RuntimeID)
	if err != nil {
		return db.MachineExposureMethodCloudflareTunnel // default fallback
	}
	return db.GetRuntimeExposureMethod(runtimeCatalog.ConfigJSON)
}

func (w *Worker) ensureMachineExposureProxyViaServer(ctx context.Context, machine db.Machine) error {
	runtimeCatalog, err := w.store.GetRuntimeByID(ctx, machine.RuntimeID)
	if err != nil {
		return fmt.Errorf("load runtime config: %w", err)
	}
	exposureCfg := db.GetRuntimeExposureConfig(runtimeCatalog.ConfigJSON)

	baseDomain := strings.TrimSpace(exposureCfg.BaseDomain)
	domainPrefix := strings.TrimSpace(exposureCfg.DomainPrefix)
	if baseDomain == "" {
		return fmt.Errorf("machine exposure base domain is not configured in runtime exposure config")
	}

	hostname := machineSubdomain(domainPrefix, machine.Name) + "." + baseDomain

	// Use arcad's listen port (21030) as the service URL so the server
	// proxies to arcad, which handles __arca/* path routing (ttyd, shelley, etc.).
	if _, err := w.store.UpsertMachineExposure(ctx, machine.ID, "default", hostname, "http://localhost:21030"); err != nil {
		return fmt.Errorf("upsert machine exposure: %w", err)
	}
	if err := w.store.UpdateMachineEndpointByID(ctx, machine.ID, hostname); err != nil {
		return fmt.Errorf("update machine endpoint: %w", err)
	}
	slog.Info(
		"machine exposure registered (proxy via server)",
		"machine_id", machine.ID,
		"hostname", hostname,
	)
	return nil
}

func (w *Worker) handleStop(ctx context.Context, machine db.Machine, jobID string) error {
	if w.ipCache != nil {
		w.ipCache.Invalidate(machine.ID)
	}

	if machine.DesiredStatus == db.MachineDesiredDeleted {
		return w.handleDelete(ctx, machine, jobID)
	}

	if err := w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusStopping,
		db.MachineDesiredStopped,
		machine.ContainerID,
		"",
	); err != nil {
		return err
	}
	w.emitEvent(ctx, machine.ID, jobID, "info", "runtime_stopping", "stopping machine runtime")

	stopCtx, cancel := context.WithTimeout(ctx, w.stopTTL)
	defer cancel()
	if err := w.runtime.EnsureStopped(stopCtx, machine); err != nil {
		return err
	}

	if err := w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusStopped,
		db.MachineDesiredStopped,
		"",
		"",
	); err != nil {
		return err
	}
	w.emitEvent(ctx, machine.ID, jobID, "info", "stopped", "machine is stopped")
	return nil
}

func (w *Worker) handleDelete(ctx context.Context, machine db.Machine, jobID string) error {
	if w.ipCache != nil {
		w.ipCache.Invalidate(machine.ID)
	}

	if err := w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusDeleting,
		db.MachineDesiredDeleted,
		machine.ContainerID,
		"",
	); err != nil {
		return err
	}
	w.emitEvent(ctx, machine.ID, jobID, "info", "deleting", "deleting machine resources")

	stopCtx, cancel := context.WithTimeout(ctx, w.stopTTL)
	defer cancel()
	if err := w.runtime.EnsureDeleted(stopCtx, machine); err != nil {
		return err
	}

	exposureMethod := w.getMachineExposureMethod(ctx, machine)
	if exposureMethod == db.MachineExposureMethodCloudflareTunnel {
		if err := w.deleteMachineDNSRecords(ctx, machine.ID); err != nil {
			return err
		}
		if err := w.deleteMachineTunnel(ctx, machine.ID); err != nil {
			return err
		}
	}

	deleted, err := w.store.DeleteMachineByID(ctx, machine.ID)
	if err != nil {
		return err
	}
	if !deleted {
		return nil
	}
	w.emitEvent(ctx, machine.ID, jobID, "info", "deleted", "machine deleted")
	return nil
}

func (w *Worker) deleteMachineDNSRecords(ctx context.Context, machineID string) error {
	if w.cfClient == nil {
		return errors.New("cloudflare client unavailable")
	}

	setup, err := w.store.GetSetupState(ctx)
	if err != nil {
		return err
	}
	apiToken := strings.TrimSpace(setup.CloudflareAPIToken)
	if apiToken == "" {
		return errors.New("cloudflare api token is not configured")
	}
	zoneID := strings.TrimSpace(setup.CloudflareZoneID)
	if zoneID == "" {
		return errors.New("cloudflare zone id is not configured")
	}

	exposures, err := w.store.ListMachineExposuresByMachineID(ctx, machineID)
	if err != nil {
		return err
	}
	for _, exposure := range exposures {
		hostname := strings.TrimSpace(exposure.Hostname)
		if hostname == "" {
			continue
		}
		if err := w.cfClient.DeleteDNSCNAME(ctx, apiToken, zoneID, hostname); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) deleteMachineTunnel(ctx context.Context, machineID string) error {
	tunnel, err := w.store.GetMachineTunnelByMachineID(ctx, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	if w.cfClient == nil {
		return errors.New("cloudflare client unavailable")
	}

	setup, err := w.store.GetSetupState(ctx)
	if err != nil {
		return err
	}
	apiToken := strings.TrimSpace(setup.CloudflareAPIToken)
	if apiToken == "" {
		return errors.New("cloudflare api token is not configured")
	}

	for attempt := 1; attempt <= deleteTunnelMaxAttempts; attempt++ {
		err = w.cfClient.DeleteTunnel(ctx, apiToken, tunnel.AccountID, tunnel.TunnelID)
		if err == nil {
			return nil
		}

		var apiErr cloudflare.APIError
		if !errors.As(err, &apiErr) {
			return err
		}

		msg := strings.ToLower(apiErr.Message)
		if strings.Contains(msg, "not found") || apiErr.Code == 1003 || apiErr.Code == 1033 {
			return nil
		}
		if !isActiveTunnelConnectionError(apiErr) || attempt == deleteTunnelMaxAttempts {
			return err
		}

		if err := sleepContext(ctx, deleteTunnelRetryBackoff); err != nil {
			return err
		}
	}

	return nil
}

func (w *Worker) emitEvent(ctx context.Context, machineID, jobID, level, eventType, message string) {
	if w.store == nil || strings.TrimSpace(machineID) == "" {
		return
	}
	if err := w.store.CreateMachineEvent(ctx, db.MachineEventInput{
		MachineID: strings.TrimSpace(machineID),
		JobID:     strings.TrimSpace(jobID),
		Level:     strings.TrimSpace(level),
		EventType: strings.TrimSpace(eventType),
		Message:   strings.TrimSpace(message),
	}); err != nil {
		slog.Warn("record machine event failed", "machine_id", machineID, "job_id", jobID, "event_type", eventType, "error", err)
	}
}

func isActiveTunnelConnectionError(err cloudflare.APIError) bool {
	if err.Code == 1022 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Message), "active connections")
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelaySeconds(attempt int64) int64 {
	exponent := math.Min(float64(attempt+1), 6)
	return int64(math.Pow(2, exponent))
}
