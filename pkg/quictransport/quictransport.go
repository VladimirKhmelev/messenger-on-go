// Package quictransport connects the services over QUIC with quic-rpc-go
// (github.com/VladimirKhmelev/quic-rpc-go), next to gRPC. A server serves
// the same services over UDP on the port number of its gRPC server; a client
// chooses the transport by its settings. The calls carry TLS 1.3 with the
// certificates that scripts/quic-certs.sh issues, are traced by otelgrpc
// with the span attribute rpc.transport=quic and are counted by pkg/metrics
// with the label transport="quic".
package quictransport

import (
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/quic-go/quic-go"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"

	"github.com/VladimirKhmelev/quic-rpc-go/grpcbridge"
	rpcquic "github.com/VladimirKhmelev/quic-rpc-go/rpc/transport/quic"
)

// Name is the transport in metrics and traces.
const Name = "quic"

// transportAttr marks the spans and the metrics of otelgrpc.
var transportAttr = attribute.String("rpc.transport", Name)

// maxStreams limits the calls in flight on one connection. grpc-go sets no
// such limit; quic-go's default of 100 would make QUIC queue calls that gRPC
// runs at once.
const maxStreams = 1 << 16

// keepAlive is how often a client pings an idle connection. Without it
// quic-go closes the connection after 30 s of silence and the next call pays
// for a new handshake, which a gRPC connection does not; the pings also
// reveal a restarted server before a call does.
const keepAlive = 10 * time.Second

// ServerConfig configures a server.
type ServerConfig struct {
	// Service names the service; its stateless reset key derives from it.
	Service string
	// Addr is the UDP address to listen on, such as ":50051".
	Addr string
	// CertFile and KeyFile hold the certificate of the service and its key.
	CertFile, KeyFile string
	// ResetSecret is the secret the stateless reset key derives from.
	ResetSecret string
	// Interceptors wrap the calls, the first outermost.
	Interceptors []grpc.UnaryServerInterceptor
}

// Server serves services registered by generated gRPC code over QUIC, on a
// UDP socket of its own.
type Server struct {
	*grpcbridge.Server
	pc *net.UDPConn
	tr *quic.Transport
	l  *rpcquic.Listener
}

// NewServer listens on c.Addr. Register the services on the server, then
// call Serve.
func NewServer(c ServerConfig) (*Server, error) {
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("quictransport: %w", err)
	}
	key, err := ResetKey(c.ResetSecret, c.Service)
	if err != nil {
		return nil, err
	}
	addr, err := net.ResolveUDPAddr("udp", c.Addr)
	if err != nil {
		return nil, fmt.Errorf("quictransport: %w", err)
	}
	pc, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("quictransport: %w", err)
	}
	tr := &quic.Transport{Conn: pc, StatelessResetKey: key}
	tlsConf := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
	l, err := rpcquic.NewListener(tr, tlsConf, &quic.Config{MaxIncomingStreams: maxStreams})
	if err != nil {
		_ = tr.Close()
		_ = pc.Close()
		return nil, fmt.Errorf("quictransport: %w", err)
	}
	srv := grpcbridge.NewServer(
		grpcbridge.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithSpanAttributes(transportAttr), otelgrpc.WithMetricAttributes(transportAttr))),
		grpcbridge.ChainUnaryInterceptor(c.Interceptors...),
	)
	return &Server{Server: srv, pc: pc, tr: tr, l: l}, nil
}

// Addr returns the address the server listens on.
func (s *Server) Addr() net.Addr { return s.pc.LocalAddr() }

// Serve serves until GracefulStop or Stop, and returns nil then.
func (s *Server) Serve() error { return s.Server.Serve(s.l) }

// GracefulStop lets the calls in flight finish, waiting at most
// grpcbridge.DefaultGracefulStopTimeout, and closes the socket.
func (s *Server) GracefulStop() {
	s.Server.GracefulStop()
	s.close()
}

// Stop stops at once and closes the socket.
func (s *Server) Stop() {
	s.Server.Stop()
	s.close()
}

func (s *Server) close() {
	_ = s.tr.Close()
	_ = s.pc.Close()
}

// ResetKey derives the stateless reset key of a service from secret. It is
// the same on every start, so that the clients of a restarted service close
// their old connections at once instead of after the idle timeout, and it
// differs from service to service.
func ResetKey(secret, service string) (*quic.StatelessResetKey, error) {
	if secret == "" {
		return nil, errors.New("quictransport: no stateless reset secret")
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, "messenger quic stateless reset "+service, len(quic.StatelessResetKey{}))
	if err != nil {
		return nil, fmt.Errorf("quictransport: %w", err)
	}
	return (*quic.StatelessResetKey)(key), nil
}

// ClientConfig configures a client.
type ClientConfig struct {
	// Addr is the server's address, such as "chat-service:50051"; its host
	// must be the name in the server's certificate.
	Addr string
	// CAFile holds the certificate of the CA that issued the server's.
	CAFile string
	// Interceptors wrap the calls, the first outermost.
	Interceptors []grpc.UnaryClientInterceptor
}

// Dial returns a client for generated gRPC code that calls the server at
// c.Addr over QUIC. It connects on the first call and again after the
// connection is lost.
func Dial(c ClientConfig) (*grpcbridge.ClientConn, error) {
	pem, err := os.ReadFile(c.CAFile)
	if err != nil {
		return nil, fmt.Errorf("quictransport: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("quictransport: no certificate in %s", c.CAFile)
	}
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return nil, fmt.Errorf("quictransport: %w", err)
	}
	d := &rpcquic.Dialer{
		Addr:       c.Addr,
		TLSConfig:  &tls.Config{RootCAs: pool, ServerName: host, MinVersion: tls.VersionTLS13},
		QUICConfig: &quic.Config{KeepAlivePeriod: keepAlive},
	}
	return grpcbridge.NewClient(d,
		grpcbridge.WithStatsHandler(otelgrpc.NewClientHandler(
			otelgrpc.WithSpanAttributes(transportAttr), otelgrpc.WithMetricAttributes(transportAttr))),
		grpcbridge.WithChainUnaryInterceptor(c.Interceptors...),
	), nil
}
