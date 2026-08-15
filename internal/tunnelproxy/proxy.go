package tunnelproxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/forward/forward/internal/inspector"
)

func ServeStream(stream net.Conn, localHost string, localPort int) error {
	defer stream.Close()

	br := bufio.NewReader(stream)
	req, err := http.ReadRequest(br)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}

	target := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", localHost, localPort),
	}

	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	if req.URL.Path == "" {
		req.URL.Path = "/"
	}
	req.RequestURI = ""

	if host := req.Header.Get("X-Forwarded-Host"); host != "" {
		req.Host = host
	}

	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.DialTimeout("tcp", target.Host, 10*time.Second)
			},
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return writeErrorResponse(stream, http.StatusBadGateway, err)
	}
	defer resp.Body.Close()

	// После HTTP 101 тело ответа становится двунаправленным соединением.
	// Обычный resp.Write записал бы только handshake и сразу закрыл stream,
	// поэтому дальше связываем yamux stream напрямую с локальным WebSocket.
	if resp.StatusCode == http.StatusSwitchingProtocols {
		upstream, ok := resp.Body.(io.ReadWriteCloser)
		if !ok {
			return fmt.Errorf("upgrade response body is not writable")
		}
		if err := writeResponseHead(stream, resp); err != nil {
			return fmt.Errorf("write upgrade response: %w", err)
		}

		return bridge(
			&bufferedConn{Conn: stream, reader: br},
			upstream,
		)
	}

	if err := resp.Write(stream); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}

func ProxyToSession(w http.ResponseWriter, r *http.Request, openStream func() (net.Conn, error), onComplete func(inspector.Captured)) {
	start := time.Now()

	// В туннель передаётся всё тело запроса. Инспектор параллельно сохраняет
	// только ограниченный префикс, чтобы большие upload'ы не обрезались.
	reqCapture := newLimitedCapture(inspector.MaxBodyBytes)
	if r.Body != nil {
		r.Body = &teeReadCloser{
			Reader: io.TeeReader(r.Body, reqCapture),
			Closer: r.Body,
		}
	}

	stream, err := openStream()
	if err != nil {
		http.Error(w, "туннель недоступен", http.StatusBadGateway)
		return
	}

	if r.Header.Get("X-Forwarded-Proto") == "" {
		r.Header.Set("X-Forwarded-Proto", "https")
	}
	if r.Header.Get("X-Forwarded-Host") == "" {
		r.Header.Set("X-Forwarded-Host", r.Host)
	}
	if r.Header.Get("X-Forwarded-For") == "" {
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	if err := r.Write(stream); err != nil {
		stream.Close()
		http.Error(w, "ошибка проксирования", http.StatusBadGateway)
		return
	}

	br := bufio.NewReader(stream)
	resp, err := http.ReadResponse(br, r)
	if err != nil {
		stream.Close()
		http.Error(w, "ошибка ответа туннеля", http.StatusBadGateway)
		return
	}
	defer stream.Close()

	// При Upgrade net/http больше не должен управлять соединением: передаём
	// клиенту handshake и переключаем обе стороны в raw duplex-режим.
	if resp.StatusCode == http.StatusSwitchingProtocols {
		proxyUpgrade(w, r, stream, br, resp, start, reqCapture.Bytes(), onComplete)
		return
	}
	defer resp.Body.Close()

	// Полный ответ потоково уходит клиенту, а инспектор получает не больше
	// MaxBodyBytes. Так Content-Length остаётся согласован с реальным ответом.
	respCapture := newLimitedCapture(inspector.MaxBodyBytes)

	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.TeeReader(resp.Body, respCapture))

	if onComplete != nil {
		onComplete(inspector.Captured{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Headers:     r.Header.Clone(),
			Body:        reqCapture.Bytes(),
			StatusCode:  resp.StatusCode,
			RespHeaders: resp.Header.Clone(),
			RespBody:    respCapture.Bytes(),
			Duration:    time.Since(start),
		})
	}
}

func proxyUpgrade(
	w http.ResponseWriter,
	r *http.Request,
	stream net.Conn,
	streamReader *bufio.Reader,
	resp *http.Response,
	start time.Time,
	reqBody []byte,
	onComplete func(inspector.Captured),
) {
	// Забираем TCP-соединение у HTTP-сервера после получения 101 от агента.
	clientConn, clientRW, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, "WebSocket upgrade не поддерживается", http.StatusInternalServerError)
		return
	}

	if err := writeResponseHead(clientRW.Writer, resp); err != nil {
		clientConn.Close()
		return
	}
	if err := clientRW.Flush(); err != nil {
		clientConn.Close()
		return
	}

	if onComplete != nil {
		onComplete(inspector.Captured{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Headers:     r.Header.Clone(),
			Body:        reqBody,
			StatusCode:  resp.StatusCode,
			RespHeaders: resp.Header.Clone(),
			Duration:    time.Since(start),
		})
	}

	_ = bridge(
		&bufferedConn{Conn: clientConn, reader: clientRW.Reader},
		&bufferedConn{Conn: stream, reader: streamReader},
	)
}

func writeResponseHead(w io.Writer, resp *http.Response) error {
	proto := resp.Proto
	if proto == "" {
		proto = "HTTP/1.1"
	}
	status := resp.Status
	if status == "" {
		status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if _, err := fmt.Fprintf(w, "%s %s\r\n", proto, status); err != nil {
		return err
	}
	if err := resp.Header.Write(w); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\r\n")
	return err
}

func bridge(a, b io.ReadWriteCloser) error {
	// WebSocket должен одновременно передавать сообщения в обоих направлениях.
	// Завершение любой стороны закрывает обе и разблокирует второй io.Copy.
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(a, b)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(b, a)
		errCh <- err
	}()

	err := <-errCh
	_ = a.Close()
	_ = b.Close()
	return err
}

type bufferedConn struct {
	net.Conn
	reader io.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	// Сначала возвращаем байты, которые bufio.Reader успел прочитать сверх
	// HTTP-заголовков; иначе начало первого WebSocket frame могло бы потеряться.
	return c.reader.Read(p)
}

type teeReadCloser struct {
	io.Reader
	io.Closer
}

type limitedCapture struct {
	limit int
	data  []byte
}

func newLimitedCapture(limit int64) *limitedCapture {
	return &limitedCapture{limit: int(limit)}
}

func (c *limitedCapture) Write(p []byte) (int, error) {
	// Для вызывающего TeeReader считаем принятым весь p, хотя сохраняем только
	// префикс: лимит относится к инспектору, а не к проксируемому трафику.
	remaining := c.limit - len(c.data)
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		c.data = append(c.data, p[:remaining]...)
	}
	return len(p), nil
}

func (c *limitedCapture) Bytes() []byte {
	return c.data
}

func writeErrorResponse(w io.Writer, code int, err error) error {
	body := err.Error()
	resp := &http.Response{
		StatusCode:    code,
		Status:        fmt.Sprintf("%d %s", code, http.StatusText(code)),
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: int64(len(body)),
	}
	resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return resp.Write(w)
}
