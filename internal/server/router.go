package server

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/crypto"
	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/gen/arca/v1/arcav1connect"
	"github.com/ryotarai/arca/internal/machine"
	"github.com/ryotarai/arca/internal/notification"
)

type Dependencies struct {
	HealthChecker    HealthChecker
	Authenticator    Authenticator
	MachineStore     MachineStore
	Store            *db.Store
	MachineProxy     *MachineProxyHandler
	Slack            *notification.SlackService
	Encryptor        *crypto.Encryptor
	LLMTokenExecutor *LLMTokenExecutor
	RateLimiter      *RateLimiter
	MockRuntime      *machine.MockRuntime
}

type HealthChecker interface {
	Ping(context.Context) error
}

type Authenticator interface {
	Register(context.Context, string, string) (string, string, error)
	Login(context.Context, string, string) (string, string, string, string, time.Time, error)
	ListUsers(context.Context) ([]db.ManagedUser, error)
	ProvisionUser(context.Context, string, string) (string, string, string, time.Time, error)
	IssueUserSetupToken(context.Context, string, string) (string, time.Time, error)
	CompleteUserSetup(context.Context, string, string) (string, string, error)
	StartOIDCLogin(context.Context, string, string) (string, error)
	LoginWithOIDCCode(context.Context, string, string) (string, string, string, string, time.Time, error)
	Authenticate(context.Context, string) (string, string, string, error)
	AuthenticateIAPJWT(context.Context, string) (string, string, string, error)
	Logout(context.Context, string) error
	AuthenticateFull(context.Context, string) (auth.AuthResult, error)
}

type MachineStore interface {
	CreateMachineWithOwner(context.Context, string, string, string, string, ...string) (db.Machine, error)
	ListMachinesByUser(context.Context, string) ([]db.Machine, error)
	GetMachineByIDForUser(context.Context, string, string) (db.Machine, error)
	ListMachineEventsByMachineIDForUser(context.Context, string, string, int64) ([]db.MachineEvent, error)
	UpdateMachineNameByIDForOwner(context.Context, string, string, string) (bool, error)
	RequestStartMachineByIDForOwner(context.Context, string, string) (bool, error)
	RequestStopMachineByIDForOwner(context.Context, string, string) (bool, error)
	RequestRestartMachineByIDForOwner(context.Context, string, string) (bool, error)
	RequestDeleteMachineByIDForOwner(context.Context, string, string) (bool, error)
	DeleteMachineByIDForOwner(context.Context, string, string) (bool, error)
	DeleteMachineByID(context.Context, string) (bool, error)
	GetMachineProfileByID(context.Context, string) (db.MachineProfile, error)
}

