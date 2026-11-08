package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// command is injected by tests; constructing a controller executes nothing.
type command func(context.Context, []string, []string) ([]byte, error)
type compose struct {
	root, work, project, transport string
	exec                           command
}

func execute(ctx context.Context, args, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = env
	return cmd.CombinedOutput()
}

func (c *compose) call(ctx context.Context, args ...string) ([]byte, error) {
	argv := []string{"docker-compose", "--env-file", "/dev/null", "-p", c.project, "-f", filepath.Join(c.root, "test/e2e/compose.yaml")}
	argv = append(argv, args...)
	env := append(os.Environ(), "E2E_BIN_DIR="+filepath.Join(c.work, "bin"), "E2E_CERTS_DIR="+filepath.Join(c.work, "certs"), "E2E_TRANSPORT="+c.transport)
	out, err := c.exec(ctx, argv, env)
	if err != nil {
		return out, fmt.Errorf("compose %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (c *compose) endpoint(ctx context.Context, service, port string) (string, error) {
	out, err := c.call(ctx, "port", service, port)
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(string(out))
	host, p, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" {
		return "", fmt.Errorf("%s: expected a single loopback port, got %q", service, addr)
	}
	return "http://" + net.JoinHostPort(host, p), nil
}

var containerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

// impair changes only RPC responses in the selected container namespace.
// Profiles are correctness probes, not calibrated benchmark network models.
func (c *compose) impair(ctx context.Context, service, profile string) error {
	if service != "auth-service" && service != "chat-service" {
		return fmt.Errorf("cannot impair %q", service)
	}
	profiles := map[string]string{"delay-loss": "delay 50ms loss 1%", "blackhole": "loss 100%", "off": ""}
	args, ok := profiles[profile]
	if !ok {
		return fmt.Errorf("unknown impairment %q", profile)
	}
	out, err := c.call(ctx, "ps", "-q", service)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(string(out))
	if !containerID.MatchString(id) {
		return fmt.Errorf("invalid container ID %q", id)
	}
	script := "tc qdisc del dev eth0 root 2>/dev/null || true"
	if args != "" {
		script += "\ntc qdisc add dev eth0 root handle 1: prio bands 3\ntc qdisc add dev eth0 parent 1:3 handle 30: netem " + args + "\ntc filter add dev eth0 protocol ip parent 1: prio 1 u32 match ip sport 50051 0xffff flowid 1:3"
	}
	argv := []string{"docker", "run", "--rm", "--network", "container:" + id, "--cap-add", "NET_ADMIN", "wisply-e2e-netem", "sh", "-ec", script}
	out, err = c.exec(ctx, argv, os.Environ())
	if err != nil {
		return fmt.Errorf("netem %s/%s: %w: %s", service, profile, err, out)
	}
	return nil
}

func (c *compose) restart(ctx context.Context, service string, crash bool) error {
	if service != "auth-service" && service != "chat-service" {
		return fmt.Errorf("cannot restart %q", service)
	}
	var err error
	if crash {
		_, err = c.call(ctx, "kill", "-s", "KILL", service)
	} else {
		_, err = c.call(ctx, "stop", "-t", "15", service)
	}
	if err != nil {
		return err
	}
	_, err = c.call(ctx, "start", service)
	return err
}

func (c *compose) logs(ctx context.Context, w io.Writer) error {
	out, err := c.call(ctx, "logs", "--no-color", "--timestamps")
	if _, writeErr := w.Write(out); writeErr != nil {
		return writeErr
	}
	return err
}

func cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Minute)
}
