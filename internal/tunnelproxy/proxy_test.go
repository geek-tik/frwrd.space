package tunnelproxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestProxyToSessionStreamsNDJSON(t *testing.T) {
	releaseSecond := make(chan struct{})
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "{\"msg\":\"a\"}\n")
		w.(http.Flusher).Flush()
		<-releaseSecond
		io.WriteString(w, "{\"msg\":\"b\"}\n")
	}))
	defer origin.Close()

	_, portStr, err := net.SplitHostPort(origin.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	edgeConn, agentConn := net.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStream(agentConn, "127.0.0.1", port)
	}()

	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ProxyToSession(w, r, func() (net.Conn, error) {
			return edgeConn, nil
		}, nil)
	}))
	defer edge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, edge.URL+"/api/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", resp.Header.Get("X-Accel-Buffering"))
	}

	br := bufio.NewReader(resp.Body)
	line, err := br.ReadBytes('\n')
	if err != nil {
		t.Fatalf("first chunk blocked or missing: %v", err)
	}
	if string(line) != "{\"msg\":\"a\"}\n" {
		t.Fatalf("unexpected first chunk: %q", line)
	}

	close(releaseSecond)

	rest, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "{\"msg\":\"b\"}\n" {
		t.Fatalf("unexpected rest: %q", rest)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("agent stream did not finish")
	}
}