const sessionCookieName = "arca_session"
const oidcStateCookieName = "arca_oidc_state"

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(RequestIDToContext)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Apply a 30-second timeout to API requests but skip it for machine
	// proxy requests which carry long-lived WebSocket connections (e.g. ttyd).
	r.Use(timeoutUnlessMachineProxy(deps.MachineProxy, 30*time.Second))
	r.Use(securityHeaders)

	if deps.Authenticator != nil {
		path, handler := arcav1connect.NewAuthServiceHandler(newAuthConnectService(deps.Authenticator, deps.Store, deps.RateLimiter))
		r.Mount(path, handler)
	}
	if deps.Authenticator != nil && deps.MachineStore != nil {
		path, handler := arcav1connect.NewMachineServiceHandler(newMachineConnectService(deps.Authenticator, deps.MachineStore, deps.Store))
		r.Mount(path, handler)
	}
	if deps.Authenticator != nil && deps.Store != nil {
		path, handler := arcav1connect.NewUserServiceHandler(newUserConnectService(deps.Store, deps.Authenticator, deps.Encryptor))
		r.Mount(path, handler)

		path, handler = arcav1connect.NewMachineProfileServiceHandler(newMachineProfileConnectService(deps.Store, deps.Authenticator))
		r.Mount(path, handler)
	}
	if deps.Store != nil && deps.Authenticator != nil {
		path, handler := arcav1connect.NewSharingServiceHandler(newSharingConnectService(deps.Store, deps.Authenticator))
		r.Mount(path, handler)

		path, handler = arcav1connect.NewGroupServiceHandler(newGroupConnectService(deps.Store, deps.Authenticator))
		r.Mount(path, handler)
	}
	if deps.Store != nil && deps.Authenticator != nil {
		path, handler := arcav1connect.NewSetupServiceHandler(newSetupConnectService(deps.Store, deps.Authenticator))
		r.Mount(path, handler)

		path, handler = arcav1connect.NewTicketServiceHandler(newTicketConnectService(deps.Store, deps.Authenticator))
		r.Mount(path, handler)
	}
	if deps.Store != nil {
		path, handler := arcav1connect.NewExposureServiceHandler(newExposureConnectService(deps.Store, deps.Authenticator, deps.Encryptor, deps.LLMTokenExecutor))
		r.Mount(path, handler)
	}
	if deps.Store != nil && deps.Authenticator != nil && deps.Slack != nil {
		path, handler := arcav1connect.NewNotificationServiceHandler(newNotificationConnectService(deps.Store, deps.Authenticator, deps.Slack))
		r.Mount(path, handler)
	}
	if deps.Store != nil && deps.Authenticator != nil {
		path, handler := arcav1connect.NewAdminServiceHandler(newAdminConnectService(deps.Store, deps.Authenticator))
		r.Mount(path, handler)

		path, handler = arcav1connect.NewImageServiceHandler(newImageConnectService(deps.Store, deps.Authenticator))
		r.Mount(path, handler)
	}
	if deps.MockRuntime != nil && deps.Authenticator != nil {
		path, handler := arcav1connect.NewMockServiceHandler(newMockConnectService(deps.MockRuntime, deps.Authenticator, deps.Store))
		r.Mount(path, handler)
	}
	if deps.Store != nil && deps.Authenticator != nil {
		authorizeHandler := newConsoleAuthorizeHandler(deps.Store, deps.Authenticator)
		r.Get("/console/authorize", authorizeHandler)
		// Backward-compatible alias.
		r.Get("/auth/authorize", authorizeHandler)
	}
	if deps.Store != nil {
		arcadHandler := newArcadBinaryHandler(deps.Store)
		r.Route("/arcad", func(sub chi.Router) {
			sub.Use(middleware.Timeout(5 * time.Minute))
			sub.Get("/download", arcadHandler.ServeHTTP)
			sub.Get("/version", newArcadVersionHandler(deps.Store))
		})
	}

	// Health check endpoint — always available, no auth required.
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if err := deps.HealthChecker.Ping(req.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})

	// Readiness check endpoint — reports DB connectivity and job health metrics.
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := deps.HealthChecker.Ping(req.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"status":"unhealthy","db":"down"}`)
			return
		}

		nowUnix := time.Now().Unix()
		fiveMinAgo := nowUnix - 300
		stats, err := deps.Store.CountRecentJobsByStatus(req.Context(), fiveMinAgo, nowUnix)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"status":"healthy","db":"up","jobs":"unknown"}`)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"healthy","db":"up","jobs":{"succeeded":%d,"failed":%d,"stuck":%d}}`,
			stats.Succeeded, stats.Failed, stats.Stuck)
	})

	// Machine proxy middleware: intercept requests with Host headers matching
	// machine exposures in proxy-via-server mode before the SPA handler.
	spa := spaHandler()
	machineProxy := deps.MachineProxy
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if machineProxy != nil && machineProxy.TryServeHTTP(w, req) {
			return
		}
		spa(w, req)
	})
	return r
}

func spaHandler() http.HandlerFunc {
	staticFS, err := fs.Sub(spaAssets, "ui/dist")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(staticFS))

	return func(w http.ResponseWriter, req *http.Request) {
		requestedPath := strings.TrimPrefix(path.Clean(req.URL.Path), "/")
		if requestedPath == "." {
			requestedPath = ""
		}

		if requestedPath != "" {
			if f, err := staticFS.Open(requestedPath); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, req)
				return
			}
		}

		indexFile, err := staticFS.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		defer indexFile.Close()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, indexFile)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// timeoutUnlessMachineProxy returns a middleware that applies the given timeout
// to all requests except those whose Host header matches a proxy-via-server
// machine exposure. Those requests may carry long-lived WebSocket connections
// (ttyd, shelley) that must not be interrupted by a short deadline.
func timeoutUnlessMachineProxy(proxy *MachineProxyHandler, timeout time.Duration) func(http.Handler) http.Handler {
	inner := middleware.Timeout(timeout)
	return func(next http.Handler) http.Handler {
		withTimeout := inner(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if proxy != nil && proxy.IsMachineProxyRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			withTimeout.ServeHTTP(w, r)
		})
	}
}
