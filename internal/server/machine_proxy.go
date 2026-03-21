package server

import (
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
// "proxy via server" mode. It resolves hostname→machine via setup_state
// prefix/base_domain configuration, and reverse-proxies the request.
// Authentication is handled by arcad.
type MachineProxyHandler struct {
	store   *db.Store
	ipCache *machine.MachineIPCache
}

func NewMachineProxyHandler(store *db.Store, ipCache *machine.MachineIPCache) *MachineProxyHandler {
	return &MachineProxyHandler{store: store, ipCache: ipCache}
}

// resolveMachineFromHostname looks up a machine by hostname using setup_state
// prefix/base_domain to extract the machine name, then fetches the machine.
func (h *MachineProxyHandler) resolveMachineFromHostname(r *http.Request, hostname string) (db.Machine, bool) {
	setup, err := h.store.GetSetupState(r.Context())
	if err != nil {
		return db.Machine{}, false
	}
	name, ok := db.ExtractMachineNameFromHostname(hostname, setup.DomainPrefix, setup.BaseDomain)
	if !ok {
		return db.Machine{}, false
	}
	m, err := h.store.GetMachineByName(r.Context(), name)
	if err != nil {
		return db.Machine{}, false
	}
	return m, true
}

// IsMachineProxyRequest returns true if the request's Host header matches a
// machine that uses proxy-via-server mode. This is a lightweight check used
// by the timeout middleware to skip the deadline for long-lived WebSocket connections.
func (h *MachineProxyHandler) IsMachineProxyRequest(r *http.Request) bool {
	if h == nil || h.store == nil {
		return false
	}
	hostname := extractHostname(r.Host)
	if hostname == "" {
		return false
	}
	m, ok := h.resolveMachineFromHostname(r, hostname)
	if !ok {
		return false
	}
	return db.GetTemplateExposureMethod(m.TemplateConfigJSON) == db.MachineExposureMethodProxyViaServer
}

// TryServeHTTP attempts to handle the request as a machine proxy request.
// Returns true if the request was handled (the Host matched a machine).
func (h *MachineProxyHandler) TryServeHTTP(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.store == nil {
		return false
	}

	hostname := extractHostname(r.Host)
	if hostname == "" {
		return false
	}

	m, ok := h.resolveMachineFromHostname(r, hostname)
	if !ok {
		return false
	}

	// Check if this machine uses proxy-via-server exposure
	exposureMethod := db.GetTemplateExposureMethod(m.TemplateConfigJSON)
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
	connectivity := db.GetTemplateExposureConfig(m.TemplateConfigJSON).Connectivity
	upstreamURL := resolveUpstreamURL(machineInfo, connectivity)
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

const arcadPort = "21030"

func resolveUpstreamURL(info *machine.RuntimeMachineInfo, connectivity string) string {
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
		return "http://" + net.JoinHostPort(ip, arcadPort)
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
