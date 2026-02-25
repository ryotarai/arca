package server

import (
	"context"
	"database/sql"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Dependencies struct {
	HealthChecker HealthChecker
}

type HealthChecker interface {
	Ping(context.Context) error
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
			statusCode := http.StatusOK
			body := `{"status":"ok","db":"ok"}`
			if deps.HealthChecker == nil {
				statusCode = http.StatusServiceUnavailable
				body = `{"status":"degraded","db":"unconfigured"}`
			} else if err := deps.HealthChecker.Ping(req.Context()); err != nil {
				statusCode = http.StatusServiceUnavailable
				if err == sql.ErrNoRows {
					body = `{"status":"degraded","db":"schema_uninitialized"}`
				} else {
					body = `{"status":"degraded","db":"error"}`
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(body))
		})
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		})
	})

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
