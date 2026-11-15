package main

import (
	"strings"
	"testing"
)

func TestDialAuthTransport(t *testing.T) {
	for _, transport := range []string{"", "grpc", "grpc-tls", "quic", "invalid"} {
		t.Run(transport, func(t *testing.T) {
			t.Setenv("AUTH_TRANSPORT", transport)
			t.Setenv("AUTH_SERVICE_TLS_ADDR", "")
			t.Setenv("QUIC_CA_FILE", "")
			client, err := dialAuth("localhost:1")
			switch transport {
			case "", "grpc":
				if err != nil {
					t.Fatal(err)
				}
				_ = client.Close()
			case "grpc-tls":
				if err == nil {
					t.Fatal("TLS without endpoint must fail")
				}
			case "quic":
				if err == nil || !strings.Contains(err.Error(), "quictransport:") {
					t.Fatalf("QUIC without CA must fail, got %v", err)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), "AUTH_TRANSPORT") {
					t.Fatalf("unknown transport: %v", err)
				}
			}
		})
	}
}
