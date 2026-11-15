package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type resourceContainer struct {
	ID          string            `json:"id"`
	Service     string            `json:"service"`
	Image       string            `json:"image_id"`
	MemoryLimit int64             `json:"memory_limit_bytes"`
	NanoCPUs    int64             `json:"cpu_limit_nanocpus"`
	CPUSet      string            `json:"cpuset"`
	Runtime     map[string]string `json:"runtime_settings"`
}
type resourceSample struct {
	Service     string    `json:"service"`
	Before      time.Time `json:"before"`
	After       time.Time `json:"after"`
	EngineRead  time.Time `json:"engine_read"`
	CPUTimeNS   uint64    `json:"cpu_time_ns"`
	MemoryBytes uint64    `json:"memory_bytes"`
	RxBytes     uint64    `json:"rx_bytes"`
	TxBytes     uint64    `json:"tx_bytes"`
}
type resourceBoundary struct {
	LoadProcessTicks [2]uint64           `json:"load_process_cpu_ticks"`
	Target           time.Time           `json:"target"`
	Before           time.Time           `json:"before"`
	After            time.Time           `json:"after"`
	CPU              map[string][]uint64 `json:"host_cpu_ticks"`
	Containers       []resourceSample    `json:"containers"`
}
type resourceReport struct {
	Kernel             string              `json:"kernel"`
	CPUCount           int                 `json:"logical_cpus"`
	HostNetwork        map[string]string   `json:"host_network_settings"`
	ProcTicksPerSecond int64               `json:"proc_ticks_per_second"`
	Schema             int                 `json:"schema"`
	Go                 string              `json:"go_version"`
	OS                 string              `json:"os"`
	Architecture       string              `json:"architecture"`
	Containers         []resourceContainer `json:"containers"`
	Boundaries         []resourceBoundary  `json:"boundaries"`
	Complete           bool                `json:"complete"`
}

var ownedProject = regexp.MustCompile(`^wisply-load-[a-f0-9]{12}-(grpc|grpc-tls|quic)$`)

func dockerClient() (*http.Client, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	if !strings.HasPrefix(host, "unix://") {
		return nil, fmt.Errorf("resource collection requires a local Unix Docker socket")
	}
	socket := strings.TrimPrefix(host, "unix://")
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socket)
	}}}, nil
}

func dockerJSON(ctx context.Context, client *http.Client, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("docker resource request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("docker resources: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(result)
}

func discoverContainers(ctx context.Context, client *http.Client, project string) ([]resourceContainer, error) {
	if !ownedProject.MatchString(project) {
		return nil, fmt.Errorf("resource project must be an owned wisply-load stack")
	}
	filters, _ := json.Marshal(map[string][]string{"label": {"com.docker.compose.project=" + project}})
	var list []struct {
		ID, ImageID string
		Labels      map[string]string
	}
	if err := dockerJSON(ctx, client, "/containers/json?filters="+url.QueryEscape(string(filters)), &list); err != nil {
		return nil, err
	}
	if len(list) != 13 {
		return nil, fmt.Errorf("expected 13 owned containers, got %d", len(list))
	}
	result := make([]resourceContainer, 0, len(list))
	for _, c := range list {
		if c.Labels["com.docker.compose.project"] != project {
			return nil, fmt.Errorf("foreign Docker resource target")
		}
		var info struct {
			Image      string
			HostConfig struct {
				Memory, NanoCpus int64
				CpusetCpus       string
			}
			Config struct{ Env []string }
		}
		if err := dockerJSON(ctx, client, "/containers/"+c.ID+"/json", &info); err != nil {
			return nil, err
		}
		settings := map[string]string{}
		for _, item := range info.Config.Env {
			key, value, _ := strings.Cut(item, "=")
			if key == "GOMAXPROCS" || key == "QUIC_GO_DISABLE_GSO" || key == "FANOUT_WORKERS" || key == "FANOUT_QUEUE_SIZE" || key == "FANOUT_TIMEOUT" {
				settings[key] = value
			}
		}
		result = append(result, resourceContainer{ID: c.ID, Service: c.Labels["com.docker.compose.service"], Image: info.Image,
			MemoryLimit: info.HostConfig.Memory, NanoCPUs: info.HostConfig.NanoCpus, CPUSet: info.HostConfig.CpusetCpus, Runtime: settings})
	}
	slices.SortFunc(result, func(a, b resourceContainer) int { return strings.Compare(a.Service, b.Service) })
	return result, nil
}

func readCPU(reader io.Reader) (map[string][]uint64, error) {
	result := map[string][]uint64{}
	s := bufio.NewScanner(reader)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		if len(fields) < 9 {
			return nil, fmt.Errorf("short proc CPU record")
		}
		values := make([]uint64, len(fields)-1)
		for i, f := range fields[1:] {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return nil, err
			}
			values[i] = v
		}
		result[fields[0]] = values
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no proc CPU counters")
	}
	return result, s.Err()
}

