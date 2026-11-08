package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/quictransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
	transportgrpc "github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/transport/grpc"
)

// Health must be implemented by the application service as well as the
// standard grpc.health service, including through its generated HTTP route.
func TestAuthHealthThroughHTTPGateway(t *testing.T) {
	for _, transport := range []string{"grpc", "quic"} {
		t.Run(transport, func(t *testing.T) {
			auth := transportgrpc.NewAuthServer(nil, false)
			interceptor := transportgrpc.AuthInterceptor(nil, nil, "test-internal-secret")
			var cc grpc.ClientConnInterface
			if transport == "grpc" {
				lis, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				srv := grpc.NewServer(grpc.UnaryInterceptor(interceptor))
				authv1.RegisterAuthServiceServer(srv, auth)
				go func() { _ = srv.Serve(lis) }()
				t.Cleanup(srv.Stop)
				conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				cc = conn
			} else {
				ca := configureQUIC(t)
				srv, err := newQUICServer("127.0.0.1:0", auth, health.NewServer(), interceptor)
				if err != nil {
					t.Fatal(err)
				}
				go func() { _ = srv.Serve() }()
				t.Cleanup(srv.Stop)
				_, port, _ := net.SplitHostPort(srv.Addr().String())
				conn, err := quictransport.Dial(quictransport.ClientConfig{Addr: net.JoinHostPort("localhost", port), CAFile: ca})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				cc = conn
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			mux := runtime.NewServeMux()
			if err := authv1.RegisterAuthServiceHandlerClient(ctx, mux, authv1.NewAuthServiceClient(cc)); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequestWithContext(ctx, "GET", "/v1/auth/health", nil)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			var response struct {
				OK bool `json:"ok"`
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("public health route: HTTP %d: %s", recorder.Code, recorder.Body)
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !response.OK {
				t.Fatalf("health response: %s, %v", recorder.Body, err)
			}
		})
	}
}
