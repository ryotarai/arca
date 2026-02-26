package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ryotarai/hayai/internal/auth"
	"github.com/ryotarai/hayai/internal/gen/hayai/v1/hayaiv1connect"
)

type Dependencies struct {
	HealthChecker HealthChecker
	Authenticator Authenticator
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

const sessionCookieName = "hayai_session"

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

		r.Post("/auth/register", func(w http.ResponseWriter, req *http.Request) {
			if deps.Authenticator == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "auth unavailable")
				return
			}

			var payload credentialsPayload
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid request body")
				return
			}

			userID, email, err := deps.Authenticator.Register(req.Context(), payload.Email, payload.Password)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrInvalidInput):
					writeJSONError(w, http.StatusBadRequest, "email or password is invalid")
				case errors.Is(err, auth.ErrEmailAlreadyUsed):
					writeJSONError(w, http.StatusConflict, "email already used")
				default:
					log.Printf("register failed: %v", err)
					writeJSONError(w, http.StatusInternalServerError, "failed to register")
				}
				return
			}

			writeJSON(w, http.StatusCreated, map[string]any{
				"user": map[string]string{
					"id":    userID,
					"email": email,
				},
			})
		})

		r.Post("/auth/login", func(w http.ResponseWriter, req *http.Request) {
			if deps.Authenticator == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "auth unavailable")
				return
			}

			var payload credentialsPayload
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid request body")
				return
			}

			userID, email, token, expiresAt, err := deps.Authenticator.Login(req.Context(), payload.Email, payload.Password)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrInvalidCredentials):
					writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
				default:
					log.Printf("login failed: %v", err)
					writeJSONError(w, http.StatusInternalServerError, "failed to login")
				}
				return
			}

			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   req.TLS != nil,
				Expires:  expiresAt,
			})

			writeJSON(w, http.StatusOK, map[string]any{
				"user": map[string]string{
					"id":    userID,
					"email": email,
				},
			})
		})

		r.Post("/auth/logout", func(w http.ResponseWriter, req *http.Request) {
			if deps.Authenticator == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "auth unavailable")
				return
			}

			sessionCookie, err := req.Cookie(sessionCookieName)
			if err == nil && sessionCookie.Value != "" {
				_ = deps.Authenticator.Logout(req.Context(), sessionCookie.Value)
			}

			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   req.TLS != nil,
				MaxAge:   -1,
				Expires:  time.Unix(0, 0),
			})

			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})

		r.Get("/auth/me", func(w http.ResponseWriter, req *http.Request) {
			if deps.Authenticator == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "auth unavailable")
				return
			}

			sessionCookie, err := req.Cookie(sessionCookieName)
			if err != nil || sessionCookie.Value == "" {
				writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
				return
			}

			userID, email, err := deps.Authenticator.Authenticate(req.Context(), sessionCookie.Value)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrUnauthenticated):
					writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
				default:
					log.Printf("authenticate failed: %v", err)
					writeJSONError(w, http.StatusInternalServerError, "failed to authenticate")
				}
				return
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"user": map[string]string{
					"id":    userID,
					"email": email,
				},
			})
		})

		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeJSONError(w, http.StatusNotFound, "not found")
		})
	})

	if deps.Authenticator != nil {
		path, handler := hayaiv1connect.NewAuthServiceHandler(newAuthConnectService(deps.Authenticator))
		r.Mount(path, handler)
	}

	r.NotFound(spaHandler())
	return r
}

type credentialsPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
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
