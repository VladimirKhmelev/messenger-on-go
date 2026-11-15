package main

import "testing"

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
