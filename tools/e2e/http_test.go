package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMailCodeUsesExactRecipient(t *testing.T) {
	box := []byte(`{"items":[{"To":[{"Mailbox":"other","Domain":"example.test"}],"Content":{"Body":"Your verification code is: 111111"}},{"To":[{"Mailbox":"alice","Domain":"example.test"}],"Content":{"Body":"Your verification code is: 002345\r\nIt expires in 10 minutes."}}]}`)
	if code, err := mailCode(box, "alice@example.test"); err != nil || code != "002345" {
		t.Fatalf("code = %s, %v", code, err)
	}
	if _, err := mailCode(box, "alice@elsewhere.test"); err == nil {
		t.Fatal("another recipient matched")
	}
	if _, err := mailCode([]byte(`broken`), "alice@example.test"); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
func TestMetricsRequireSuccessfulSelectedTransport(t *testing.T) {
	text := `grpc_request_duration_seconds_count{method="/auth.v1.AuthService/GetUserByID",code="OK",service="chat-service",transport="quic"} 3
` + `grpc_request_duration_seconds_count{method="/auth.v1.AuthService/GetUserByID",code="OK",service="chat-service",transport="grpc"} 5
` + `grpc_request_duration_seconds_count{method="/auth.v1.AuthService/GetUserByID",code="Unavailable",service="chat-service",transport="quic"} 7
` + `grpc_request_duration_seconds_sum{method="/auth.v1.AuthService/GetUserByID",code="OK",service="chat-service",transport="quic"} 11
`
	if n := rpcCount(text, "chat-service", "quic", "/auth.v1.AuthService/GetUserByID"); n != 3 {
		t.Fatal(n)
	}
	if n := rpcCount(text, "media-service", "quic", "/auth.v1.AuthService/GetUserByID"); n != 0 {
		t.Fatal(n)
	}
}
func TestAPIPropagatesContextAndDoesNotRetryWrites(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing authorization")
		}
		http.Error(w, "private error", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	a := newAPI("")
	defer a.client.CloseIdleConnections()
	status, err := a.request(t.Context(), "POST", srv.URL, "token", map[string]string{"text": "once"}, nil)
	if err == nil || status != 503 || calls != 1 || strings.Contains(err.Error(), "private error") {
		t.Fatalf("status %d, calls %d, error %v", status, calls, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := a.json(ctx, "GET", srv.URL, "", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestObjectPreservesSignedHostAndPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "minio:9000" || r.URL.Path != "/messenger-media/chats/test" || r.URL.RawQuery != "signature=opaque" {
			t.Errorf("signed target changed: %s %s", r.Host, r.URL)
		}
		_, _ = io.WriteString(w, "bytes")
	}))
	defer srv.Close()
	a := newAPI(strings.TrimPrefix(srv.URL, "http://"))
	defer a.client.CloseIdleConnections()
	b, err := a.object(t.Context(), "GET", "http://minio:9000/messenger-media/chats/test?signature=opaque", nil)
	if err != nil || string(b) != "bytes" {
		t.Fatalf("body %q, error %v", b, err)
	}
	for _, addr := range []string{"http://elsewhere:9000/messenger-media/test", "http://minio:9000/other/test", "https://minio:9000/messenger-media/test"} {
		if _, err := a.object(t.Context(), "GET", addr, nil); err == nil {
			t.Errorf("accepted %s", addr)
		}
	}
}
func TestBoundedBodyAndPolling(t *testing.T) {
	if _, err := readBody(strings.NewReader(strings.Repeat("x", maxBody+1))); err == nil {
		t.Fatal("oversized response accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	if err := poll(ctx, func() error { calls++; return nil }); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("err %v, calls %d", err, calls)
	}
	ctx, cancel = context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	if err := poll(ctx, func() error { return errors.New("not ready") }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
