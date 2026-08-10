package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/forward/forward/internal/auth"
	"github.com/forward/forward/internal/config"
	"github.com/forward/forward/internal/inspector"
	"github.com/forward/forward/internal/registry"
	"github.com/forward/forward/internal/store"
)

type HTTPServer struct {
	cfg           config.Server
	pool          *pgxpool.Pool
	tokens        *auth.TokenService
	users         *store.UserStore
	refreshTokens *store.RefreshTokenStore
	apiTokens     *store.APITokenStore
	httpRequests  *store.HTTPRequestStore
	inspector     *inspector.Service
	registry      *registry.Registry
	secureCookies bool
}

func NewHTTPServer(cfg config.Server, pool *pgxpool.Pool, reg *registry.Registry, insp *inspector.Service) *HTTPServer {
	return &HTTPServer{
		cfg:           cfg,
		pool:          pool,
		tokens:        auth.NewTokenService(cfg.JWTSecret, 15*time.Minute, 7*24*time.Hour),
		users:         store.NewUserStore(pool),
		refreshTokens: store.NewRefreshTokenStore(pool),
		apiTokens:     store.NewAPITokenStore(pool),
		httpRequests:  store.NewHTTPRequestStore(pool),
		inspector:     insp,
		registry:      reg,
		secureCookies: cfg.SecureCookies,
	}
}

func (s *HTTPServer) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", s.health)
	r.Get("/ready", s.ready)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/status", s.apiStatus)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.register)
			r.Post("/login", s.login)
			r.Post("/refresh", s.refresh)
			r.Post("/logout", s.logout)
		})

		r.Group(func(r chi.Router) {
			r.Use(s.requireJWT)
			r.Get("/me", s.me)
			r.Get("/tunnels", s.listTunnels)
			r.Get("/tunnels/{id}/requests", s.listTunnelRequests)
			r.Get("/tunnels/{id}/requests/stream", s.streamTunnelRequests)
			r.Get("/tunnels/{id}/requests/{req_id}", s.getTunnelRequest)
			r.Get("/tokens", s.listTokens)
			r.Post("/tokens", s.createToken)
			r.Delete("/tokens/{id}", s.deleteToken)
		})
	})

	return r
}

func (s *HTTPServer) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "forward",
	})
}

func (s *HTTPServer) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  "database unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *HTTPServer) apiStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":     "forward",
		"base_domain": s.cfg.BaseDomain,
		"version":     "0.1.0-dev",
	})
}
