package machine

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
}

type RuntimeStartOptions struct {
	TunnelToken string
}

type Worker struct {
	store        *db.Store
	runtime      Runtime
	cfClient     *cloudflare.Client
	workerID     string
	pollInterval time.Duration
	leaseTTL     time.Duration
}

func NewWorker(store *db.Store, runtime Runtime, cfClient *cloudflare.Client, workerID string) *Worker {
	return &Worker{
		store:        store,
		runtime:      runtime,
		cfClient:     cfClient,
		workerID:     workerID,
		pollInterval: 2 * time.Second,
		leaseTTL:     30 * time.Second,
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
	default:
		err = fmt.Errorf("unknown machine job kind: %s", job.Kind)
	}

	if err != nil {
		_ = w.store.UpdateMachineRuntimeStateByMachineID(ctx, machine.ID, db.MachineStatusFailed, machine.DesiredStatus, machine.ContainerID, err.Error())
		_ = w.store.RequeueMachineJob(ctx, job.ID, nowUnix+retryDelaySeconds(job.Attempt), err.Error(), nowUnix)
		return
	}

	if err := w.store.MarkMachineJobSucceeded(ctx, job.ID, nowUnix); err != nil {
		log.Printf("machine worker mark success failed: %v", err)
	}
}

func (w *Worker) handleStart(ctx context.Context, machine db.Machine) error {
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

	containerID, err := w.runtime.EnsureRunning(ctx, machine, RuntimeStartOptions{TunnelToken: tunnelToken})
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
		created, createErr := w.cfClient.CreateTunnel(ctx, setup.CloudflareAPIToken, accountID, tunnelName)
		if createErr != nil {
			return "", fmt.Errorf("create cloudflare tunnel: %w", createErr)
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
	}

	hostname := machineSubdomain(machine.Name)
	hostname = hostname + "." + strings.TrimSpace(setup.BaseDomain)
	target := tunnel.TunnelID + ".cfargotunnel.com"
	if err := w.cfClient.UpsertDNSCNAME(ctx, setup.CloudflareAPIToken, setup.CloudflareZoneID, hostname, target, true); err != nil {
		return "", fmt.Errorf("upsert machine cname: %w", err)
	}

	ingressRules := []cloudflare.IngressRule{
		{Hostname: hostname, Service: "http://localhost:8080"},
	}
	if err := w.cfClient.UpdateTunnelIngress(ctx, setup.CloudflareAPIToken, tunnel.AccountID, tunnel.TunnelID, ingressRules); err != nil {
		return "", fmt.Errorf("update tunnel ingress: %w", err)
	}

	_, _ = w.store.UpsertMachineExposure(ctx, machine.ID, "default", hostname, "http://localhost:8080", true)
	return tunnel.TunnelToken, nil
}

func machineSubdomain(name string) string {
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
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
		if out == "" {
			return "machine"
		}
	}
	return out
}

func (w *Worker) handleStop(ctx context.Context, machine db.Machine) error {
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

func retryDelaySeconds(attempt int64) int64 {
	exponent := math.Min(float64(attempt+1), 6)
	return int64(math.Pow(2, exponent))
}
