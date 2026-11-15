package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type loadUser struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	ChatID    string `json:"chat_id"`
	PeerID    string `json:"peer_id"`
	PublicKey string `json:"public_key"`
	Gateway   int    `json:"gateway"`
}

type loadFixture struct {
	Schema int        `json:"schema"`
	Users  []loadUser `json:"users"`
}

func (s *suite) prepareLoadUsers(ctx context.Context, n int) (loadFixture, error) {
	f := loadFixture{Schema: 1}
	for i := range n {
		for j := range 2 {
			u, err := s.register(ctx, 2*i+j)
			if err != nil {
				return f, err
			}
			s.users[j] = u
		}
		chat, err := s.createChat(ctx, false)
		if err != nil {
			return f, err
		}
		for j := range 2 {
			f.Users = append(f.Users, loadUser{ID: s.users[j].ID, Token: s.users[j].token, ChatID: chat, PeerID: s.users[1-j].ID, PublicKey: "e2e-public-key", Gateway: j})
		}
	}
	return f, nil
}

// runLoad uses the acceptance stack but writes independent load artifacts.
func (s *suite) runLoad(ctx context.Context, c config) (err error) {
	started := time.Now()
	result := caseResult{Name: "application_load_" + c.loadProfile, Outcome: "failed", Details: map[string]any{}}
	defer func() {
		result.DurationMS = time.Since(started).Milliseconds()
		if err == nil {
			result.Outcome = "passed"
		} else {
			result.Error = err.Error()
		}
		s.rep.Cases = append(s.rep.Cases, result)
	}()
	fixture, err := s.prepareLoadUsers(ctx, c.loadPairs)
	if err != nil {
		return err
	}
	private := filepath.Join(s.dir, "fixture.private.json")
	data, err := json.Marshal(fixture)
	if err != nil {
		return err
	}
	if err = os.WriteFile(private, data, 0600); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.Remove(private)) }()
	metrics := []string{s.gateways[0], s.gateways[1], s.chat, s.media, s.worker}
	mix := map[string]string{"messages": "send_message=1", "auth": "create_chat=1", "media": "media=1"}[c.loadProfile]
	minio, err := minioAddress(s.minio)
	if err != nil {
		return err
	}
	args := []string{"-fixture", private, "-out", filepath.Join(s.dir, "load"), "-gateways", "gateway-1=" + "ws" + s.gateways[0][4:] + "/ws,gateway-2=ws" + s.gateways[1][4:] + "/ws",
		"-chat", s.chat, "-media", s.media, "-minio", minio, "-users", strconv.Itoa(c.loadPairs), "-steps", strconv.FormatFloat(c.loadRate, 'g', -1, 64),
		"-warmup", c.loadWarmup.String(), "-step", c.loadDuration.String(), "-drain", c.loadDrain.String(), "-mix", mix, "-seed", strconv.FormatUint(c.loadSeed, 10),
		"-media-bytes", strconv.Itoa(c.loadMediaBytes), "-docker-project", s.control.project, "-metrics", strings.Join(metrics, ",")}
	file, err := os.OpenFile(filepath.Join(s.dir, "load.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(c.work, "bin/wsload"), args...)
	cmd.Stdout, cmd.Stderr = file, file
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 15 * time.Second
	err = cmd.Run()
	err = errors.Join(err, file.Close())
	if err != nil {
		return fmt.Errorf("wsload failed (see private load.log): %w", err)
	}
	if err = validateLoadArtifacts(filepath.Join(s.dir, "load"), result.Details); err != nil {
		return err
	}
	return s.checkLoadEdges(ctx, c.loadProfile, metrics, result.Details)
}

func validateLoadArtifacts(dir string, details map[string]any) error {
	var meta struct {
		Complete bool `json:"complete"`
		Planned  int  `json:"planned"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &meta); err != nil {
		return err
	}
	if !meta.Complete || meta.Planned == 0 {
		return fmt.Errorf("incomplete load metadata")
	}
	file, err := os.Open(filepath.Join(dir, "calls.csv"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	r := csv.NewReader(file)
	header, err := r.Read()
	if err != nil {
		return err
	}
	index := map[string]int{}
	for i, k := range header {
		index[k] = i
	}
	for _, key := range []string{"operation_id", "step", "outcome", "complete_ns"} {
		if _, ok := index[key]; !ok {
			return fmt.Errorf("missing raw column %s", key)
		}
	}
	seen := map[string]bool{}
	outcomes := map[string]int{}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		id := row[index["operation_id"]]
		if seen[id] {
			return fmt.Errorf("duplicate operation ID")
		}
		seen[id] = true
		if row[index["outcome"]] == "planned" || row[index["outcome"]] == "pending" {
			return fmt.Errorf("nonterminal raw outcome")
		}
		if row[index["step"]] != "-1" {
			outcomes[row[index["outcome"]]]++
		}
	}
	if len(seen) != meta.Planned {
		return fmt.Errorf("planned %d != raw %d", meta.Planned, len(seen))
	}
	details["planned"], details["measurement_outcomes"] = meta.Planned, outcomes
	return nil
}

func (s *suite) checkLoadEdges(_ context.Context, profile string, endpoints []string, details map[string]any) error {
	edges := map[string][]struct {
		index           int
		service, method string
	}{
		"messages": {{0, "ws-gateway", "/chat.v1.ChatService/SendMessage"}, {1, "ws-gateway", "/chat.v1.ChatService/SendMessage"},
			{4, "notification-worker", "/chat.v1.ChatService/ListMembers"}, {4, "notification-worker", "/auth.v1.AuthService/ListPushSubscriptions"}},
		"auth":  {{2, "chat-service", "/auth.v1.AuthService/GetUserByID"}},
		"media": {{3, "media-service", "/chat.v1.ChatService/ListMembers"}},
	}
	for _, edge := range edges[profile] {
		var counts [2]float64
		for i, phase := range []string{"start", "end"} {
			b, err := os.ReadFile(filepath.Join(s.dir, "load", fmt.Sprintf("metrics-%s-%d.prom", phase, edge.index)))
			if err != nil {
				return err
			}
			counts[i] = rpcCount(string(b), edge.service, s.rep.Transport, edge.method)
		}
		delta := counts[1] - counts[0]
		if delta <= 0 {
			return fmt.Errorf("no measurement RPC on %s %s over %s", edge.service, edge.method, s.rep.Transport)
		}
		details[endpoints[edge.index]+edge.method] = delta
	}
	return nil
}
