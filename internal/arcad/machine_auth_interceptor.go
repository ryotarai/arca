package arcad

import (
	"context"
	"strings"

	"connectrpc.com/connect"
)

// newMachineAuthInterceptor returns a Connect unary interceptor that attaches
// the machine identity headers (X-Arca-Machine-ID and Authorization: Bearer)
// to every outbound request.
func newMachineAuthInterceptor(machineID, machineToken string) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if req.Spec().IsClient {
				if machineID != "" {
					req.Header().Set("X-Arca-Machine-ID", machineID)
				}
				if strings.TrimSpace(machineToken) != "" {
					req.Header().Set("Authorization", "Bearer "+machineToken)
				}
			}
			return next(ctx, req)
		}
	}
}
