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

type Runtime interface {
	EnsureRunning(context.Context, db.Machine, RuntimeStartOptions) (string, error)
	EnsureStopped(context.Context, db.Machine) error
	IsRunning(context.Context, db.Machine) (bool, string, error)
}

type RuntimeStartOptions struct {
	TunnelToken     string
	ControlPlaneURL string
	MachineID       string
	MachineToken    string
}

type Worker struct {
	store        *db.Store
	runtime      Runtime
	cfClient     *cloudflare.Client
	workerID     string
	pollInterval time.Duration
	leaseTTL     time.Duration
	reconcileTTL time.Duration
	lastSweep    time.Time
}

const (
	deleteTunnelMaxAttempts  = 5
	deleteTunnelRetryBackoff = 3 * time.Second
)

func NewWorker(store *db.Store, runtime Runtime, cfClient *cloudflare.Client, workerID string) *Worker {
	return &Worker{
		store:        store,
		runtime:      runtime,
		cfClient:     cfClient,
		workerID:     workerID,
		pollInterval: 2 * time.Second,
		leaseTTL:     30 * time.Second,
		reconcileTTL: 15 * time.Second,
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
		slog.Info("machine reconcile job enqueued", "machine_id", machine.ID)
	}
}

func (w *Worker) processJob(ctx context.Context, job db.MachineJob) {
	nowUnix := time.Now().Unix()

	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("panic: %v", recovered)
			log.Printf("machine worker panic: %s\n%s", message, string(debug.Stack()))
			_ = w.store.RequeueMachineJob(ctx, job.ID, nowUnix+retryDelaySeconds(job.Attempt), message, nowUnix)
		}
	}()

	machine, err := w.store.GetMachineByID(ctx, job.MachineID)
	if err != nil {
		if err == sql.ErrNoRows {
			_ = w.store.MarkMachineJobSucceeded(ctx, job.ID, nowUnix)
			return
		}
		_ = w.store.RequeueMachineJob(ctx, job.ID, nowUnix+retryDelaySeconds(job.Attempt), err.Error(), nowUnix)
		return
	}

	switch job.Kind {
	case db.MachineJobStart, db.MachineJobReconcile:
		err = w.handleStart(ctx, machine)
	case db.MachineJobStop:
		err = w.handleStop(ctx, machine)
	case db.MachineJobDelete:
		err = w.handleDelete(ctx, machine)
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
		_ = w.store.RequeueMachineJob(ctx, job.ID, nowUnix+retryDelaySeconds(job.Attempt), err.Error(), nowUnix)
		return
	}

	if err := w.store.MarkMachineJobSucceeded(ctx, job.ID, nowUnix); err != nil {
		log.Printf("machine worker mark success failed: %v", err)
	}
}

