package inspector

import (
	"context"
	"log/slog"

	"github.com/forward/forward/internal/store"
)

const maxRecordsPerTunnel = 100

type Service struct {
	store *store.HTTPRequestStore
	hub   *Hub
}

func NewService(requests *store.HTTPRequestStore, hub *Hub) *Service {
	return &Service{store: requests, hub: hub}
}

func (s *Service) Hub() *Hub {
	return s.hub
}

func (s *Service) Record(ctx context.Context, tunnelID string, captured Captured) (Event, error) {
	headersJSON, err := store.MarshalHeaders(captured.Headers)
	if err != nil {
		return Event{}, err
	}
	respHeadersJSON, err := store.MarshalHeaders(captured.RespHeaders)
	if err != nil {
		return Event{}, err
	}

	row, err := s.store.Create(ctx, store.HTTPRequestInput{
		TunnelID:    tunnelID,
		Method:      captured.Method,
		Path:        captured.Path,
		Query:       captured.Query,
		Headers:     headersJSON,
		Body:        captured.Body,
		StatusCode:  captured.StatusCode,
		RespHeaders: respHeadersJSON,
		RespBody:    captured.RespBody,
		DurationMs:  int(captured.Duration.Milliseconds()),
	})
	if err != nil {
		return Event{}, err
	}

	go func() {
		bg := context.Background()
		if err := s.store.Prune(bg, tunnelID, maxRecordsPerTunnel); err != nil {
			slog.Debug("prune requests failed", "error", err)
		}
	}()

	event := Event{
		ID:         row.ID,
		TunnelID:   row.TunnelID,
		Method:     row.Method,
		Path:       row.Path,
		Query:      row.Query,
		StatusCode: row.StatusCode,
		DurationMs: row.DurationMs,
		CreatedAt:  row.CreatedAt,
	}
	s.hub.Publish(tunnelID, event)
	return event, nil
}
