package machine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/workflow"
)

var errStartCancelled = errors.New("machine start cancelled")

// ErrImageCreationNotSupported is returned by runtimes that do not support creating images.
var ErrImageCreationNotSupported = errors.New("image creation not supported for this runtime")

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
	CreateImage(ctx context.Context, machine db.Machine, imageName string) (map[string]string, error)
}

type RuntimeStartOptions struct {
	ControlPlaneURL    string
	AuthorizeURL       string
	MachineID          string
	MachineToken       string
	StartupScript      string
	UserStartupScript  string
	BootConfigHash     string
}

// Notifier sends notifications for machine lifecycle events.
type Notifier interface {
	NotifyMachineEvent(ctx context.Context, ownerUserID string, event NotificationEvent)
}

// NotificationEvent describes a machine event for notification dispatch.
type NotificationEvent struct {
	MachineID   string
	MachineName string
	EventType   string
	Message     string
}

type Worker struct {
	store          *db.Store
	runtime        Runtime
	ipCache        *MachineIPCache
	notifier       Notifier
	workerID       string
	pollInterval   time.Duration
	leaseTTL       time.Duration
	reconcileTTL   time.Duration
	startupTTL     time.Duration
	stopTTL        time.Duration
	lastSweep      time.Time
	maxConcurrency int
	sem            chan struct{}
	wg             sync.WaitGroup
}

// SetNotifier sets the notifier used for dispatching machine event notifications.
func (w *Worker) SetNotifier(n Notifier) {
	w.notifier = n
}

const (
	readyPollInterval = 2 * time.Second
	readyStaleAfter   = 30 * time.Second
	maxJobAttempts    = 10
)

func NewWorker(store *db.Store, runtime Runtime, workerID string, ipCache *MachineIPCache, maxConcurrency int, pollInterval time.Duration) *Worker {
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	return &Worker{
		store:          store,
		runtime:        runtime,
		ipCache:        ipCache,
		workerID:       workerID,
		pollInterval:   pollInterval,
		leaseTTL:       30 * time.Second,
		reconcileTTL:   15 * time.Second,
		startupTTL:     4 * time.Minute,
		stopTTL:        10 * time.Minute,
		maxConcurrency: maxConcurrency,
		sem:            make(chan struct{}, maxConcurrency),
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return
		default:
		}

		nowUnix := time.Now().Unix()
		w.maybeSweep(ctx, nowUnix)
		if err := w.store.RecoverExpiredMachineJobs(ctx, nowUnix); err != nil {
			slog.Error("machine worker recover failed", "error", err)
		}

		// Check if we have capacity for more jobs
		select {
		case w.sem <- struct{}{}:
			// Acquired semaphore slot
		default:
			// All slots busy, wait for next tick
			select {
			case <-ctx.Done():
				w.wg.Wait()
				return
			case <-ticker.C:
			}
			continue
		}

		job, ok, err := w.store.ClaimNextMachineJob(ctx, w.workerID, nowUnix+int64(w.leaseTTL.Seconds()), nowUnix)
		if err != nil {
			<-w.sem // release slot
			slog.Error("machine worker claim failed", "error", err)
			select {
			case <-ctx.Done():
				w.wg.Wait()
				return
			case <-ticker.C:
			}
			continue
		}
		if !ok {
			<-w.sem // release slot
			select {
			case <-ctx.Done():
				w.wg.Wait()
				return
			case <-ticker.C:
			}
			continue
		}

		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			defer func() { <-w.sem }()

			hbCtx, hbCancel := context.WithCancel(ctx)
			defer hbCancel()
			go w.runHeartbeat(hbCtx, job.ID)

			w.processJob(ctx, job)
		}()
	}
}

func (w *Worker) runHeartbeat(ctx context.Context, jobID string) {
	interval := w.leaseTTL / 2
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nowUnix := time.Now().Unix()
			leaseUntil := nowUnix + int64(w.leaseTTL.Seconds())
			ok, err := w.store.ExtendMachineJobLease(ctx, jobID, w.workerID, leaseUntil, nowUnix)
			if err != nil {
				slog.Warn("heartbeat extend lease failed", "job_id", jobID, "error", err)
				return
			}
			if !ok {
				slog.Warn("heartbeat lease lost", "job_id", jobID)
				return
			}
		}
	}
}

func (w *Worker) maybeSweep(ctx context.Context, nowUnix int64) {
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
		slog.Error("sweep list failed", "error", err)
		return
	}
	slog.Debug("sweep running", "machine_count", len(machines))

	w.reconcileMachines(ctx, nowUnix, machines)
	w.autoStopMachines(ctx, nowUnix, machines)
}

