package machine

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"runtime/debug"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

type Runtime interface {
	EnsureRunning(context.Context, db.Machine) (string, error)
	EnsureStopped(context.Context, db.Machine) error
}

type Worker struct {
	store        *db.Store
	runtime      Runtime
	workerID     string
	pollInterval time.Duration
	leaseTTL     time.Duration
}

func NewWorker(store *db.Store, runtime Runtime, workerID string) *Worker {
	return &Worker{
		store:        store,
		runtime:      runtime,
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

	containerID, err := w.runtime.EnsureRunning(ctx, machine)
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
