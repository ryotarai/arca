package server

import (
	"net/http"
	"strings"
)

func machineTokenFromHeader(header http.Header) string {
	if token := strings.TrimSpace(header.Get("X-Machine-Token")); token != "" {
		return token
	}
	authorization := strings.TrimSpace(header.Get("Authorization"))
	if authorization == "" {
		return ""
	}
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
