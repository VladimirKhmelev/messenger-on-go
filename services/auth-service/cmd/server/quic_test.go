package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/quictransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	transportgrpc "github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/transport/grpc"
)

type tokenChecker struct{ stale atomic.Bool }

func (c *tokenChecker) IsAccessTokenStale(context.Context, string, time.Time) (bool, error) {
	return c.stale.Load(), nil
}

type authHandler struct {
	authv1.UnimplementedAuthServiceServer
	calls atomic.Int32
}

func (h *authHandler) GetUserByID(ctx context.Context, _ *authv1.GetUserByIDRequest) (*authv1.GetUserByIDResponse, error) {
	h.calls.Add(1)
	user, _ := transportgrpc.UserIDFromContext(ctx)
	return &authv1.GetUserByIDResponse{UserId: user}, nil
}

func (h *authHandler) ListPushSubscriptions(context.Context, *authv1.ListPushSubscriptionsRequest) (*authv1.ListPushSubscriptionsResponse, error) {
	h.calls.Add(1)
	return &authv1.ListPushSubscriptionsResponse{}, nil
}

func configureQUIC(t *testing.T) string {
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
	t.Setenv("QUIC_CERT_FILE", filepath.Join(dir, "localhost/cert.pem"))
	t.Setenv("QUIC_KEY_FILE", filepath.Join(dir, "localhost/key.pem"))
	t.Setenv("QUIC_RESET_SECRET", "test-reset-secret")
	return filepath.Join(dir, "localhost/ca.pem")
}

func TestQUICAuthorization(t *testing.T) {
	ca := configureQUIC(t)
	issuer, checker, handler := jwtutil.NewIssuer("test-secret"), new(tokenChecker), new(authHandler)
	srv, err := newQUICServer("127.0.0.1:0", handler, health.NewServer(), transportgrpc.AuthInterceptor(issuer, checker, "internal-secret"))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(srv.Stop)
	_, port, _ := net.SplitHostPort(srv.Addr().String())
	cc, err := quictransport.Dial(quictransport.ClientConfig{Addr: net.JoinHostPort("localhost", port), CAFile: ca})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health without authentication: %v, %v", resp, err)
	}
	token, err := issuer.IssueAccessToken("caller")
	if err != nil {
		t.Fatal(err)
	}
	client := authv1.NewAuthServiceClient(cc)
	for _, tt := range []struct {
		name, token string
		stale       bool
		want        codes.Code
	}{
		{"missing", "", false, codes.Unauthenticated},
		{"invalid", "invalid", false, codes.Unauthenticated},
		{"valid", token, false, codes.OK},
		{"stale", token, true, codes.Unauthenticated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checker.stale.Store(tt.stale)
			before := handler.calls.Load()
			ctx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tt.token)
			resp, err := client.GetUserByID(ctx, &authv1.GetUserByIDRequest{UserId: "caller"})
			if status.Code(err) != tt.want {
				t.Fatalf("GetUserByID: %v, want %v", err, tt.want)
			}
			if tt.want == codes.OK {
				if resp.GetUserId() != "caller" || handler.calls.Load() != before+1 {
					t.Fatal("authorized handler lost the caller identity")
				}
			} else if handler.calls.Load() != before {
				t.Fatal("unauthorized call reached the handler")
			}
		})
	}
	for _, secret := range []string{"wrong", "internal-secret"} {
		ctx := metadata.AppendToOutgoingContext(ctx, "x-internal-secret", secret)
		_, err := client.ListPushSubscriptions(ctx, &authv1.ListPushSubscriptionsRequest{})
		want := codes.Unauthenticated
		if secret == "internal-secret" {
			want = codes.OK
		}
		if status.Code(err) != want {
			t.Fatalf("internal authentication: %v, want %v", err, want)
		}
	}
	// Keep the reset key and certificate across a real server restart.
	checker.stale.Store(false)
	addr := srv.Addr().String()
	srv.Stop()
	restarted, err := newQUICServer(addr, handler, health.NewServer(), transportgrpc.AuthInterceptor(issuer, checker, "internal-secret"))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = restarted.Serve() }()
	t.Cleanup(restarted.Stop)
	for {
		attempt, stop := context.WithTimeout(ctx, time.Second)
		attempt = metadata.AppendToOutgoingContext(attempt, "authorization", "Bearer "+token)
		resp, err := client.GetUserByID(attempt, &authv1.GetUserByIDRequest{UserId: "caller"})
		stop()
		if err == nil && resp.GetUserId() == "caller" {
			break
		}
		if ctx.Err() != nil || (status.Code(err) != codes.Unavailable && status.Code(err) != codes.DeadlineExceeded) {
			t.Fatalf("reconnect after restart: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	restarted.GracefulStop()
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("graceful stop did not free UDP: %v", err)
	}
	_ = pc.Close()
}

func TestQUICConfiguration(t *testing.T) {
	t.Setenv("QUIC_CERT_FILE", "")
	if srv, err := newQUICServer("127.0.0.1:0", nil, nil, nil); err != nil || srv != nil {
		t.Fatalf("QUIC without certificates: %v, %v", srv, err)
	}
	configureQUIC(t)
	t.Setenv("QUIC_KEY_FILE", "missing-key.pem")
	if srv, err := newQUICServer("127.0.0.1:0", nil, nil, nil); err == nil || srv != nil {
		t.Fatal("invalid TLS configuration was accepted")
	}
}
