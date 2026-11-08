package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestComposeUsesIsolatedProjectAndExplicitEnvironment(t *testing.T) {
	var calls [][]string
	c := &compose{root: "/repo", work: "/work", project: "wisply-e2e-abc-grpc", transport: "grpc", exec: func(_ context.Context, args, env []string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		for _, required := range []string{"E2E_BIN_DIR=/work/bin", "E2E_CERTS_DIR=/work/certs", "E2E_TRANSPORT=grpc"} {
			if !slices.Contains(env, required) {
				t.Errorf("missing %s", required)
			}
		}
		return []byte("127.0.0.1:42345\n"), nil
	}}
	addr, err := c.endpoint(t.Context(), "chat-service", "8080")
	if err != nil || addr != "http://127.0.0.1:42345" {
		t.Fatalf("endpoint = %s, %v", addr, err)
	}
	want := []string{"docker-compose", "--env-file", "/dev/null", "-p", "wisply-e2e-abc-grpc", "-f", "/repo/test/e2e/compose.yaml", "port", "chat-service", "8080"}
	if len(calls) != 1 || !slices.Equal(calls[0], want) {
		t.Fatalf("calls = %v", calls)
	}
}
func TestComposeRejectsExposedOrAmbiguousPorts(t *testing.T) {
	for _, out := range []string{"0.0.0.0:8080", "[::]:8080", "127.0.0.1:1\n127.0.0.1:2", ""} {
		c := &compose{exec: func(context.Context, []string, []string) ([]byte, error) { return []byte(out), nil }}
		if _, err := c.endpoint(t.Context(), "chat-service", "8080"); err == nil {
			t.Errorf("accepted %q", out)
		}
	}
}
func TestFaultsStayInTargetNamespace(t *testing.T) {
	var calls [][]string
	c := &compose{project: "wisply-e2e-test-quic", exec: func(_ context.Context, args, _ []string) ([]byte, error) {
		calls = append(calls, slices.Clone(args))
		if args[0] == "docker-compose" {
			return []byte(strings.Repeat("a", 64)), nil
		}
		return nil, nil
	}}
	if err := c.impair(t.Context(), "chat-service", "blackhole"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v", calls)
	}
	want := []string{"docker", "run", "--rm", "--network", "container:" + strings.Repeat("a", 64), "--cap-add", "NET_ADMIN", "wisply-e2e-netem", "sh", "-ec"}
	if !slices.Equal(calls[1][:len(want)], want) {
		t.Fatal(calls[1])
	}
	script := calls[1][len(want)]
	if !strings.Contains(script, "sport 50051") || !strings.Contains(script, "loss 100%") || strings.Contains(script, "host") {
		t.Fatal(script)
	}
	calls = nil
	for _, pair := range [][2]string{{"postgres", "off"}, {"chat-service", "unknown"}} {
		if err := c.impair(t.Context(), pair[0], pair[1]); err == nil {
			t.Fatal("unsafe fault accepted")
		}
	}
	if len(calls) != 0 {
		t.Fatal("invalid fault executed a command")
	}
	c.exec = func(context.Context, []string, []string) ([]byte, error) { return []byte("not-a-container"), nil }
	if err := c.impair(t.Context(), "auth-service", "off"); err == nil {
		t.Fatal("invalid ID accepted")
	}
}
func TestRestartAndFaultCleanup(t *testing.T) {
	var calls [][]string
	c := &compose{exec: func(_ context.Context, args, _ []string) ([]byte, error) {
		calls = append(calls, slices.Clone(args))
		if slices.Contains(args, "ps") {
			return []byte(strings.Repeat("a", 64)), nil
		}
		return nil, nil
	}}
	if err := c.restart(t.Context(), "auth-service", false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls[0][len(calls[0])-4:], []string{"stop", "-t", "15", "auth-service"}) {
		t.Fatal(calls)
	}
	calls = nil
	if err := c.restart(t.Context(), "chat-service", true); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls[0][len(calls[0])-4:], []string{"kill", "-s", "KILL", "chat-service"}) {
		t.Fatal(calls)
	}
	failure := errors.New("probe failed")
	s := &suite{control: c}
	calls = nil
	if err := s.withFault(t.Context(), "chat-service", "delay-loss", func() error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if len(calls) != 4 || !strings.Contains(calls[1][len(calls[1])-1], "delay 50ms") || strings.Contains(calls[3][len(calls[3])-1], "qdisc add") {
		t.Fatalf("fault not restored: %v", calls)
	}
}

func TestRestartRefreshesPublishedHTTPPort(t *testing.T) {
	c := &compose{exec: func(_ context.Context, args, _ []string) ([]byte, error) {
		if slices.Contains(args, "port") {
			return []byte("127.0.0.1:41234"), nil
		}
		return nil, nil
	}}
	s := &suite{control: c, endpoints: endpoints{auth: "http://127.0.0.1:30001", chat: "http://127.0.0.1:30002"}}
	if err := s.restartService(t.Context(), "auth-service", false); err != nil {
		t.Fatal(err)
	}
	if s.auth != "http://127.0.0.1:41234" || s.chat != "http://127.0.0.1:30002" {
		t.Fatalf("endpoints not refreshed correctly: %+v", s.endpoints)
	}
	if err := s.restartService(t.Context(), "chat-service", true); err != nil {
		t.Fatal(err)
	}
	if s.chat != "http://127.0.0.1:41234" {
		t.Fatal("old chat port retained")
	}
}
