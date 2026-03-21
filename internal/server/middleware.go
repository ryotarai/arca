package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// RequestIDToContext extracts chi's request ID and stores it in the context
// so that slog handlers can include it automatically.
func RequestIDToContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := middleware.GetReqID(r.Context())
		if reqID != "" {
			ctx := context.WithValue(r.Context(), requestIDKey, reqID)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// LoggerFromContext returns a slog.Logger with the request ID from context.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	logger := slog.Default()
	if reqID, ok := ctx.Value(requestIDKey).(string); ok && reqID != "" {
		logger = logger.With("request_id", reqID)
	}
	return logger
}
