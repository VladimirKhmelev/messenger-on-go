package quictransport

import (
	"bytes"
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
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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

// serve starts a server with the health service on a free UDP port of
// localhost, counting its calls.
func serve(t *testing.T, certFile, keyFile string, calls *atomic.Int32) *Server {
	t.Helper()
	t.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	count := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		calls.Add(1)
		return handler(ctx, req)
	}
	srv, err := NewServer(ServerConfig{
		Service: "test-service", Addr: "127.0.0.1:0", CertFile: certFile, KeyFile: keyFile,
		ResetSecret: "secret", Interceptors: []grpc.UnaryServerInterceptor{count},
	})
	if err != nil {
		t.Fatal(err)
	}
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve() }()
	t.Cleanup(srv.Stop)
	return srv
}

// port returns the UDP port the server listens on.
func port(s *Server) string { return strconv.Itoa(s.Addr().(*net.UDPAddr).Port) } //nolint:forcetypeassert // a UDP socket

// TestCall makes a call over QUIC through both chains of interceptors.
func TestCall(t *testing.T) {
	caFile, certFile, keyFile := issue(t, t.TempDir(), "localhost")
	var serverCalls, clientCalls atomic.Int32
	srv := serve(t, certFile, keyFile, &serverCalls)
	count := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		clientCalls.Add(1)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	cc, err := Dial(ClientConfig{Addr: "localhost:" + port(srv), CAFile: caFile, Interceptors: []grpc.UnaryClientInterceptor{count}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cc.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("Check = %v, %v", resp, err)
	}
	if serverCalls.Load() != 1 || clientCalls.Load() != 1 {
		t.Errorf("interceptors saw %d calls on the server, %d on the client", serverCalls.Load(), clientCalls.Load())
	}
}

// TestUntrustedServer: a client that trusts another CA does not talk to the
// server.
func TestUntrustedServer(t *testing.T) {
	_, certFile, keyFile := issue(t, t.TempDir(), "localhost")
	otherCA, _, _ := issue(t, t.TempDir(), "localhost")
	var calls atomic.Int32
	srv := serve(t, certFile, keyFile, &calls)
	cc, err := Dial(ClientConfig{Addr: "localhost:" + port(srv), CAFile: otherCA})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cc.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{}); status.Code(err) != codes.Unavailable || calls.Load() != 0 {
		t.Fatalf("Check with an untrusted server: %v, %d calls served", err, calls.Load())
	}
}

// TestGracefulStopFreesThePort: a stopped server gives its UDP port back, so
// that a restarted one can listen on it.
func TestGracefulStopFreesThePort(t *testing.T) {
	_, certFile, keyFile := issue(t, t.TempDir(), "localhost")
	var calls atomic.Int32
	srv := serve(t, certFile, keyFile, &calls)
	addr := srv.Addr().String()
	srv.GracefulStop()
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("listen on the port of the stopped server: %v", err)
	}
	_ = pc.Close()
}

func TestResetKey(t *testing.T) {
	a, err := ResetKey("secret", "chat-service")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := ResetKey("secret", "chat-service")
	other, _ := ResetKey("secret", "auth-service")
	otherSecret, _ := ResetKey("another", "chat-service")
	if *a != *again || *a == *other || *a == *otherSecret {
		t.Error("the key must depend on the secret and the service, and only on them")
	}
	if _, err := ResetKey("", "chat-service"); err == nil {
		t.Error("ResetKey without a secret succeeded")
	}
}

// TestCertsScript runs scripts/quic-certs.sh: what it issues is what Go's
// TLS takes, a second run changes nothing and a new service gets only its
// own certificate.
func TestCertsScript(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("no openssl")
	}
	dir := t.TempDir()
	run := func(servers, clients string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "sh", "../../scripts/quic-certs.sh")
		cmd.Env = append(os.Environ(), "CERTS_DIR="+dir, "QUIC_SERVERS="+servers, "QUIC_CLIENTS="+clients)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("quic-certs.sh: %v\n%s", err, out)
		}
	}
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // the test's own files
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	run("chat-service", "ws-gateway")
	cert := read("chat-service/cert.pem")
	if !bytes.Equal(read("ws-gateway/ca.pem"), read("ca/ca.pem")) || !bytes.Equal(read("chat-service/ca.pem"), read("ca/ca.pem")) {
		t.Error("a service has another CA certificate")
	}
	pair, err := tls.X509KeyPair(cert, read("chat-service/key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(read("ca/ca.pem"))
	if _, err := pair.Leaf.Verify(x509.VerifyOptions{DNSName: "chat-service", Roots: pool}); err != nil {
		t.Errorf("the certificate of chat-service: %v", err)
	}

	run("chat-service auth-service", "ws-gateway")
	if !bytes.Equal(read("chat-service/cert.pem"), cert) {
		t.Error("a second run reissued the certificate of chat-service")
	}
	if _, err := tls.X509KeyPair(read("auth-service/cert.pem"), read("auth-service/key.pem")); err != nil {
		t.Errorf("the certificate of a service added later: %v", err)
	}
}
