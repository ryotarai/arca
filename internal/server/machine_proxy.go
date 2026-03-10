package server

import (
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/machine"
)

// MachineProxyHandler handles HTTP requests for machines exposed via
// "proxy via server" mode. It looks up the machine exposure by hostname
// (from the Host header), resolves the machine's upstream address, and
// reverse-proxies the request. Authentication is handled by arcad.
type MachineProxyHandler struct {
	store   *db.Store
	ipCache *machine.MachineIPCache
}

func NewMachineProxyHandler(store *db.Store, ipCache *machine.MachineIPCache) *MachineProxyHandler {
	return &MachineProxyHandler{store: store, ipCache: ipCache}
}

// TryServeHTTP attempts to handle the request as a machine proxy request.
// Returns true if the request was handled (the Host matched a machine exposure).
func (h *MachineProxyHandler) TryServeHTTP(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.store == nil {
		return false
	}

	hostname := extractHostname(r.Host)
	if hostname == "" {
		return false
	}

	exposure, err := h.store.GetMachineExposureByHostname(r.Context(), hostname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		log.Printf("machine proxy: lookup exposure by hostname %q failed: %v", hostname, err)
		return false
	}

	// Check if this machine uses proxy-via-server exposure
	m, err := h.store.GetMachineByID(r.Context(), exposure.MachineID)
	if err != nil {
		log.Printf("machine proxy: lookup machine %q failed: %v", exposure.MachineID, err)
		http.Error(w, "machine not found", http.StatusBadGateway)
		return true
	}

	runtimeCatalog, err := h.store.GetRuntimeByID(r.Context(), m.RuntimeID)
	if err != nil {
		log.Printf("machine proxy: lookup runtime %q failed: %v", m.RuntimeID, err)
		http.Error(w, "runtime not found", http.StatusBadGateway)
		return true
	}

	exposureMethod := db.GetRuntimeExposureMethod(runtimeCatalog.ConfigJSON)
	if exposureMethod != db.MachineExposureMethodProxyViaServer {
		return false
	}

	// Resolve the upstream URL for the machine via IP cache
	var machineInfo *machine.RuntimeMachineInfo
	if h.ipCache != nil {
		var infoErr error
		machineInfo, infoErr = h.ipCache.Get(r.Context(), m)
		if infoErr != nil {
			log.Printf("machine proxy: ip cache lookup for %q failed: %v", m.ID, infoErr)
		}
	}
	connectivity := db.GetRuntimeExposureConfig(runtimeCatalog.ConfigJSON).Connectivity
	upstreamURL := resolveUpstreamURL(machineInfo, m, exposure, connectivity)
	if upstreamURL == "" {
		http.Error(w, "machine upstream unavailable", http.StatusBadGateway)
		return true
	}

	target, err := url.Parse(upstreamURL)
	if err != nil {
		log.Printf("machine proxy: parse upstream url %q failed: %v", upstreamURL, err)
		http.Error(w, "invalid upstream", http.StatusBadGateway)
		return true
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			// Preserve the original Host header so arcad can perform
			// hostname-based exposure lookup and authentication.
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
			log.Printf("machine proxy: upstream error for %q: %v", m.ID, proxyErr)
			http.Error(rw, "upstream error", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
	return true
}

func resolveUpstreamURL(info *machine.RuntimeMachineInfo, m db.Machine, exposure db.MachineExposure, connectivity string) string {
	service := strings.TrimSpace(exposure.Service)
	if service == "" {
		service = "http://localhost:11030"
	}

	serviceURL, err := url.Parse(service)
	if err != nil {
		return ""
	}
	port := serviceURL.Port()
	if port == "" {
		port = "11030"
	}

	// Determine the target IP based on the runtime's connectivity setting.
	var ip string
	if info != nil {
		conn := strings.ToLower(strings.TrimSpace(connectivity))
		switch {
		case conn == "public_ip" || strings.HasSuffix(conn, "_public_ip"):
			ip = info.PublicIP
			if ip == "" {
				ip = info.PrivateIP
			}
		default:
			ip = info.PrivateIP
		}
	}

	if ip != "" {
		return "http://" + net.JoinHostPort(ip, port)
	}

	// Legacy fallback: check if endpoint is an IP address.
	if m.Endpoint != "" {
		if parsed := net.ParseIP(m.Endpoint); parsed != nil {
			return "http://" + net.JoinHostPort(m.Endpoint, port)
		}
	}

	return ""
}

func extractHostname(host string) string {
	hostname := strings.TrimSpace(host)
	if hostname == "" {
		return ""
	}
	h, _, err := net.SplitHostPort(hostname)
	if err != nil {
		return strings.ToLower(hostname)
	}
	return strings.ToLower(h)
}
