package arcad

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrepareForImageHandler(t *testing.T) {
	cfg := Config{
		MachineToken: "test-token-abc",
	}

	tests := []struct {
		name           string
		method         string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "rejects GET with 405",
			method:         http.MethodGet,
			authHeader:     "Bearer test-token-abc",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "rejects missing authorization with 401",
			method:         http.MethodPost,
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "rejects wrong token with 401",
			method:         http.MethodPost,
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "accepts valid bearer token with 200",
			method:         http.MethodPost,
			authHeader:     "Bearer test-token-abc",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/prepare-for-image", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rr := httptest.NewRecorder()

			handler := PrepareForImageHandler(cfg)
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}
		})
	}
}
