package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
)

// connectErrorf creates a Connect error with the appropriate code based on the error type.
func connectErrorf(err error, msg string, args ...interface{}) *connect.Error {
	code := classifyError(err)
	return connect.NewError(code, fmt.Errorf(msg, args...))
}

// classifyError determines the appropriate Connect error code for a given error.
func classifyError(err error) connect.Code {
	if err == nil {
		return connect.CodeInternal
	}
	if errors.Is(err, sql.ErrNoRows) {
		return connect.CodeNotFound
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.CodeDeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return connect.CodeCanceled
	}
	// Check for common transient errors
	errMsg := err.Error()
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "i/o timeout") {
		return connect.CodeUnavailable
	}
	return connect.CodeInternal
}
