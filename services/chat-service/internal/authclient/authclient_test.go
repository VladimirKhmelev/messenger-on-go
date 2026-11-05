package authclient

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
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
)

type authStub struct {
	authv1.UnimplementedAuthServiceServer
	get func(context.Context, *authv1.GetUserByIDRequest) (*authv1.GetUserByIDResponse, error)
}

func (s *authStub) GetUserByID(ctx context.Context, req *authv1.GetUserByIDRequest) (*authv1.GetUserByIDResponse, error) {
	return s.get(ctx, req)
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

func client(t *testing.T, transport string, handler authv1.AuthServiceServer) *Client {
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
		c, err = Dial(l.Addr().String())
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
		c, err = DialQUIC(net.JoinHostPort("localhost", port), ca)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestUserExists(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	for _, tr := range []string{"grpc", "quic"} {
		t.Run(tr, func(t *testing.T) {
			c := client(t, tr, &authStub{get: func(ctx context.Context, req *authv1.GetUserByIDRequest) (*authv1.GetUserByIDResponse, error) {
				md, _ := metadata.FromIncomingContext(ctx)
				if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer token" {
					return nil, status.Error(codes.Internal, "authorization lost")
				}
				if got := md.Get("traceparent"); len(got) != 1 || !strings.Contains(got[0], sc.TraceID().String()) {
					return nil, status.Error(codes.Internal, "trace context lost")
				}
				switch req.GetUserId() {
				case "found":
					return &authv1.GetUserByIDResponse{UserId: "found"}, nil
				case "missing":
					return nil, status.Error(codes.NotFound, "missing")
				default:
					return nil, status.Error(codes.Unauthenticated, "invalid token")
				}
			}})
			ctx, cancel := context.WithTimeout(trace.ContextWithSpanContext(t.Context(), sc), 5*time.Second)
			defer cancel()
			for _, tt := range []struct {
				id     string
				exists bool
				code   codes.Code
			}{{"found", true, codes.OK}, {"missing", false, codes.OK}, {"denied", false, codes.Unauthenticated}} {
				exists, err := c.UserExists(ctx, "token", tt.id)
				if exists != tt.exists || status.Code(err) != tt.code {
					t.Fatalf("%s: %v, %v; want %v, %v", tt.id, exists, err, tt.exists, tt.code)
				}
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	for _, tr := range []string{"grpc", "quic"} {
		t.Run(tr, func(t *testing.T) {
			started, done := make(chan struct{}, 1), make(chan struct{}, 1)
			c := client(t, tr, &authStub{get: func(ctx context.Context, _ *authv1.GetUserByIDRequest) (*authv1.GetUserByIDResponse, error) {
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
				go func() { _, err := c.UserExists(ctx, "token", "blocked"); result <- err }()
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

func TestCloseDuringQUICHandshake(t *testing.T) {
	ca, _, _ := certificates(t)
	peer, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	_, port, _ := net.SplitHostPort(peer.LocalAddr().String())
	c, err := DialQUIC(net.JoinHostPort("localhost", port), ca)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := c.UserExists(ctx, "token", "pending"); result <- err }()
	_ = peer.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := peer.ReadFrom(make([]byte, 1500)); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; status.Code(err) != codes.Canceled || ctx.Err() != nil {
		t.Fatalf("Close during handshake: %v, caller context %v", err, ctx.Err())
	}
}