func (w *Worker) reconcileMachines(ctx context.Context, nowUnix int64, machines []db.Machine) {
	for _, machine := range machines {
		running, containerID, runErr := w.runtime.IsRunning(ctx, machine)
		if runErr != nil {
			slog.Warn("machine reconcile probe failed", "machine_id", machine.ID, "error", runErr)
			continue
		}

		if running {
			// Skip machines in "starting" state — the start job is responsible
			// for transitioning to "running" after readiness is confirmed.
			if machine.Status == db.MachineStatusStarting {
				continue
			}
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

func (w *Worker) autoStopMachines(ctx context.Context, nowUnix int64, machines []db.Machine) {
	for _, machine := range machines {
		if machine.Status != db.MachineStatusRunning {
			slog.Debug("auto-stop skip: not running", "machine_id", machine.ID, "status", machine.Status)
			continue
		}
		if machine.LastActivityAt == 0 {
			slog.Debug("auto-stop skip: no activity recorded", "machine_id", machine.ID)
			continue
		}

		// Read auto_stop_timeout from profile (live) if available, else from frozen config
		var timeout int64
		if profileID := strings.TrimSpace(machine.ProfileID); profileID != "" {
			if p, pErr := w.store.GetMachineProfileByID(ctx, profileID); pErr == nil {
				timeout = db.GetProfileAutoStopTimeoutSeconds(p.ConfigJSON)
			}
		}
		if timeout <= 0 {
			timeout = db.GetProfileAutoStopTimeoutSeconds(machine.InfrastructureConfigJSON)
		}
		if timeout <= 0 {
			slog.Debug("auto-stop skip: no timeout configured", "machine_id", machine.ID, "timeout", timeout)
			continue
		}

		idleDuration := nowUnix - machine.LastActivityAt
		if idleDuration <= timeout {
			slog.Debug("auto-stop skip: not idle enough", "machine_id", machine.ID, "idle_seconds", idleDuration, "timeout_seconds", timeout)
			continue
		}
		slog.Debug("auto-stop triggering", "machine_id", machine.ID, "idle_seconds", idleDuration, "timeout_seconds", timeout)

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
			slog.Error("machine worker panic", "message", message, "stack", string(debug.Stack()))
			w.emitEvent(ctx, job.MachineID, job.ID, "error", "job_panic", message)
			if job.Attempt >= maxJobAttempts {
				w.emitEvent(ctx, job.MachineID, job.ID, "error", "max_retries_exceeded", fmt.Sprintf("job exceeded maximum attempts (%d)", maxJobAttempts))
				_ = w.store.MarkMachineJobFailed(ctx, job.ID, message, nowUnix)
				if job.Kind == db.MachineJobCreateImage {
					_ = w.store.ClearMachineLockedOperation(ctx, job.MachineID)
				}
			} else {
				_ = w.store.RequeueMachineJob(ctx, job.ID, nowUnix+retryDelaySeconds(job.Attempt), message, nowUnix)
			}
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
	case db.MachineJobCreateImage:
		err = w.handleCreateImage(ctx, machine, job)
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
		w.emitEvent(ctx, machine.ID, job.ID, "error", "job_failed", err.Error())

		isTerminal := workflow.IsTerminal(err)

		if isTerminal || job.Attempt >= maxJobAttempts {
			if isTerminal {
				w.emitEvent(ctx, machine.ID, job.ID, "error", "terminal_failure", err.Error())
			} else {
				w.emitEvent(ctx, machine.ID, job.ID, "error", "max_retries_exceeded", fmt.Sprintf("job exceeded maximum attempts (%d)", maxJobAttempts))
			}
			_ = w.store.MarkMachineJobFailed(ctx, job.ID, err.Error(), nowUnix)
			if job.Kind == db.MachineJobCreateImage {
				_ = w.store.ClearMachineLockedOperation(ctx, machine.ID)
				// Best-effort restart: the machine may have been stopped
				// during imaging. EnsureRunning is idempotent — if the
				// machine is already running this is a no-op.
				w.bestEffortRestartMachine(ctx, machine)
			}
		} else {
			nextRunAt := nowUnix + retryDelaySeconds(job.Attempt)
			w.emitEvent(ctx, machine.ID, job.ID, "info", "retry_scheduled", fmt.Sprintf("retry scheduled at unix=%d", nextRunAt))
			_ = w.store.RequeueMachineJob(ctx, job.ID, nextRunAt, err.Error(), nowUnix)
		}
		return
	}

	w.emitEvent(ctx, machine.ID, job.ID, "info", "job_succeeded", "job completed")
	if err := w.store.MarkMachineJobSucceeded(ctx, job.ID, nowUnix); err != nil {
		slog.Error("machine worker mark success failed", "error", err)
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

	startOpts := w.buildStartOptions(ctx, machine)
	if startOpts.ControlPlaneURL == "" {
		return fmt.Errorf("server domain is not configured")
	}

	startCtx, startCancel := context.WithTimeout(ctx, w.startupTTL)
	defer startCancel()
	containerID, err := w.runtime.EnsureRunning(startCtx, machine, startOpts)
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

	// Update applied_boot_config_hash from the profile used for this start
	if startOpts.BootConfigHash != "" {
		if err := w.store.UpdateMachineAppliedBootConfigHash(ctx, machine.ID, startOpts.BootConfigHash); err != nil {
			slog.Warn("update applied boot config hash failed", "machine_id", machine.ID, "error", err)
		}
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

// buildStartOptions assembles RuntimeStartOptions from the machine's config and
// setup state. It is used by handleStart and the create-image restart path.
func (w *Worker) buildStartOptions(ctx context.Context, machine db.Machine) RuntimeStartOptions {
	// Fetch the machine's profile for live dynamic settings
	var profile db.MachineProfile
	if profileID := strings.TrimSpace(machine.ProfileID); profileID != "" {
		p, pErr := w.store.GetMachineProfileByID(ctx, profileID)
		if pErr != nil {
			slog.Warn("fetch profile for machine start", "machine_id", machine.ID, "profile_id", profileID, "error", pErr)
		} else {
			profile = p
		}
	}

	setup, err := w.store.GetSetupState(ctx)
	if err != nil {
		slog.Warn("buildStartOptions: load setup state failed", "machine_id", machine.ID, "error", err)
		return RuntimeStartOptions{}
	}
	controlPlaneURL := controlPlaneURLFromSetup(setup)
	// Override control plane URL from profile (live) if available, else from frozen config
	if profile.ConfigJSON != "" {
		if override := db.GetProfileServerAPIURL(profile.ConfigJSON); override != "" {
			controlPlaneURL = override
		}
	} else if override := db.GetProfileServerAPIURL(machine.InfrastructureConfigJSON); override != "" {
		controlPlaneURL = override
	}
	// Compute the authorize URL from the public server domain so that arcad
	// can redirect browsers even when the control-plane URL is an internal IP.
	var authorizeURL string
	if serverDomain := strings.TrimSpace(setup.ServerDomain); serverDomain != "" {
		authorizeURL = "https://" + serverDomain + "/console/authorize"
	}

	// Read startup_script from the live profile (boot setting).
	var profileStartupScript string
	if profile.ConfigJSON != "" {
		profileStartupScript = db.GetProfileStartupScript(profile.ConfigJSON)
	}

	// Fetch the machine owner's startup script.
	var userStartupScript string
	ownerUserID, ownerErr := w.store.GetMachineOwnerUserID(ctx, machine.ID)
	if ownerErr == nil {
		userStartupScript, _ = w.store.GetUserStartupScript(ctx, ownerUserID)
	}

	return RuntimeStartOptions{
		ControlPlaneURL:   controlPlaneURL,
		AuthorizeURL:      authorizeURL,
		MachineID:         machine.ID,
		MachineToken:      machine.MachineToken,
		StartupScript:     profileStartupScript,
		UserStartupScript: userStartupScript,
		BootConfigHash:    profile.BootConfigHash,
	}
}

func controlPlaneURLFromSetup(setup db.SetupState) string {
	if serverDomain := strings.TrimSpace(setup.ServerDomain); serverDomain != "" {
		return "https://" + serverDomain
	}
	return ""
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

// notifyEventTypes lists the event types that trigger Slack notifications.
var notifyEventTypes = map[string]bool{
	"ready":       true,
	"auto_stop":   true,
	"job_failed":  true,
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

	if w.notifier != nil && notifyEventTypes[eventType] {
		go w.dispatchNotification(machineID, eventType, message)
	}
}

func (w *Worker) dispatchNotification(machineID, eventType, message string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("notification dispatch panicked", "machine_id", machineID, "error", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ownerUserID, err := w.store.GetMachineOwnerUserID(ctx, machineID)
	if err != nil {
		slog.Debug("notification: could not find machine owner", "machine_id", machineID, "error", err)
		return
	}

	machineName := machineID
	if m, err := w.store.GetMachineByID(ctx, machineID); err == nil {
		machineName = m.Name
	}

	w.notifier.NotifyMachineEvent(ctx, ownerUserID, NotificationEvent{
		MachineID:   machineID,
		MachineName: machineName,
		EventType:   eventType,
		Message:     message,
	})
}

// bestEffortRestartMachine attempts to restart a machine after a terminal job
// failure. If the restart fails, the reconcile loop will eventually pick it up
// (locked_operation is already cleared at this point).
func (w *Worker) bestEffortRestartMachine(ctx context.Context, machine db.Machine) {
	startOpts := w.buildStartOptions(ctx, machine)
	restartCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if _, err := w.runtime.EnsureRunning(restartCtx, machine, startOpts); err != nil {
		slog.Warn("best-effort restart failed after terminal job failure", "machine_id", machine.ID, "error", err)
	}
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
