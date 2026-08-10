package server

import (
	"net/http"

	"github.com/forward/forward/internal/subdomain"
)

type tunnelView struct {
	ID        string `json:"id"`
	Subdomain string `json:"subdomain"`
	PublicURL string `json:"public_url"`
	LocalPort int    `json:"local_port"`
	Protocol  string `json:"protocol"`
	Status    string `json:"status"`
}

func (s *HTTPServer) listTunnels(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	sessions := s.registry.ListByUser(user.ID)
	result := make([]tunnelView, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, tunnelView{
			ID:        session.ID,
			Subdomain: session.Subdomain,
			PublicURL: subdomain.PublicURL(s.cfg.BaseDomain, session.Subdomain),
			LocalPort: session.LocalPort,
			Protocol:  session.Protocol,
			Status:    "active",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"tunnels": result})
}
