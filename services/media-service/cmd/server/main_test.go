package main

import (
	"strings"
	"testing"
)

func TestDialChatTransport(t *testing.T) {
	for _, transport := range []string{"", "grpc", "quic", "invalid"} {
		t.Run(transport, func(t *testing.T) {
			t.Setenv("CHAT_TRANSPORT", transport)
			t.Setenv("QUIC_CA_FILE", "")
			c, err := dialChat("localhost:1", "secret")
			switch transport {
			case "", "grpc":
				if err != nil {
					t.Fatal(err)
				}
				_ = c.Close()
			case "quic":
				if err == nil || !strings.Contains(err.Error(), "quictransport:") {
					t.Fatalf("QUIC without CA must fail: %v", err)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), "CHAT_TRANSPORT") {
					t.Fatalf("unknown transport: %v", err)
				}
			}
		})
	}
}
