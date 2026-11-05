package chatclient

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
)

type chatStub struct {
	chatv1.UnimplementedChatServiceServer
	members  func(context.Context, *chatv1.ListMembersRequest) (*chatv1.ListMembersResponse, error)
	presence func(context.Context, *chatv1.GetPresenceRequest) (*chatv1.GetPresenceResponse, error)
}

func (s *chatStub) ListMembers(ctx context.Context, req *chatv1.ListMembersRequest) (*chatv1.ListMembersResponse, error) {
	return s.members(ctx, req)
}
func (s *chatStub) GetPresence(ctx context.Context, req *chatv1.GetPresenceRequest) (*chatv1.GetPresenceResponse, error) {
	return s.presence(ctx, req)
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

func client(t *testing.T, transport, secret string, handler chatv1.ChatServiceServer) *Client {
	t.Helper()
	var c *Client
	var err error
	if transport == "grpc" {
		l, listenErr := net.Listen("tcp", "127.0.0.1:0")
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		srv := grpc.NewServer()
		chatv1.RegisterChatServiceServer(srv, handler)
		go func() { _ = srv.Serve(l) }()
		t.Cleanup(srv.Stop)
		c, err = Dial(l.Addr().String(), secret)
	} else {
		ca, cert, key := certificates(t)
		srv, listenErr := quictransport.NewServer(quictransport.ServerConfig{
			Service: "chat-service", Addr: "127.0.0.1:0", CertFile: cert, KeyFile: key, ResetSecret: "test-secret",
		})
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		chatv1.RegisterChatServiceServer(srv, handler)
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
			handler := &chatStub{members: func(ctx context.Context, req *chatv1.ListMembersRequest) (*chatv1.ListMembersResponse, error) {
				if err := checkMetadata(ctx, true); err != nil {
					return nil, err
				}
				switch req.GetChatId() {
				case "missing":
					return nil, status.Error(codes.NotFound, "missing chat")
				case "broken":
					return nil, status.Error(codes.Unavailable, "unavailable")
				case "empty":
					return &chatv1.ListMembersResponse{}, nil
				default:
					return &chatv1.ListMembersResponse{UserIds: []string{"second", "first"}}, nil
				}
			}}
			c := client(t, tr, "secret", handler)
			for _, tt := range []struct {
				chat, user string
				want       bool
				code       codes.Code
			}{
				{"chat", "first", true, codes.OK}, {"chat", "other", false, codes.OK},
				{"empty", "first", false, codes.OK}, {"missing", "first", false, codes.NotFound},
				{"broken", "first", false, codes.Unavailable},
			} {
				got, err := c.IsMember(ctx, tt.chat, tt.user)
				if got != tt.want || status.Code(err) != tt.code {
					t.Fatalf("%s/%s: %v, %v; want %v, %v", tt.chat, tt.user, got, err, tt.want, tt.code)
				}
			}
			wrong := client(t, tr, "wrong", handler)
			if _, err := wrong.IsMember(ctx, "chat", "first"); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("wrong internal secret: %v", err)
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := c.IsMember(ctx, "chat", "first"); status.Code(err) != codes.Canceled {
				t.Fatalf("call after Close: %v", err)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	for _, tr := range []string{"grpc", "quic"} {
		t.Run(tr, func(t *testing.T) {
			started, done := make(chan struct{}, 1), make(chan struct{}, 1)
			c := client(t, tr, "secret", &chatStub{members: func(ctx context.Context, _ *chatv1.ListMembersRequest) (*chatv1.ListMembersResponse, error) {
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
				go func() { _, err := c.IsMember(ctx, "chat", "user"); result <- err }()
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
