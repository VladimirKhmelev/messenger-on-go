package rpctransport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// issue writes a CA certificate and a certificate with its key for host
// into dir, as scripts/quic-certs.sh does, and returns the file names.
func issue(t *testing.T, dir, host string) (caFile, certFile, keyFile string) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caTmpl, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	caFile, certFile, keyFile = filepath.Join(dir, "ca.pem"), filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	for file, block := range map[string]*pem.Block{
		caFile: {Type: "CERTIFICATE", Bytes: caDER}, certFile: {Type: "CERTIFICATE", Bytes: der}, keyFile: {Type: "PRIVATE KEY", Bytes: keyDER},
	} {
		if err := os.WriteFile(file, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return caFile, certFile, keyFile
}

func TestTLSAndInterceptors(t *testing.T) {
	ca, cert, key := issue(t, t.TempDir(), "localhost")
	var serverCalls, clientCalls atomic.Int32
	srv, err := NewServer(ServerConfig{Addr: "127.0.0.1:0", CertFile: cert, KeyFile: key,
		Interceptors: []grpc.UnaryServerInterceptor{func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			p, ok := peer.FromContext(ctx)
			if !ok {
				return nil, status.Error(codes.Internal, "no peer")
			}
			ti, ok := p.AuthInfo.(credentials.TLSInfo)
			if !ok || ti.State.Version != tls.VersionTLS13 || ti.State.NegotiatedProtocol != "h2" {
				return nil, status.Error(codes.Internal, "wrong TLS policy")
			}
			serverCalls.Add(1)
			return next(ctx, req)
		}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve() }()
	_, port, _ := net.SplitHostPort(srv.Addr().String())
	cc, err := Dial(ClientConfig{Addr: net.JoinHostPort("localhost", port), CAFile: ca,
		Interceptors: []grpc.UnaryClientInterceptor{func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, next grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			clientCalls.Add(1)
			return next(ctx, method, req, reply, cc, opts...)
		}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING || serverCalls.Load() != 1 || clientCalls.Load() != 1 {
		t.Fatalf("health: %v %v, calls %d/%d", resp, err, serverCalls.Load(), clientCalls.Load())
	}
	addr := srv.Addr().String()
	srv.GracefulStop()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
}

func TestCertificateAndPolicyRejection(t *testing.T) {
	ca, cert, key := issue(t, t.TempDir(), "localhost")
	other, _, _ := issue(t, t.TempDir(), "localhost")
	var calls atomic.Int32
	srv, err := NewServer(ServerConfig{Addr: "127.0.0.1:0", CertFile: cert, KeyFile: key, Interceptors: []grpc.UnaryServerInterceptor{
		func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			calls.Add(1)
			return next(ctx, req)
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve() }()
	_, port, _ := net.SplitHostPort(srv.Addr().String())
	for _, tc := range []struct{ name, host, ca string }{{"other CA", "localhost", other}, {"wrong hostname", "127.0.0.1", ca}} {
		t.Run(tc.name, func(t *testing.T) {
			cc, err := Dial(ClientConfig{Addr: net.JoinHostPort(tc.host, port), CAFile: tc.ca})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cc.Close() }()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			_, err = healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{})
			if status.Code(err) != codes.Unavailable || calls.Load() != 0 {
				t.Fatalf("untrusted peer reached handler: %v, calls=%d", err, calls.Load())
			}
		})
	}
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		cfg, err := ClientTLS(net.JoinHostPort("localhost", port), ca)
		if err != nil {
			t.Fatal(err)
		}
		cfg.MinVersion, cfg.MaxVersion = version, version
		if version == tls.VersionTLS13 {
			cfg.CurvePreferences = []tls.CurveID{tls.X25519}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		conn, err := (&tls.Dialer{Config: cfg}).DialContext(ctx, "tcp", srv.Addr().String())
		cancel()
		if err == nil {
			_ = conn.Close()
			t.Fatalf("accepted incompatible TLS policy: version=%x", version)
		}
	}
}

func TestConfiguration(t *testing.T) {
	ca, cert, key := issue(t, t.TempDir(), "localhost")
	s, err := ServerTLS(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ClientTLS("localhost:50052", ca)
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []*tls.Config{s, c} {
		if cfg.MinVersion != tls.VersionTLS13 || cfg.MaxVersion != tls.VersionTLS13 || !slices.Equal(cfg.CurvePreferences, []tls.CurveID{tls.X25519MLKEM768}) || cfg.InsecureSkipVerify {
			t.Fatal("TLS policy changed")
		}
	}
	for _, addr := range []string{"", ":50052", "localhost", "localhost:", "dns:///localhost:50052"} {
		if _, err := ClientTLS(addr, ca); err == nil {
			t.Errorf("invalid address accepted: %s", addr)
		}
	}
	empty := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(empty, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, caFile := range []string{"", empty} {
		if _, err := Dial(ClientConfig{Addr: "localhost:50052", CAFile: caFile}); err == nil {
			t.Fatal("missing or invalid CA accepted")
		}
	}
	if _, err := NewServer(ServerConfig{Addr: "127.0.0.1:0", CertFile: cert, KeyFile: "missing"}); err == nil {
		t.Fatal("missing key accepted")
	}
}