func (w *Worker) handleStart(ctx context.Context, machine db.Machine) error {
	if machine.DesiredStatus == db.MachineDesiredDeleted {
		return w.handleDelete(ctx, machine)
	}
	if machine.DesiredStatus == db.MachineDesiredStopped {
		return w.handleStop(ctx, machine)
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

	tunnelToken, err := w.ensureMachineTunnel(ctx, machine)
	if err != nil {
		return err
	}

	setup, err := w.store.GetSetupState(ctx)
	if err != nil {
		return fmt.Errorf("load setup state: %w", err)
	}
	controlPlaneURL, err := controlPlaneURLForMachine(setup)
	if err != nil {
		return err
	}

	containerID, err := w.runtime.EnsureRunning(ctx, machine, RuntimeStartOptions{
		TunnelToken:     tunnelToken,
		ControlPlaneURL: controlPlaneURL,
		MachineID:       machine.ID,
		MachineToken:    machine.MachineToken,
	})
	if err != nil {
		return err
	}

	return w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusRunning,
		db.MachineDesiredRunning,
		containerID,
		"",
	)
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

	setup, err := w.store.GetSetupState(ctx)
	if err != nil {
		return "", fmt.Errorf("load setup state: %w", err)
	}
	if strings.TrimSpace(setup.CloudflareAPIToken) == "" {
		return "", fmt.Errorf("cloudflare api token is not configured")
	}
	if strings.TrimSpace(setup.CloudflareZoneID) == "" {
		return "", fmt.Errorf("cloudflare zone id is not configured")
	}
	if strings.TrimSpace(setup.BaseDomain) == "" {
		return "", fmt.Errorf("base domain is not configured")
	}

	zone, err := w.cfClient.GetZone(ctx, setup.CloudflareAPIToken, setup.CloudflareZoneID)
	if err != nil {
		return "", fmt.Errorf("fetch cloudflare zone: %w", err)
	}
	accountID := strings.TrimSpace(zone.Account.ID)
	if accountID == "" {
		return "", fmt.Errorf("cloudflare zone %q does not include account id", setup.CloudflareZoneID)
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
		created, createErr := w.cfClient.CreateTunnel(ctx, setup.CloudflareAPIToken, accountID, tunnelName)
		if createErr != nil {
			var apiErr cloudflare.APIError
			if errors.As(createErr, &apiErr) && apiErr.Code == 1013 {
				slog.Warn(
					"cloudflare tunnel already exists, reusing existing tunnel",
					"machine_id", machine.ID,
					"account_id", accountID,
					"tunnel_name", tunnelName,
				)
				existing, findErr := w.cfClient.GetTunnelByName(ctx, setup.CloudflareAPIToken, accountID, tunnelName)
				if findErr != nil {
					return "", fmt.Errorf("find existing cloudflare tunnel: %w", findErr)
				}
				created = existing
			} else {
				return "", fmt.Errorf("create cloudflare tunnel: %w", createErr)
			}
		}
		tunnelToken, tokenErr := w.cfClient.CreateTunnelToken(ctx, setup.CloudflareAPIToken, accountID, created.ID)
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

	hostname, err := w.resolveMachineHostname(ctx, machine, setup)
	if err != nil {
		return "", err
	}
	target := tunnel.TunnelID + ".cfargotunnel.com"
	if err := w.cfClient.UpsertDNSCNAME(ctx, setup.CloudflareAPIToken, setup.CloudflareZoneID, hostname, target, true); err != nil {
		return "", fmt.Errorf("upsert machine cname: %w", err)
	}

	ingressRules := []cloudflare.IngressRule{
		{Hostname: hostname, Service: "http://localhost:21030"},
	}
	if err := w.cfClient.UpdateTunnelIngress(ctx, setup.CloudflareAPIToken, tunnel.AccountID, tunnel.TunnelID, ingressRules); err != nil {
		return "", fmt.Errorf("update tunnel ingress: %w", err)
	}

	if _, err := w.store.UpsertMachineExposure(ctx, machine.ID, "default", hostname, "http://localhost:8080", false); err != nil {
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

func (w *Worker) resolveMachineHostname(ctx context.Context, machine db.Machine, setup db.SetupState) (string, error) {
	exposures, err := w.store.ListMachineExposuresByMachineID(ctx, machine.ID)
	if err != nil {
		return "", fmt.Errorf("list machine exposures: %w", err)
	}
	for _, exposure := range exposures {
		if exposure.Name == "default" && strings.TrimSpace(exposure.Hostname) != "" {
			return strings.TrimSpace(exposure.Hostname), nil
		}
	}

	hostname := machineSubdomain(setup.DomainPrefix, machine.Name)
	hostname = hostname + "." + strings.TrimSpace(setup.BaseDomain)
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
	baseDomain := strings.TrimSpace(setup.BaseDomain)
	if baseDomain == "" {
		return "", fmt.Errorf("base domain is not configured")
	}
	prefix := sanitizeSubdomainPart(setup.DomainPrefix)
	label := strings.Trim(prefix+"app", "-")
	if label == "" {
		label = "app"
	}
	return "https://" + label + "." + baseDomain, nil
}

func (w *Worker) handleStop(ctx context.Context, machine db.Machine) error {
	if machine.DesiredStatus == db.MachineDesiredDeleted {
		return w.handleDelete(ctx, machine)
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

	if err := w.runtime.EnsureStopped(ctx, machine); err != nil {
		return err
	}

	return w.store.UpdateMachineRuntimeStateByMachineID(
		ctx,
		machine.ID,
		db.MachineStatusStopped,
		db.MachineDesiredStopped,
		"",
		"",
	)
}

func (w *Worker) handleDelete(ctx context.Context, machine db.Machine) error {
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

	if err := w.runtime.EnsureStopped(ctx, machine); err != nil {
		return err
	}

	if err := w.deleteMachineTunnel(ctx, machine.ID); err != nil {
		return err
	}

	_, err := w.store.DeleteMachineByID(ctx, machine.ID)
	return err
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
