package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/rpctransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	transportgrpc "github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/transport/grpc"
)

func TestTLSAuthorization(t *testing.T) {
	ca := configureQUIC(t)
	t.Setenv("GRPC_TLS_PORT", "0")
	issuer, checker, handler := jwtutil.NewIssuer("test-secret"), new(tokenChecker), new(authHandler)
	srv, err := newTLSServer(handler, health.NewServer(), transportgrpc.AuthInterceptor(issuer, checker, "internal-secret"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	go func() { _ = srv.Serve() }()
	_, port, _ := net.SplitHostPort(srv.Addr().String())
	cc, err := rpctransport.Dial(rpctransport.ClientConfig{Addr: net.JoinHostPort("localhost", port), CAFile: ca})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	token, err := issuer.IssueAccessToken("caller")
	if err != nil {
		t.Fatal(err)
	}
	client := authv1.NewAuthServiceClient(cc)
	for _, tc := range []struct {
		token string
		stale bool
		code  codes.Code
	}{
		{"", false, codes.Unauthenticated}, {"invalid", false, codes.Unauthenticated},
		{token, false, codes.OK}, {token, true, codes.Unauthenticated},
	} {
		checker.stale.Store(tc.stale)
		before := handler.calls.Load()
		call := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tc.token)
		resp, err := client.GetUserByID(call, &authv1.GetUserByIDRequest{UserId: "caller"})
		if status.Code(err) != tc.code {
			t.Fatalf("authorization: %v, want %s", err, tc.code)
		}
		if tc.code == codes.OK {
			if resp.GetUserId() != "caller" || handler.calls.Load() != before+1 {
				t.Fatal("caller identity lost")
			}
		} else if handler.calls.Load() != before {
			t.Fatal("unauthorized call reached handler")
		}
	}
	for _, secret := range []string{"wrong", "internal-secret"} {
		call := metadata.AppendToOutgoingContext(ctx, "x-internal-secret", secret)
		_, err := client.ListPushSubscriptions(call, &authv1.ListPushSubscriptionsRequest{})
		want := codes.Unauthenticated
		if secret == "internal-secret" {
			want = codes.OK
		}
		if status.Code(err) != want {
			t.Fatalf("internal authorization: %v", err)
		}
	}
}

func TestTLSConfiguration(t *testing.T) {
	t.Setenv("GRPC_TLS_PORT", "")
	if srv, err := newTLSServer(nil, nil, nil); srv != nil || err != nil {
		t.Fatalf("disabled TLS: %v, %v", srv, err)
	}
	t.Setenv("GRPC_TLS_PORT", "0")
	t.Setenv("QUIC_CERT_FILE", "missing-cert.pem")
	if srv, err := newTLSServer(nil, nil, nil); srv != nil || err == nil {
		t.Fatal("TLS without certificate accepted")
	}
}
