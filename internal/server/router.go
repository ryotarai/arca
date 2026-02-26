package server

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ryotarai/hayai/internal/db"
	"github.com/ryotarai/hayai/internal/gen/hayai/v1/hayaiv1connect"
)

type Dependencies struct {
	HealthChecker HealthChecker
	Authenticator Authenticator
	MachineStore  MachineStore
}

type HealthChecker interface {
	Ping(context.Context) error
}

type Authenticator interface {
	Register(context.Context, string, string) (string, string, error)
	Login(context.Context, string, string) (string, string, string, time.Time, error)
	Authenticate(context.Context, string) (string, string, error)
	Logout(context.Context, string) error
}

type MachineStore interface {
	CreateMachineWithOwner(context.Context, string, string) (db.Machine, error)
	ListMachinesByUser(context.Context, string) ([]db.Machine, error)
	UpdateMachineNameByIDForOwner(context.Context, string, string, string) (bool, error)
	RequestStartMachineByIDForOwner(context.Context, string, string) (bool, error)
	RequestStopMachineByIDForOwner(context.Context, string, string) (bool, error)
	DeleteMachineByIDForOwner(context.Context, string, string) (bool, error)
}

const sessionCookieName = "hayai_session"

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	if deps.Authenticator != nil {
		path, handler := hayaiv1connect.NewAuthServiceHandler(newAuthConnectService(deps.Authenticator))
		r.Mount(path, handler)
	}
	if deps.Authenticator != nil && deps.MachineStore != nil {
		r.Route("/api/machines", newMachineRouter(deps.Authenticator, deps.MachineStore))
	}

	r.NotFound(spaHandler())
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
