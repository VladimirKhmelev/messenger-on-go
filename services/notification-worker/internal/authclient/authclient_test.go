package authclient

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/quictransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/domain"
)

type authStub struct {
	authv1.UnimplementedAuthServiceServer
	list func(context.Context, *authv1.ListPushSubscriptionsRequest) (*authv1.ListPushSubscriptionsResponse, error)
}

func (s *authStub) ListPushSubscriptions(ctx context.Context, req *authv1.ListPushSubscriptionsRequest) (*authv1.ListPushSubscriptionsResponse, error) {
	return s.list(ctx, req)
}
func certificates(t *testing.T) (ca, cert, key string) {
	t.Helper()
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("no openssl")
	}
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "sh", "../../../../scripts/quic-certs.sh")
	cmd.Env = append(os.Environ(), "CERTS_DIR="+dir, "QUIC_SERVERS=localhost", "QUIC_CLIENTS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("certificates: %v\n%s", err, out)
	}
	return filepath.Join(dir, "localhost/ca.pem"), filepath.Join(dir, "localhost/cert.pem"), filepath.Join(dir, "localhost/key.pem")
}

func client(t *testing.T, transport, secret string, handler authv1.AuthServiceServer) *Client {
	t.Helper()
	var c *Client
	var err error
	if transport == "grpc" {
		l, listenErr := net.Listen("tcp", "127.0.0.1:0")
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		srv := grpc.NewServer()
		authv1.RegisterAuthServiceServer(srv, handler)
		go func() { _ = srv.Serve(l) }()
		t.Cleanup(srv.Stop)
		c, err = Dial(l.Addr().String(), secret)
	} else {
		ca, cert, key := certificates(t)
		srv, listenErr := quictransport.NewServer(quictransport.ServerConfig{
			Service: "auth-service", Addr: "127.0.0.1:0", CertFile: cert, KeyFile: key, ResetSecret: "test-secret",
		})
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		authv1.RegisterAuthServiceServer(srv, handler)
		go func() { _ = srv.Serve() }()
		t.Cleanup(srv.Stop)
		_, port, _ := net.SplitHostPort(srv.Addr().String())
		c, err = DialQUIC(net.JoinHostPort("localhost", port), secret, ca)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// tracedContext lets the server check that both transports carry the
// caller's trace alongside the internal authorization metadata.
func tracedContext(t *testing.T) context.Context {
	t.Helper()
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	return trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	}))
}
func checkMetadata(ctx context.Context, traced bool) error {
	md, _ := metadata.FromIncomingContext(ctx)
	if got := md.Get("x-internal-secret"); len(got) != 1 || got[0] != "secret" {
		return status.Error(codes.Unauthenticated, "invalid internal secret")
	}
	if traced {
		got := md.Get("traceparent")
		if len(got) != 1 || !strings.Contains(got[0], (trace.TraceID{1}).String()) {
			return status.Error(codes.Internal, "trace context lost")
		}
	}
	return nil
}

func TestCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(tracedContext(t), 5*time.Second)
	defer cancel()
	for _, tr := range []string{"grpc", "quic"} {
		t.Run(tr, func(t *testing.T) {
			handler := &authStub{list: func(ctx context.Context, req *authv1.ListPushSubscriptionsRequest) (*authv1.ListPushSubscriptionsResponse, error) {
				if err := checkMetadata(ctx, true); err != nil {
					return nil, err
				}
				switch req.GetUserId() {
				case "missing":
					return nil, status.Error(codes.NotFound, "missing user")
				case "broken":
					return nil, status.Error(codes.Unavailable, "unavailable")
				case "empty":
					return &authv1.ListPushSubscriptionsResponse{}, nil
				default:
					return &authv1.ListPushSubscriptionsResponse{Subscriptions: []*authv1.PushSubscription{
						{Endpoint: "https://push.test/second", P256DhKey: "second-key", AuthKey: "second-auth"},
						{Endpoint: "https://push.test/first", P256DhKey: "first-key", AuthKey: "first-auth"},
					}}, nil
				}
			}}
			c := client(t, tr, "secret", handler)
			want := []domain.PushSubscription{
				{Endpoint: "https://push.test/second", P256dhKey: "second-key", AuthKey: "second-auth"},
				{Endpoint: "https://push.test/first", P256dhKey: "first-key", AuthKey: "first-auth"},
			}
			for _, tt := range []struct {
				user string
				want []domain.PushSubscription
				code codes.Code
			}{
				{"user", want, codes.OK}, {"empty", nil, codes.OK}, {"missing", nil, codes.NotFound}, {"broken", nil, codes.Unavailable},
			} {
				got, err := c.ListPushSubscriptions(ctx, tt.user)
				if !slices.Equal(got, tt.want) || status.Code(err) != tt.code {
					t.Fatalf("ListPushSubscriptions %s: %v, %v", tt.user, got, err)
				}
			}
			wrong := client(t, tr, "wrong", handler)
			if _, err := wrong.ListPushSubscriptions(ctx, "user"); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("wrong internal secret: %v", err)
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := c.ListPushSubscriptions(ctx, "user"); status.Code(err) != codes.Canceled {
				t.Fatalf("call after Close: %v", err)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	for _, tr := range []string{"grpc", "quic"} {
		t.Run(tr, func(t *testing.T) {
			started, done := make(chan struct{}, 1), make(chan struct{}, 1)
			c := client(t, tr, "secret", &authStub{list: func(ctx context.Context, _ *authv1.ListPushSubscriptionsRequest) (*authv1.ListPushSubscriptionsResponse, error) {
				if err := checkMetadata(ctx, false); err != nil {
					return nil, err
				}
				started <- struct{}{}
				<-ctx.Done()
				done <- struct{}{}
				return nil, status.FromContextError(ctx.Err()).Err()
			}})
			for _, deadline := range []bool{false, true} {
				ctx, cancel := context.WithCancel(t.Context())
				want := codes.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(t.Context(), 200*time.Millisecond)
					want = codes.DeadlineExceeded
				}
				result := make(chan error, 1)
				go func() { _, err := c.ListPushSubscriptions(ctx, "user"); result <- err }()
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					cancel()
					t.Fatal("handler did not start")
				}
				if !deadline {
					cancel()
				}
				if err := <-result; status.Code(err) != want {
					t.Fatalf("call: %v, want %v", err, want)
				}
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("server context was not canceled")
				}
			}
		})
	}
}
