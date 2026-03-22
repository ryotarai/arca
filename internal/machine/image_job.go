package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

// Image creation step constants. The step field in createImageMetadata tracks
// which step to execute next, enabling reliable retry from the last checkpoint.
// This is a lightweight "durable execution" pattern: each step persists its
// completion to job metadata before advancing, so retries always resume from
// the correct point regardless of process crashes or timeouts.
const (
	imageStepPrepare  = "prepare"  // call arcad prepare-for-image
	imageStepStop     = "stop"     // shut down the machine
	imageStepSnapshot = "snapshot" // create disk image
	imageStepSave     = "save"     // persist custom_images DB record
	imageStepRestart  = "restart"  // bring machine back up
)

// terminalJobError wraps errors that should not be retried by the job system.
// For example, a 404 from arcad means the endpoint doesn't exist and retrying
// will never succeed.
type terminalJobError struct {
	err error
}

func (e *terminalJobError) Error() string { return e.err.Error() }
func (e *terminalJobError) Unwrap() error { return e.err }

type createImageMetadata struct {
	ImageName     string `json:"image_name"`
	CustomImageID string `json:"custom_image_id,omitempty"`
	Step          string `json:"step,omitempty"`
	ImageData     string `json:"image_data,omitempty"`
}

func (w *Worker) handleCreateImage(ctx context.Context, machine db.Machine, job db.MachineJob) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	var meta createImageMetadata
	if err := json.Unmarshal([]byte(job.MetadataJSON), &meta); err != nil {
		return fmt.Errorf("parse job metadata: %w", err)
	}
	// Backward compat: jobs created before step tracking default to prepare.
	if meta.Step == "" {
		meta.Step = imageStepPrepare
	}

	// Validate step to avoid silently skipping all steps on corrupted metadata.
	switch meta.Step {
	case imageStepPrepare, imageStepStop, imageStepSnapshot, imageStepSave, imageStepRestart:
	default:
		return &terminalJobError{err: fmt.Errorf("unknown image creation step: %q", meta.Step)}
	}

	// Deferred cleanup:
	// - On success: clear locked_operation so other operations can proceed.
	// - On failure: only emit the error event. Do NOT restart the machine here.
	//   processJob handles retry/terminal logic and will restart the machine on
	//   terminal failure. Keeping the machine stopped during retries avoids the
	//   stop→start loop that previously cycled on every failed attempt.
	var jobErr error
	defer func() {
		if jobErr != nil {
			w.emitEvent(ctx, machine.ID, job.ID, "error", "imaging_failed",
				fmt.Sprintf("Image creation failed at step %q: %v", meta.Step, jobErr))
		} else {
			if clearErr := w.store.ClearMachineLockedOperation(ctx, machine.ID); clearErr != nil {
				slog.Error("failed to clear locked_operation", "machine_id", machine.ID, "error", clearErr)
			}
		}
	}()

	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_started",
		fmt.Sprintf("Image creation started (step: %s, attempt: %d)", meta.Step, job.Attempt))

	// Step: prepare — call arcad to clean up machine state before imaging.
	if meta.Step == imageStepPrepare {
		if jobErr = w.callArcadPrepareForImage(ctx, machine); jobErr != nil {
			return jobErr
		}
		w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_prepared", "Machine state cleaned for imaging")
		meta.Step = imageStepStop
		w.updateImageJobMeta(ctx, job.ID, meta)
	}

	// Step: stop — shut down the machine for a clean disk snapshot.
	if meta.Step == imageStepStop {
		stopCtx, stopCancel := context.WithTimeout(ctx, w.stopTTL)
		defer stopCancel()
		if jobErr = w.runtime.EnsureStopped(stopCtx, machine); jobErr != nil {
			return jobErr
		}
		w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_stopped", "Machine stopped")
		meta.Step = imageStepSnapshot
		w.updateImageJobMeta(ctx, job.ID, meta)
	}

	// Step: snapshot — create the image from the stopped machine's disk.
	if meta.Step == imageStepSnapshot {
		snapshotCtx, snapshotCancel := context.WithTimeout(ctx, 10*time.Minute)
		defer snapshotCancel()
		imageRef, err := w.runtime.CreateImage(snapshotCtx, machine, meta.ImageName)
		if err != nil {
			jobErr = err
			return jobErr
		}
		w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_snapshot_created", "Snapshot created")
		dataJSON, _ := json.Marshal(imageRef)
		meta.ImageData = string(dataJSON)
		meta.Step = imageStepSave
		w.updateImageJobMeta(ctx, job.ID, meta)
	}

	// Step: save — persist the custom image record in the database.
	if meta.Step == imageStepSave {
		customImage, err := w.store.CreateCustomImageFromMachine(ctx,
			meta.ImageName, machine.ProviderType, meta.ImageData,
			job.Description, machine.ID, machine.ProfileID)
		if err != nil {
			jobErr = err
			return jobErr
		}
		meta.CustomImageID = customImage.ID
		meta.Step = imageStepRestart
		w.updateImageJobMeta(ctx, job.ID, meta)
	}

	// Step: restart — bring the machine back up. Failure is non-fatal
	// because the image was already created successfully.
	if meta.Step == imageStepRestart {
		w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_restarting", "Restarting machine")
		startCtx, startCancel := context.WithTimeout(ctx, 4*time.Minute)
		defer startCancel()
		startOpts := w.buildStartOptions(ctx, machine)
		if _, restartErr := w.runtime.EnsureRunning(startCtx, machine, startOpts); restartErr != nil {
			w.emitEvent(ctx, machine.ID, job.ID, "warn", "imaging_restart_failed",
				fmt.Sprintf("Failed to restart: %v", restartErr))
		}
	}

	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_completed",
		fmt.Sprintf("Image '%s' created successfully", meta.ImageName))
	return nil
}

// updateImageJobMeta persists the current step and intermediate data to job
// metadata. This is best-effort: if it fails, the worst case is that the
// current step re-executes on retry (each step is idempotent).
func (w *Worker) updateImageJobMeta(ctx context.Context, jobID string, meta createImageMetadata) {
	metaBytes, _ := json.Marshal(meta)
	if err := w.store.UpdateMachineJobMetadataJSON(ctx, jobID, string(metaBytes)); err != nil {
		slog.Warn("failed to update image job metadata", "job_id", jobID, "error", err)
	}
}

// callArcadPrepareForImage calls the arcad prepare-for-image HTTP endpoint
// running inside the machine.
func (w *Worker) callArcadPrepareForImage(ctx context.Context, machine db.Machine) error {
	info, err := w.runtime.GetMachineInfo(ctx, machine)
	if err != nil {
		return fmt.Errorf("get machine info: %w", err)
	}
	if info.PrivateIP == "" {
		return fmt.Errorf("machine has no private IP")
	}

	// arcad listens on port 21032
	url := fmt.Sprintf("http://%s:21032/api/prepare-for-image", info.PrivateIP)

	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+machine.MachineToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call arcad prepare-for-image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &terminalJobError{
			err: fmt.Errorf("arcad does not support image creation (404); update arcad on the machine"),
		}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("arcad prepare-for-image failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
