package inspector

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const MaxBodyBytes = 1 << 20 // 1 MB

type Event struct {
	ID         string    `json:"id"`
	TunnelID   string    `json:"tunnel_id"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Query      string    `json:"query,omitempty"`
	StatusCode int       `json:"status_code"`
	DurationMs int       `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

type Detail struct {
	Event
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body,omitempty"`
	RespHeaders map[string]string `json:"resp_headers"`
	RespBody    string            `json:"resp_body,omitempty"`
}

type Captured struct {
	Method      string
	Path        string
	Query       string
	Headers     http.Header
	Body        []byte
	StatusCode  int
	RespHeaders http.Header
	RespBody    []byte
	Duration    time.Duration
}

type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan Event]struct{})}
}

func (h *Hub) Subscribe(tunnelID string) (<-chan Event, func()) {
	ch := make(chan Event, 8)
	h.mu.Lock()
	if h.subs[tunnelID] == nil {
		h.subs[tunnelID] = make(map[chan Event]struct{})
	}
	h.subs[tunnelID][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.subs[tunnelID], ch)
		if len(h.subs[tunnelID]) == 0 {
			delete(h.subs, tunnelID)
		}
		h.mu.Unlock()
		close(ch)
	}
}

func (h *Hub) Publish(tunnelID string, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[tunnelID] {
		select {
		case ch <- event:
		default:
		}
	}
}

func HeadersToMap(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, values := range h {
		out[k] = joinHeader(values)
	}
	return out
}

func joinHeader(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for i := 1; i < len(values); i++ {
		out += ", " + values[i]
	}
	return out
}

func MarshalSSE(event Event) string {
	raw, _ := json.Marshal(event)
	return string(raw)
}
