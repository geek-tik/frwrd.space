package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/forward/forward/internal/inspector"
	"github.com/forward/forward/internal/registry"
	"github.com/forward/forward/internal/store"
)

func (s *HTTPServer) listTunnelRequests(w http.ResponseWriter, r *http.Request) {
	user, tunnelID, ok := s.tunnelAccess(w, r)
	if !ok {
		return
	}
	_ = user

	rows, err := s.httpRequests.ListByTunnel(r.Context(), tunnelID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить запросы")
		return
	}

	result := make([]inspector.Event, 0, len(rows))
	for _, row := range rows {
		result = append(result, toEvent(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": result})
}

func (s *HTTPServer) getTunnelRequest(w http.ResponseWriter, r *http.Request) {
	_, tunnelID, ok := s.tunnelAccess(w, r)
	if !ok {
		return
	}

	requestID := chi.URLParam(r, "req_id")
	row, err := s.httpRequests.GetByID(r.Context(), tunnelID, requestID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "запрос не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось получить запрос")
		return
	}

	writeJSON(w, http.StatusOK, toDetail(row))
}

func (s *HTTPServer) streamTunnelRequests(w http.ResponseWriter, r *http.Request) {
	_, tunnelID, ok := s.tunnelAccess(w, r)
	if !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming не поддерживается")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsub := s.inspector.Hub().Subscribe(tunnelID)
	defer unsub()

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "event: request\ndata: %s\n\n", inspector.MarshalSSE(event))
			flusher.Flush()
		}
	}
}

func (s *HTTPServer) tunnelAccess(w http.ResponseWriter, r *http.Request) (AuthUser, string, bool) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "требуется авторизация")
		return AuthUser{}, "", false
	}

	tunnelID := chi.URLParam(r, "id")
	if tunnelID == "" {
		writeError(w, http.StatusBadRequest, "tunnel id обязателен")
		return AuthUser{}, "", false
	}

	if session := s.findActiveTunnel(user.ID, tunnelID); session != nil {
		return user, tunnelID, true
	}

	owned, err := s.httpRequests.TunnelOwnedByUser(r.Context(), tunnelID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка проверки доступа")
		return AuthUser{}, "", false
	}
	if !owned {
		writeError(w, http.StatusNotFound, "туннель не найден")
		return AuthUser{}, "", false
	}
	return user, tunnelID, true
}

func (s *HTTPServer) findActiveTunnel(userID, tunnelID string) *registry.Session {
	for _, session := range s.registry.ListByUser(userID) {
		if session.ID == tunnelID {
			return session
		}
	}
	return nil
}

func toEvent(row store.HTTPRequest) inspector.Event {
	return inspector.Event{
		ID:         row.ID,
		TunnelID:   row.TunnelID,
		Method:     row.Method,
		Path:       row.Path,
		Query:      row.Query,
		StatusCode: row.StatusCode,
		DurationMs: row.DurationMs,
		CreatedAt:  row.CreatedAt,
	}
}

func toDetail(row store.HTTPRequest) inspector.Detail {
	return inspector.Detail{
		Event:       toEvent(row),
		Headers:     store.UnmarshalHeaders(row.Headers),
		Body:        string(row.Body),
		RespHeaders: store.UnmarshalHeaders(row.RespHeaders),
		RespBody:    string(row.RespBody),
	}
}
