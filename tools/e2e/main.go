// Command e2e checks the real application over both interservice transports.
// Running this command starts Docker; tests and scripts/e2e.sh prepare do not.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type config struct {
	sealOnly              bool
	root, work, transport string
	observe, timeout      time.Duration
}

func parseConfig(args []string) (config, error) {
	var c config
	fs := flag.NewFlagSet("e2e", flag.ContinueOnError)
	fs.BoolVar(&c.sealOnly, "seal", false, "record prepared binary and source hashes without Docker")
	fs.StringVar(&c.root, "root", ".", "repository root")
	fs.StringVar(&c.work, "work", "out/e2e", "prepared binaries, certificates and reports")
	fs.StringVar(&c.transport, "transport", "all", "grpc, quic or all; sequential independent stacks")
	fs.DurationVar(&c.observe, "observe", time.Second, "bounded observation window for extra notifications")
	fs.DurationVar(&c.timeout, "timeout", 20*time.Minute, "overall deadline including Docker startup")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 || (c.transport != "all" && c.transport != "grpc" && c.transport != "quic") || c.observe <= 0 || c.observe > 10*time.Second || c.timeout <= 0 {
		return c, fmt.Errorf("want transport grpc/quic/all, observe in (0,10s] and a positive timeout; no positional arguments")
	}
	var err error
	c.root, err = filepath.Abs(c.root)
	if err != nil {
		return c, err
	}
	c.work, err = filepath.Abs(c.work)
	return c, err
}
func runVariant(ctx context.Context, c config, transport string) (err error) {
	run := newID()
	dir := filepath.Join(c.work, "runs", run+"-"+transport)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	manifest, err := c.loadPrepared()
	if err != nil {
		return err
	}
	revision := os.Getenv("E2E_REVISION")
	if revision == "" {
		revision = manifest.Revision
	}
	rep := &report{Prepared: manifest, Schema: 1, Run: run, Revision: revision, Transport: transport, Started: time.Now().UTC()}
	control := &compose{root: c.root, work: c.work, project: "wisply-e2e-" + run + "-" + transport, transport: transport, exec: execute}
	defer func() {
		logsCtx, cancelLogs := context.WithTimeout(context.Background(), 20*time.Second)
		file, logErr := os.OpenFile(filepath.Join(dir, "services.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if logErr == nil {
			logErr = control.logs(logsCtx, file)
			logErr = errors.Join(logErr, file.Close())
		}
		cancelLogs()
		clean, cancel := cleanupContext()
		defer cancel()
		_, downErr := control.call(clean, "down", "--volumes", "--remove-orphans", "--timeout", "15")
		err = errors.Join(err, logErr, downErr)
		finished := time.Now().UTC()
		rep.Finished = &finished
		rep.Passed = err == nil
		if err != nil {
			rep.Cases = append(rep.Cases, caseResult{Name: "stack_lifecycle", Outcome: "failed", Error: err.Error()})
		}
		err = errors.Join(err, rep.save(dir))
		fmt.Printf("Report: %s\n", filepath.Join(dir, "report.json"))
	}()
	if err = rep.save(dir); err != nil {
		return err
	}
	// The tools image is built only by an explicitly requested Docker run.
	if _, err = control.call(ctx, "--profile", "tools", "build", "netem"); err != nil {
		return err
	}
	if _, err = control.call(ctx, "up", "-d", "auth-service", "chat-service", "media-service"); err != nil {
		return err
	}
	var e endpoints
	for _, item := range []struct {
		service, port string
		dst           *string
	}{
		{"auth-service", "8080", &e.auth}, {"chat-service", "8080", &e.chat}, {"media-service", "8080", &e.media},
		{"mailhog", "8025", &e.mail}, {"nats", "8222", &e.nats}, {"minio", "9000", &e.minio},
	} {
		*item.dst, err = control.endpoint(ctx, item.service, item.port)
		if err != nil {
			return err
		}
	}
	minio, err := minioAddress(e.minio)
	if err != nil {
		return err
	}
	s := &suite{api: newAPI(minio), endpoints: e, control: control, rep: rep, dir: dir, observe: c.observe}
	defer s.api.client.CloseIdleConnections()
	startup, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err = s.ready(startup); err != nil {
		return err
	}
	if _, err = control.call(ctx, "up", "-d", "notification-worker"); err != nil {
		return err
	}
	s.worker, err = control.endpoint(ctx, "notification-worker", "9090")
	if err != nil {
		return err
	}
	if err = poll(startup, func() error { _, err := s.metrics(startup, s.worker); return err }); err != nil {
		return err
	}
	if _, err = control.call(ctx, "up", "-d", "ws-gateway-1", "ws-gateway-2"); err != nil {
		return err
	}
	for i, name := range []string{"ws-gateway-1", "ws-gateway-2"} {
		s.gateways[i], err = control.endpoint(ctx, name, "8080")
		if err != nil {
			return err
		}
	}
	if err = s.consumersReady(startup); err != nil {
		return err
	}
	return s.run(ctx)
}
func main() {
	c, err := parseConfig(os.Args[1:])
	if err == nil {
		if c.sealOnly {
			err = c.seal()
		} else {
			_, err = c.loadPrepared()
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if c.sealOnly {
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	transports := []string{c.transport}
	if c.transport == "all" {
		transports = []string{"grpc", "quic"}
	}
	var failures error
	for _, transport := range transports {
		if ctx.Err() != nil {
			failures = errors.Join(failures, ctx.Err())
			break
		}
		failures = errors.Join(failures, runVariant(ctx, c, transport))
	}
	if failures != nil {
		fmt.Fprintln(os.Stderr, failures)
		os.Exit(1)
	}
}