func sampleBoundary(ctx context.Context, client *http.Client, containers []resourceContainer, target time.Time) (resourceBoundary, error) {
	b := resourceBoundary{Target: target.UTC(), Before: time.Now().UTC(), Containers: make([]resourceSample, len(containers))}
	f, err := os.Open("/proc/stat")
	if err != nil {
		return b, err
	}
	b.CPU, err = readCPU(f)
	_ = f.Close()
	if err != nil {
		return b, err
	}
	stat, readErr := os.ReadFile("/proc/self/stat")
	if readErr != nil {
		return b, readErr
	}
	fields := strings.Fields(string(stat)[strings.LastIndexByte(string(stat), ')')+2:])
	if len(fields) < 13 {
		return b, fmt.Errorf("short process CPU record")
	}
	for i := range 2 {
		b.LoadProcessTicks[i], err = strconv.ParseUint(fields[11+i], 10, 64)
		if err != nil {
			return b, err
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(containers))
	for i, c := range containers {
		wg.Go(func() {
			var data struct {
				Read     time.Time
				CPUStats struct {
					CPUUsage struct {
						TotalUsage uint64 `json:"total_usage"`
					} `json:"cpu_usage"`
				} `json:"cpu_stats"`
				MemoryStats struct{ Usage uint64 } `json:"memory_stats"`
				Networks    map[string]struct {
					RxBytes uint64 `json:"rx_bytes"`
					TxBytes uint64 `json:"tx_bytes"`
				}
			}
			s := resourceSample{Service: c.Service, Before: time.Now().UTC()}
			if err := dockerJSON(ctx, client, "/containers/"+c.ID+"/stats?stream=false&one-shot=true", &data); err != nil {
				errs <- err
				return
			}
			s.After, s.EngineRead = time.Now().UTC(), data.Read
			if data.Read.IsZero() {
				errs <- fmt.Errorf("docker stats lacks read timestamp for %s", c.Service)
				return
			}
			s.CPUTimeNS, s.MemoryBytes = data.CPUStats.CPUUsage.TotalUsage, data.MemoryStats.Usage
			for _, n := range data.Networks {
				s.RxBytes += n.RxBytes
				s.TxBytes += n.TxBytes
			}
			b.Containers[i] = s
		})
	}
	wg.Wait()
	close(errs)
	b.After = time.Now().UTC()
	for e := range errs {
		err = errors.Join(err, e)
	}
	return b, err
}

func metricSnapshots(ctx context.Context, c config, phase string) error {
	for i, addr := range c.metricURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(addr, "/")+"/metrics", nil)
		if err != nil {
			return err
		}
		before := time.Now()
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("metrics request failed")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode != 200 || len(data) > 4<<20 {
			return fmt.Errorf("invalid metrics response")
		}
		after := time.Now()
		data = append([]byte(fmt.Sprintf("# wsload_snapshot_before_unix_ns %d\n# wsload_snapshot_after_unix_ns %d\n", before.UnixNano(), after.UnixNano())), data...)
		if err = os.WriteFile(filepath.Join(c.out, fmt.Sprintf("metrics-%s-%d.prom", phase, i)), data, 0600); err != nil {
			return err
		}
	}
	return nil
}

func captureResources(ctx context.Context, c config, start time.Time) (err error) {
	if c.dockerProject == "" {
		return nil
	}
	client, err := dockerClient()
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	rep := resourceReport{HostNetwork: map[string]string{}, CPUCount: runtime.NumCPU(), Schema: 1, Go: runtime.Version(), OS: runtime.GOOS, Architecture: runtime.GOARCH}
	kernel, kernelErr := os.ReadFile("/proc/sys/kernel/osrelease")
	if kernelErr != nil {
		return kernelErr
	}
	rep.Kernel = strings.TrimSpace(string(kernel))
	for key, path := range map[string]string{"rmem_max": "/proc/sys/net/core/rmem_max", "wmem_max": "/proc/sys/net/core/wmem_max", "tcp_congestion_control": "/proc/sys/net/ipv4/tcp_congestion_control"} {
		value, e := os.ReadFile(path)
		if e != nil {
			rep.HostNetwork[key] = "unavailable"
		} else {
			rep.HostNetwork[key] = strings.TrimSpace(string(value))
		}
	}
	defer func() {
		rep.Complete = err == nil
		err = errors.Join(err, atomicJSON(filepath.Join(c.out, "resources.json"), rep))
	}()
	ticks, tickErr := exec.CommandContext(ctx, "getconf", "CLK_TCK").Output()
	if tickErr != nil {
		return tickErr
	}
	rep.ProcTicksPerSecond, err = strconv.ParseInt(strings.TrimSpace(string(ticks)), 10, 64)
	if err != nil || rep.ProcTicksPerSecond <= 0 {
		return fmt.Errorf("unknown proc CPU clock")
	}
	rep.Containers, err = discoverContainers(ctx, client, c.dockerProject)
	if err != nil {
		return err
	}
	for i, at := range []time.Time{start.Add(c.warmup), start.Add(c.warmup + time.Duration(len(c.rates))*c.step)} {
		timer := time.NewTimer(max(0, time.Until(at)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		capture, cancel := context.WithTimeout(ctx, 10*time.Second)
		b, e := sampleBoundary(capture, client, rep.Containers, at)
		if e == nil {
			e = metricSnapshots(capture, c, []string{"start", "end"}[i])
		}
		cancel()
		rep.Boundaries = append(rep.Boundaries, b)
		if e != nil {
			return e
		}
	}
	for i, last := range rep.Boundaries[1].Containers {
		first := rep.Boundaries[0].Containers[i]
		if last.CPUTimeNS < first.CPUTimeNS || last.EngineRead.Before(first.EngineRead) {
			return fmt.Errorf("container resource counters reset during measurement")
		}
	}
	return nil
}
