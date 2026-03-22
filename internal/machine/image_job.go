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
	"github.com/ryotarai/arca/internal/workflow"
)

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

	// Deferred cleanup:
	// - On success: clear locked_operation so other operations can proceed.
	// - On failure: only emit the error event. Do NOT restart the machine
	//   here. processJob handles retry/terminal logic and will restart the
	//   machine on terminal failure. Keeping the machine stopped during
	//   retries avoids the stop→start loop.
	var jobErr error
	defer func() {
		if jobErr != nil {
			w.emitEvent(ctx, machine.ID, job.ID, "error", "imaging_failed",
				fmt.Sprintf("Image creation failed: %v", jobErr))
		} else {
			if clearErr := w.store.ClearMachineLockedOperation(ctx, machine.ID); clearErr != nil {
				slog.Error("failed to clear locked_operation", "machine_id", machine.ID, "error", clearErr)
			}
		}
	}()

	// Checkpoint callback: persist the next step to job metadata so retries
	// resume from the right point instead of re-executing completed steps.
	checkpoint := func(nextStep string) {
		meta.Step = nextStep
		metaBytes, _ := json.Marshal(meta)
		if err := w.store.UpdateMachineJobMetadataJSON(ctx, job.ID, string(metaBytes)); err != nil {
			slog.Warn("failed to checkpoint image job", "job_id", job.ID, "error", err)
		}
	}

	w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_started",
		fmt.Sprintf("Image creation started (step: %s, attempt: %d)", meta.Step, job.Attempt))

	jobErr = workflow.New(checkpoint).
		Step("prepare", func(sCtx context.Context) error {
			if err := w.callArcadPrepareForImage(sCtx, machine); err != nil {
				return err
			}
			w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_prepared", "Machine state cleaned for imaging")
			return nil
		}).
		StepWithTimeout("stop", w.stopTTL, func(sCtx context.Context) error {
			if err := w.runtime.EnsureStopped(sCtx, machine); err != nil {
				return err
			}
			w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_stopped", "Machine stopped")
			return nil
		}).
		StepWithTimeout("snapshot", 10*time.Minute, func(sCtx context.Context) error {
			imageRef, err := w.runtime.CreateImage(sCtx, machine, meta.ImageName)
			if err != nil {
				return err
			}
			dataJSON, _ := json.Marshal(imageRef)
			meta.ImageData = string(dataJSON)
			w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_snapshot_created", "Snapshot created")
			return nil
		}).
		Step("save", func(sCtx context.Context) error {
			customImage, err := w.store.CreateCustomImageFromMachine(sCtx,
				meta.ImageName, machine.ProviderType, meta.ImageData,
				job.Description, machine.ID, machine.ProfileID)
			if err != nil {
				return err
			}
			meta.CustomImageID = customImage.ID
			return nil
		}).
		StepWithTimeout("restart", 4*time.Minute, func(sCtx context.Context) error {
			w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_restarting", "Restarting machine")
			startOpts := w.buildStartOptions(ctx, machine)
			if _, restartErr := w.runtime.EnsureRunning(sCtx, machine, startOpts); restartErr != nil {
				w.emitEvent(ctx, machine.ID, job.ID, "warn", "imaging_restart_failed",
					fmt.Sprintf("Failed to restart: %v", restartErr))
			}
			return nil
		}).
		Run(ctx, meta.Step)

	if jobErr == nil {
		w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_completed",
			fmt.Sprintf("Image '%s' created successfully", meta.ImageName))
	}
	return jobErr
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
		return workflow.Terminal(
			fmt.Errorf("arcad does not support image creation (404); update arcad on the machine"),
		)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("arcad prepare-for-image failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
