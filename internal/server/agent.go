package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	"github.com/forward/forward/internal/config"
	"github.com/forward/forward/internal/protocol"
	"github.com/forward/forward/internal/protocol/wsconn"
	"github.com/forward/forward/internal/registry"
	"github.com/forward/forward/internal/store"
	"github.com/forward/forward/internal/subdomain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentServer struct {
	cfg       config.Server
	pool      *pgxpool.Pool
	apiTokens *store.APITokenStore
	users     *store.UserStore
	tunnels   *store.TunnelStore
	registry  *registry.Registry
}

func NewAgentServer(cfg config.Server, pool *pgxpool.Pool, reg *registry.Registry) *AgentServer {
	return &AgentServer{
		cfg:       cfg,
		pool:      pool,
		apiTokens: store.NewAPITokenStore(pool),
		users:     store.NewUserStore(pool),
		tunnels:   store.NewTunnelStore(pool),
		registry:  reg,
	}
}

func (s *AgentServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/connect", s.handleConnect)
	return mux
}

func (s *AgentServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	user, err := s.authenticateAgent(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "недействительный API token")
		return
	}

	if !isWebSocketUpgrade(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "authenticated",
			"user_id": user.ID,
			"email":   user.Email,
			"message": "используйте WebSocket upgrade для туннеля",
		})
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}

	s.handleAgentSession(r.Context(), conn, user)
}

func (s *AgentServer) handleAgentSession(ctx context.Context, conn *websocket.Conn, user AuthUser) {
	defer conn.Close(websocket.StatusNormalClosure, "")

	typ, data, err := conn.Read(ctx)
	if err != nil {
		slog.Error("read register failed", "error", err)
		return
	}
	if typ != websocket.MessageText {
		s.writeControlError(ctx, conn, "ожидалось текстовое сообщение register")
		return
	}

	var msg protocol.ControlMessage
	if err := json.Unmarshal(data, &msg); err != nil || msg.Type != protocol.TypeRegister {
		s.writeControlError(ctx, conn, "некорректное сообщение register")
		return
	}
	if msg.Protocol != "http" {
		s.writeControlError(ctx, conn, "поддерживается только protocol=http")
		return
	}
	if msg.LocalPort < 1 || msg.LocalPort > 65535 {
		s.writeControlError(ctx, conn, "некорректный local_port")
		return
	}

	if s.registry.CountByUser(user.ID) >= registry.MaxTunnelsPerUser {
		s.writeControlError(ctx, conn, fmt.Sprintf("лимит активных туннелей: %d", registry.MaxTunnelsPerUser))
		return
	}

	name, err := subdomain.Generate(s.registry.Exists)
	if err != nil {
		s.writeControlError(ctx, conn, "не удалось выделить subdomain")
		return
	}

	tunnel, err := s.tunnels.Create(ctx, user.ID, name, msg.LocalPort, msg.Protocol)
	if err != nil {
		slog.Error("create tunnel record failed", "error", err)
		s.writeControlError(ctx, conn, "не удалось создать туннель")
		return
	}

	reply := protocol.ControlMessage{
		Type:      protocol.TypeRegistered,
		TunnelID:  tunnel.ID,
		Subdomain: name,
		PublicURL: subdomain.PublicURL(s.cfg.BaseDomain, name),
	}
	replyRaw, _ := json.Marshal(reply)
	if err := conn.Write(ctx, websocket.MessageText, replyRaw); err != nil {
		_ = s.tunnels.Close(ctx, tunnel.ID)
		return
	}

	yamuxConn := wsconn.New(ctx, conn)
	muxSession, err := yamux.Server(yamuxConn, yamux.DefaultConfig())
	if err != nil {
		_ = s.tunnels.Close(ctx, tunnel.ID)
		slog.Error("yamux server failed", "error", err)
		return
	}

	session := &registry.Session{
		ID:        tunnel.ID,
		UserID:    user.ID,
		Subdomain: name,
		LocalPort: msg.LocalPort,
		Protocol:  msg.Protocol,
		Session:   muxSession,
	}
	s.registry.Register(session)
	defer func() {
		s.registry.Remove(tunnel.ID)
		_ = muxSession.Close()
		_ = s.tunnels.Close(context.Background(), tunnel.ID)
		slog.Info("tunnel closed", "subdomain", name, "user_id", user.ID)
	}()

	slog.Info("tunnel started",
		"subdomain", name,
		"user_id", user.ID,
		"local_port", msg.LocalPort,
	)

	<-muxSession.CloseChan()
}

func (s *AgentServer) writeControlError(ctx context.Context, conn *websocket.Conn, message string) {
	raw, _ := json.Marshal(protocol.ControlMessage{
		Type:    protocol.TypeError,
		Message: message,
	})
	_ = conn.Write(ctx, websocket.MessageText, raw)
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}
