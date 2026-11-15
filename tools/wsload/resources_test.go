package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type resourceTransport func(*http.Request) (*http.Response, error)

func (f resourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResourcesUseCountersAndRedactEnvironment(t *testing.T) {
	project := "wisply-load-aaaaaaaaaaaa-grpc-tls"
	client := &http.Client{Transport: resourceTransport(func(r *http.Request) (*http.Response, error) {
		var value any
		switch {
		case r.URL.Path == "/containers/json":
			var list []map[string]any
			for i := range 13 {
				list = append(list, map[string]any{"ID": fmt.Sprint(i), "Labels": map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": fmt.Sprint(i)}})
			}
			value = list
		case strings.HasSuffix(r.URL.Path, "/json"):
			value = map[string]any{"Image": "sha256:image", "HostConfig": map[string]any{"Memory": 123, "NanoCpus": 500000000}, "Config": map[string]any{"Env": []string{"JWT_SECRET=private-secret", "FANOUT_SECRET=private-secret", "GOMAXPROCS=2", "FANOUT_WORKERS=16"}}}
		default:
			if r.URL.Query().Get("one-shot") != "true" {
				t.Error("stats would wait for an interval")
			}
			value = map[string]any{"read": time.Now().UTC(), "cpu_stats": map[string]any{"cpu_usage": map[string]uint64{"total_usage": 9876543210}}, "memory_stats": map[string]int{"usage": 1234}, "networks": map[string]any{"eth0": map[string]int{"rx_bytes": 10, "tx_bytes": 20}}}
		}
		b, _ := json.Marshal(value)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}
	containers, err := discoverContainers(t.Context(), client, project)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(containers)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "private-secret") {
		t.Fatal("environment leaked")
	}
	sample, err := sampleBoundary(t.Context(), client, containers, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range sample.Containers {
		if c.CPUTimeNS != 9876543210 || c.MemoryBytes != 1234 || c.RxBytes != 10 || c.TxBytes != 20 || c.Before.IsZero() || c.After.Before(c.Before) {
			t.Fatal(c)
		}
	}
	if _, err := discoverContainers(context.Background(), client, "production"); err == nil {
		t.Fatal("foreign resource project accepted")
	}
}

func TestCPUParserPreservesAllFields(t *testing.T) {
	cpu, err := readCPU(strings.NewReader("cpu 1 2 3 4 5 6 7 8 9 10\ncpu0 10 9 8 7 6 5 4 3 2 1\nintr 15\n"))
	if err != nil || len(cpu) != 2 || cpu["cpu0"][6] != 4 {
		t.Fatalf("CPU: %v %v", cpu, err)
	}
	if _, err := readCPU(strings.NewReader("cpu 1 2")); err == nil {
		t.Fatal("short CPU accepted")
	}
}
