package rpctransport

import (
	"fmt"
	"net"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Name labels gRPC calls protected by TLS in metrics and traces.
const Name = "grpc-tls"

var transportAttr = attribute.String("rpc.transport", Name)

// ServerConfig configures a separate gRPC TLS listener.
type ServerConfig struct {
	Addr, CertFile, KeyFile string
	Interceptors            []grpc.UnaryServerInterceptor
}

// Server serves the same generated handlers as the other RPC listeners.
type Server struct {
	*grpc.Server
	listener net.Listener
}

// NewServer loads credentials and binds the listener. Register handlers
// before Serve. Additional options can set the application's message limits.
func NewServer(c ServerConfig, opts ...grpc.ServerOption) (*Server, error) {
	tlsConfig, err := ServerTLS(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, err
	}
	lis, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return nil, fmt.Errorf("rpctransport: %w", err)
	}
	opts = append(opts,
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithSpanAttributes(transportAttr), otelgrpc.WithMetricAttributes(transportAttr))),
		grpc.ChainUnaryInterceptor(c.Interceptors...),
	)
	return &Server{Server: grpc.NewServer(opts...), listener: lis}, nil
}

// Addr returns the bound TCP address.
func (s *Server) Addr() net.Addr { return s.listener.Addr() }

// Serve accepts calls until GracefulStop or Stop.
func (s *Server) Serve() error { return s.Server.Serve(s.listener) }

// GracefulStop waits for active calls and closes the listener.
func (s *Server) GracefulStop() {
	s.Server.GracefulStop()
	_ = s.listener.Close()
}

// Stop cancels active calls and closes the listener.
func (s *Server) Stop() {
	s.Server.Stop()
	_ = s.listener.Close()
}

// ClientConfig configures a gRPC TLS client with application interceptors.
type ClientConfig struct {
	Addr, CAFile string
	Interceptors []grpc.UnaryClientInterceptor
}

// Dial creates a lazy gRPC client which verifies the server certificate.
// Proxy environment variables are ignored, as for native QUIC connections.
func Dial(c ClientConfig) (*grpc.ClientConn, error) {
	tlsConfig, err := ClientTLS(c.Addr, c.CAFile)
	if err != nil {
		return nil, err
	}
	return grpc.NewClient(c.Addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithNoProxy(),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(
			otelgrpc.WithSpanAttributes(transportAttr), otelgrpc.WithMetricAttributes(transportAttr))),
		grpc.WithChainUnaryInterceptor(c.Interceptors...),
	)
}
