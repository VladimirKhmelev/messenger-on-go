// Package rpctransport provides gRPC over TLS and the TLS policy shared
// with the native QUIC transport.
package rpctransport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
)

// ServerTLS loads a service certificate using the shared TLS 1.3 policy.
func ServerTLS(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("rpctransport: %w", err)
	}
	c := tlsPolicy()
	c.Certificates = []tls.Certificate{cert}
	return c, nil
}

// ClientTLS verifies the server against caFile and the host in addr.
// Addresses must be explicit host:port pairs, without resolver schemes.
func ClientTLS(addr, caFile string) (*tls.Config, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" || port == "" {
		return nil, fmt.Errorf("rpctransport: expected host:port, got %q", addr)
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("rpctransport: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("rpctransport: no certificate in %s", caFile)
	}
	c := tlsPolicy()
	c.RootCAs, c.ServerName = pool, host
	return c, nil
}

func tlsPolicy() *tls.Config {
	return &tls.Config{
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519MLKEM768},
	}
}
