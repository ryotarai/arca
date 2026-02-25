package server

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
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
