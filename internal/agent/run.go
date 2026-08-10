package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	"github.com/forward/forward/internal/config"
	"github.com/forward/forward/internal/protocol"
	"github.com/forward/forward/internal/protocol/wsconn"
	"github.com/forward/forward/internal/tunnelproxy"
)

func RunHTTP(ctx context.Context, cfg config.Agent, port int) error {
	ctx, cancel := signalContext(ctx)
	defer cancel()

	for attempt := 0; ; attempt++ {
		err := runOnce(ctx, cfg, port)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			slog.Error("tunnel disconnected", "error", err)
		}

		delay := backoff(attempt)
		slog.Info("reconnecting", "delay", delay.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

func runOnce(ctx context.Context, cfg config.Agent, port int) error {
	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + cfg.Token},
		},
	}

	conn, _, err := websocket.Dial(ctx, cfg.ServerURL, opts)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	register := protocol.ControlMessage{
		Type:      protocol.TypeRegister,
		LocalPort: port,
		Protocol:  "http",
	}
	registerRaw, _ := json.Marshal(register)
	if err := conn.Write(ctx, websocket.MessageText, registerRaw); err != nil {
		return fmt.Errorf("send register: %w", err)
	}

	typ, data, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("read registered: %w", err)
	}
	if typ != websocket.MessageText {
		return fmt.Errorf("unexpected message type")
	}

	var reply protocol.ControlMessage
	if err := json.Unmarshal(data, &reply); err != nil {
		return fmt.Errorf("parse registered: %w", err)
	}
	if reply.Type == protocol.TypeError {
		return fmt.Errorf("%s", reply.Message)
	}
	if reply.Type != protocol.TypeRegistered {
		return fmt.Errorf("unexpected reply type: %s", reply.Type)
	}

	baseDomain := domainFromURL(reply.PublicURL)
	fmt.Println()
	fmt.Println("  Туннель запущен")
	fmt.Println()
	fmt.Printf("  Публичный URL   %s\n", reply.PublicURL)
	fmt.Printf("  Локальный       http://%s:%d\n", cfg.LocalHost, port)
	fmt.Printf("  Инспектор       %s/tunnels/%s\n", strings.TrimRight(cfg.DashboardURL, "/"), reply.TunnelID)
	fmt.Println()
	fmt.Println("  Локальная проверка (dev):")
	fmt.Printf("    curl -H \"Host: %s.%s\" http://localhost:8081/\n", reply.Subdomain, baseDomain)
	fmt.Println()
	fmt.Println("  Нажмите Ctrl+C для остановки")

	yamuxConn := wsconn.New(ctx, conn)
	session, err := yamux.Client(yamuxConn, yamux.DefaultConfig())
	if err != nil {
		return fmt.Errorf("yamux client: %w", err)
	}
	defer session.Close()

	errCh := make(chan error, 1)
	go func() {
		for {
			stream, err := session.Accept()
			if err != nil {
				errCh <- err
				return
			}
			go func(s net.Conn) {
				if err := tunnelproxy.ServeStream(s, cfg.LocalHost, port); err != nil {
					slog.Debug("stream error", "error", err)
				}
			}(stream)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func domainFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "frwrd.space"
	}
	host := u.Hostname()
	if i := strings.Index(host, "."); i >= 0 {
		return host[i+1:]
	}
	return host
}

func backoff(attempt int) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	return time.Second << attempt
}
