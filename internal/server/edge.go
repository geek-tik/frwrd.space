package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/forward/forward/internal/config"
	"github.com/forward/forward/internal/inspector"
	"github.com/forward/forward/internal/registry"
	"github.com/forward/forward/internal/subdomain"
	"github.com/forward/forward/internal/tunnelproxy"
)

type EdgeServer struct {
	cfg       config.Server
	registry  *registry.Registry
	inspector *inspector.Service
}

func NewEdgeServer(cfg config.Server, reg *registry.Registry, insp *inspector.Service) *EdgeServer {
	return &EdgeServer{cfg: cfg, registry: reg, inspector: insp}
}

func (s *EdgeServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)
	return mux
}

func (s *EdgeServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	name := subdomain.Extract(r.Host, s.cfg.BaseDomain)
	if name == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "Туннель не найден. Укажите subdomain в Host: {name}.%s\n", s.cfg.BaseDomain)
		return
	}

	session, ok := s.registry.GetBySubdomain(name)
	if !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "Туннель %s не активен\n", name)
		return
	}

	tunnelID := session.ID
	tunnelproxy.ProxyToSession(w, r, func() (net.Conn, error) {
		return session.Session.OpenStream()
	}, func(captured inspector.Captured) {
		go func() {
			_, _ = s.inspector.Record(context.Background(), tunnelID, captured)
		}()
	})
}
