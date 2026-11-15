package main

import (
	"os"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/pkg/rpctransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
)

// newTLSServer exposes the existing handlers on an optional internal TCP port.
func newTLSServer(handler authv1.AuthServiceServer, health healthpb.HealthServer, auth grpc.UnaryServerInterceptor) (*rpctransport.Server, error) {
	port := os.Getenv("GRPC_TLS_PORT")
	if port == "" {
		return nil, nil
	}
	srv, err := rpctransport.NewServer(rpctransport.ServerConfig{
		Addr: ":" + port, CertFile: os.Getenv("QUIC_CERT_FILE"), KeyFile: os.Getenv("QUIC_KEY_FILE"),
		Interceptors: []grpc.UnaryServerInterceptor{
			metrics.UnaryServerInterceptor("auth-service", metrics.Transport(rpctransport.Name)), auth,
		},
	})
	if err != nil {
		return nil, err
	}
	authv1.RegisterAuthServiceServer(srv, handler)
	healthpb.RegisterHealthServer(srv, health)
	return srv, nil
}
