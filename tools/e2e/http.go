package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxBody = 4 << 20

type api struct{ client *http.Client }

func newAPI(minio string) *api {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "minio:9000" && minio != "" {
			addr = minio
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return &api{client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func readBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBody {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBody)
	}
	return data, nil
}

func (a *api) request(ctx context.Context, method, addr, token string, body any, result any) (int, error) {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, addr, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := readBody(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s %s: HTTP %d", method, req.URL.Path, resp.StatusCode)
	}
	if result != nil {
		if err = json.Unmarshal(data, result); err != nil {
			return resp.StatusCode, fmt.Errorf("%s: invalid JSON: %w", req.URL.Path, err)
		}
	}
	return resp.StatusCode, nil
}

func (a *api) json(ctx context.Context, method, addr, token string, body, result any) error {
	_, err := a.request(ctx, method, addr, token, body, result)
	return err
}

// poll retries only read-only readiness and observation operations.
func poll(ctx context.Context, check func() error) error {
	var last error
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("observation stopped (%v): %w", last, err)
		}
		last = check()
		if last == nil {
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("observation stopped (%v): %w", last, ctx.Err())
		case <-timer.C:
		}
	}
}

var verificationCode = regexp.MustCompile(`Your verification code is: ([0-9]{6})`)

func mailCode(data []byte, email string) (string, error) {
	var box struct {
		Items []struct {
			To      []struct{ Mailbox, Domain string }
			Content struct{ Body string }
		}
	}
	if err := json.Unmarshal(data, &box); err != nil {
		return "", err
	}
	for _, m := range box.Items {
		for _, to := range m.To {
			if to.Mailbox+"@"+to.Domain == email {
				if match := verificationCode.FindStringSubmatch(m.Content.Body); match != nil {
					return match[1], nil
				}
			}
		}
	}
	return "", fmt.Errorf("verification mail not received")
}

var metricLabel = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)=("(?:[^"\\]|\\.)*")`)

func rpcCount(text, service, transport, method string) float64 {
	var count float64
	for _, line := range strings.Split(text, "\n") {
		prefix := "grpc_request_duration_seconds_count{"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		end := strings.Index(line, "}")
		if end < 0 {
			continue
		}
		labels := map[string]string{}
		for _, pair := range metricLabel.FindAllStringSubmatch(line[len(prefix):end], -1) {
			value, err := strconv.Unquote(pair[2])
			if err == nil {
				labels[pair[1]] = value
			}
		}
		if labels["service"] == service && labels["transport"] == transport && labels["method"] == method && labels["code"] == "OK" {
			n, err := strconv.ParseFloat(strings.TrimSpace(line[end+1:]), 64)
			if err == nil {
				count += n
			}
		}
	}
	return count
}

func (a *api) object(ctx context.Context, method, addr string, body []byte) ([]byte, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" || u.Host != "minio:9000" || !strings.HasPrefix(u.Path, "/messenger-media/") {
		return nil, fmt.Errorf("unexpected object URL host or bucket")
	}
	req, err := http.NewRequestWithContext(ctx, method, addr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("object %s: HTTP %d", method, resp.StatusCode)
	}
	return readBody(resp.Body)
}
