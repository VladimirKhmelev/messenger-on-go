package main

import (
	"os"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/pkg/quictransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
)

// newQUICServer exposes the same handlers and authorization as gRPC when
// QUIC_CERT_FILE is configured. Without it only the gRPC listener is used.
func newQUICServer(addr string, auth authv1.AuthServiceServer, health healthpb.HealthServer, authInterceptor grpc.UnaryServerInterceptor) (*quictransport.Server, error) {
	certFile := os.Getenv("QUIC_CERT_FILE")
	if certFile == "" {
		return nil, nil
	}
	srv, err := quictransport.NewServer(quictransport.ServerConfig{
		Service:     "auth-service",
		Addr:        addr,
		CertFile:    certFile,
		KeyFile:     os.Getenv("QUIC_KEY_FILE"),
		ResetSecret: os.Getenv("QUIC_RESET_SECRET"),
		Interceptors: []grpc.UnaryServerInterceptor{
			metrics.UnaryServerInterceptor("auth-service", metrics.Transport(quictransport.Name)),
			authInterceptor,
		},
	})
	if err != nil {
		return nil, err
	}
	authv1.RegisterAuthServiceServer(srv, auth)
	healthpb.RegisterHealthServer(srv, health)
	return srv, nil
}
