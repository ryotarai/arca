package arcad

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type ReadinessChecker struct {
	startupSentinel string
	tcpEndpoints    []string
}

func NewReadinessChecker(startupSentinel string, tcpEndpoints []string) *ReadinessChecker {
	filtered := make([]string, 0, len(tcpEndpoints))
	for _, endpoint := range tcpEndpoints {
		if trimmed := strings.TrimSpace(endpoint); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return &ReadinessChecker{
		startupSentinel: strings.TrimSpace(startupSentinel),
		tcpEndpoints:    filtered,
	}
}

func (c *ReadinessChecker) Ready(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if c.startupSentinel != "" {
		if _, err := os.Stat(c.startupSentinel); err != nil {
			return fmt.Errorf("startup sentinel not ready: %w", err)
		}
	}
	for _, endpoint := range c.tcpEndpoints {
		dialer := &net.Dialer{Timeout: 1 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", endpoint)
		if err != nil {
			return fmt.Errorf("tcp endpoint %s not ready: %w", endpoint, err)
		}
		_ = conn.Close()
	}
	return nil
}
