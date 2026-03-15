package arcad

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/version"
)

type ReadinessReporter struct {
	checker  *ReadinessChecker
	client   ControlPlaneClient
	interval time.Duration
}

func NewReadinessReporter(checker *ReadinessChecker, client ControlPlaneClient, interval time.Duration) *ReadinessReporter {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &ReadinessReporter{
		checker:  checker,
		client:   client,
		interval: interval,
	}
}

func (r *ReadinessReporter) Run(ctx context.Context) {
	if r == nil || r.client == nil {
		return
	}

	reportOnce := func() {
		probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
		err := r.readinessError(probeCtx)
		probeCancel()

		ready := err == nil
		reason := "ready"
		if err != nil {
			reason = strings.TrimSpace(err.Error())
		}

		reportCtx, reportCancel := context.WithTimeout(ctx, 5*time.Second)
		accepted, reportErr := r.client.ReportMachineReadiness(reportCtx, ready, reason, "", version.Version)
		reportCancel()
		if reportErr != nil {
			log.Printf("readiness report failed: %v", reportErr)
			return
		}
		if !accepted {
			log.Printf("readiness report rejected by control plane (ready=%t)", ready)
		}
	}

	reportOnce()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reportOnce()
		}
	}
}

func (r *ReadinessReporter) readinessError(ctx context.Context) error {
	if r == nil || r.checker == nil {
		return nil
	}
	return r.checker.Ready(ctx)
}
