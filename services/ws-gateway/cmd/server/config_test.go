package main

import (
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/events"
)

func TestFanoutConfig(t *testing.T) {
	for _, key := range []string{"FANOUT_WORKERS", "FANOUT_QUEUE_SIZE", "FANOUT_TIMEOUT"} {
		t.Setenv(key, "")
	}
	if cfg, err := fanoutConfig(); err != nil || cfg != events.DefaultConfig() {
		t.Fatalf("defaults = %+v, %v", cfg, err)
	}
	t.Setenv("FANOUT_WORKERS", "3")
	t.Setenv("FANOUT_QUEUE_SIZE", "7")
	t.Setenv("FANOUT_TIMEOUT", "2s")
	if cfg, err := fanoutConfig(); err != nil || cfg != (events.Config{Workers: 3, QueueSize: 7, Timeout: 2 * time.Second}) {
		t.Fatalf("configured = %+v, %v", cfg, err)
	}
	for _, tc := range []struct{ key, value string }{{"FANOUT_WORKERS", "x"}, {"FANOUT_QUEUE_SIZE", "-1"}, {"FANOUT_TIMEOUT", "0s"}, {"FANOUT_TIMEOUT", "x"}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := fanoutConfig(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
