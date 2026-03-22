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

type createImageMetadata struct {
	ImageName     string `json:"image_name"`
	CustomImageID string `json:"custom_image_id,omitempty"`
}

func (w *Worker) handleCreateImage(ctx context.Context, machine db.Machine, job db.MachineJob) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	var meta createImageMetadata
	if err := json.Unmarshal([]byte(job.MetadataJSON), &meta); err != nil {
		return fmt.Errorf("parse job metadata: %w", err)
	}

	// Deferred cleanup: on error, attempt restart but do NOT clear lock
	// (processJob will retry the job and the lock must remain held).
	// On success, clear the lock so other operations can proceed.
	var jobErr error
	defer func() {
		if jobErr != nil {
			w.emitEvent(ctx, machine.ID, job.ID, "error", "imaging_failed",
				fmt.Sprintf("Image creation failed: %v", jobErr))
			// Best-effort restart
			startOpts := w.buildStartOptions(ctx, machine)
			restartCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			if _, restartErr := w.runtime.EnsureRunning(restartCtx, machine, startOpts); restartErr != nil {
				w.emitEvent(ctx, machine.ID, job.ID, "warn", "imaging_restart_failed",
					fmt.Sprintf("Failed to restart after imaging failure: %v", restartErr))
			}
			// Do NOT clear lock here - processJob will retry the job
		} else {
			// Success: clear lock
			if clearErr := w.store.ClearMachineLockedOperation(ctx, machine.ID); clearErr != nil {
				slog.Error("failed to clear locked_operation", "machine_id", machine.ID, "error", clearErr)
			}
		}
	}()

	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_started", "Image creation started")

	// Step 1: Call arcad prepare-for-image (skip if machine already stopped on retry)
	if machine.Status == db.MachineStatusRunning {
		if jobErr = w.callArcadPrepareForImage(ctx, machine); jobErr != nil {
			return jobErr
		}
		w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_prepared", "Machine state cleaned for imaging")
	}

	// Step 2: Stop machine
	stopCtx, stopCancel := context.WithTimeout(ctx, 90*time.Second)
	defer stopCancel()
	if jobErr = w.runtime.EnsureStopped(stopCtx, machine); jobErr != nil {
		return jobErr
	}
	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_stopped", "Machine stopped")

	// Step 3: Create image snapshot
	snapshotCtx, snapshotCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer snapshotCancel()
	imageRef, err := w.runtime.CreateImage(snapshotCtx, machine, meta.ImageName)
	if err != nil {
		jobErr = err
		return jobErr
	}
	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_snapshot_created", "Snapshot created")

	// Step 4: Create custom_images record (idempotent)
	dataJSON, _ := json.Marshal(imageRef)
	customImage, err := w.store.CreateCustomImageFromMachine(ctx,
		meta.ImageName, machine.TemplateType, string(dataJSON),
		job.Description, machine.ID, machine.TemplateID)
	if err != nil {
		jobErr = err
		return jobErr
	}

	// Step 5: Update job metadata with custom_image_id
	meta.CustomImageID = customImage.ID
	metaBytes, _ := json.Marshal(meta)
	_ = w.store.UpdateMachineJobMetadataJSON(ctx, job.ID, string(metaBytes))

	// Step 6: Restart machine
	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_restarting", "Restarting machine")
	startCtx, startCancel := context.WithTimeout(ctx, 4*time.Minute)
	defer startCancel()
	startOpts := w.buildStartOptions(ctx, machine)
	if _, restartErr := w.runtime.EnsureRunning(startCtx, machine, startOpts); restartErr != nil {
		w.emitEvent(ctx, machine.ID, job.ID, "warn", "imaging_restart_failed",
			fmt.Sprintf("Failed to restart: %v", restartErr))
	}

	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_completed",
		fmt.Sprintf("Image '%s' created successfully", meta.ImageName))
	return nil // jobErr remains nil → deferred cleanup skips restart
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
		return fmt.Errorf("arcad version does not support image creation (404); update arcad on the machine before retrying")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("arcad prepare-for-image failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
