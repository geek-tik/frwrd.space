package tunnelproxy

import (
	"bufio"
	"bytes"
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

	if err := resp.Write(stream); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}

func ProxyToSession(w http.ResponseWriter, r *http.Request, openStream func() (net.Conn, error), onComplete func(inspector.Captured)) {
	start := time.Now()

	reqBody, err := readBodyLimited(r.Body, inspector.MaxBodyBytes)
	if err != nil {
		http.Error(w, "ошибка чтения запроса", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(reqBody))

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

	respBody, err := readBodyLimited(resp.Body, inspector.MaxBodyBytes)
	if err != nil {
		resp.Body.Close()
		http.Error(w, "ошибка чтения ответа", http.StatusBadGateway)
		return
	}
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)

	if onComplete != nil {
		onComplete(inspector.Captured{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Headers:     r.Header.Clone(),
			Body:        reqBody,
			StatusCode:  resp.StatusCode,
			RespHeaders: resp.Header.Clone(),
			RespBody:    respBody,
			Duration:    time.Since(start),
		})
	}
}

func readBodyLimited(body io.ReadCloser, limit int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return data[:limit], nil
	}
	return data, nil
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
