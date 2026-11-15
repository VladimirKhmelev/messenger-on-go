package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func readRows(t *testing.T, path string) []map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var result []map[string]string
	for _, row := range rows[1:] {
		m := map[string]string{}
		for i, name := range rows[0] {
			m[name] = row[i]
		}
		result = append(result, m)
	}
	return result
}

func TestRawDeadlineAndLateResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		r := newRaw(start)
		slots := make(chan struct{}, 1)
		x := r.plan(0, 1, arrival{kind: getPresence}, user{id: "a"}, 100*time.Millisecond)
		if !r.reserve(x, slots) || !r.send(x, time.Now()) {
			t.Fatal("not reserved")
		}
		y := r.plan(0, 2, arrival{kind: getPresence}, user{id: "a"}, time.Second)
		if r.reserve(y, slots) {
			t.Fatal("capacity ignored")
		}
		time.Sleep(200 * time.Millisecond)
		r.finish(x, time.Now(), "success", "", 0)
		if x.Outcome != "deadline" || x.CompleteNS != int64(100*time.Millisecond) || x.ResponseNS != int64(200*time.Millisecond) || len(slots) != 0 {
			t.Fatalf("deadline: %+v slots=%d", x, len(slots))
		}
		if y.Outcome != "not_sent" || y.Error != "inflight_limit" {
			t.Fatal(y)
		}
		r.close(time.Now(), false, nil)
	})
}

func TestRawReceiptsAcrossUsersBeforeAck(t *testing.T) {
	start := time.Now()
	r := newRaw(start)
	x := r.plan(0, 1, arrival{kind: sendMessage}, user{id: "sender", peerID: "receiver", chatID: "chat"}, time.Second)
	y := r.plan(0, 2, arrival{kind: sendMessage}, user{id: "sender", peerID: "receiver", chatID: "chat"}, time.Second)
	r.expect(x, "first", "receiver", 1)
	r.expect(y, "second", "receiver", 2)
	r.receive("sender", notification(t, "chat", "first", "a"), start.Add(time.Millisecond))
	r.noteNotify("receiver", "a", start.Add(2*time.Millisecond))
	r.receive("receiver", notification(t, "chat", "second", "b"), start.Add(3*time.Millisecond))
	r.receive("receiver", notification(t, "chat", "first", "a"), start.Add(4*time.Millisecond))
	r.receive("receiver", notification(t, "chat", "first", "a"), start.Add(5*time.Millisecond))
	r.ack(x, "first", "a", true)
	r.ack(y, "second", "b", true)
	r.close(start.Add(time.Second), false, nil)
	d := r.receipts[receiptKey("chat", "first")]
	if d.ReceivedNS != int64(4*time.Millisecond) || d.NotifyNS != int64(2*time.Millisecond) || d.Duplicates != 1 || !d.Reordered || d.Mismatch {
		t.Fatalf("receipt: %+v", d)
	}
}

func TestRawCancellationPreservesPlannedAndPrivateOutput(t *testing.T) {
	s := newStack(t)
	s.hang.Store(true)
	c := s.start(100)
	c.warmup = 0
	c.out = filepath.Join(t.TempDir(), "run")
	c.drain = time.Second
	ctx, cancel := context.WithCancel(t.Context())
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	defer cancel()
	_, err := run(ctx, c)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.out, "meta.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed run has completion marker")
	}
	var failed map[string]any
	b, err := os.ReadFile(filepath.Join(c.out, "failed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &failed); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, filepath.Join(c.out, "calls.csv"))
	if len(rows) == 0 || float64(len(rows)) != failed["planned"] {
		t.Fatal("planned requests disappeared")
	}
	for _, row := range rows {
		if row["outcome"] == "planned" || row["outcome"] == "pending" {
			t.Fatal("unfinished outcome")
		}
	}
	for _, name := range []string{"calls.csv", "deliveries.csv", "failed.json"} {
		info, err := os.Stat(filepath.Join(c.out, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("permissions %s: %v", name, err)
		}
	}
}

func TestRawKeepsErrorLatencyAndRejectsReusedDirectory(t *testing.T) {
	s := newStack(t)
	s.failing["get_presence"] = true
	c := s.start(80)
	c.mix = mix{getPresence: 1}
	c.out = filepath.Join(t.TempDir(), "run")
	c.drain = 20 * time.Millisecond
	if _, err := run(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, filepath.Join(c.out, "calls.csv"))
	for _, row := range rows {
		if row["outcome"] != "rpc_error" || row["complete_ns"] == "0" || row["response_ns"] == "0" {
			t.Fatal(row)
		}
	}
	if _, err := run(t.Context(), c); err == nil {
		t.Fatal("overwrote existing run")
	}
}

func TestNonFiniteLoadSettings(t *testing.T) {
	for _, args := range [][]string{{"-steps", "NaN"}, {"-steps", "+Inf"}, {"-mix", "send_message=NaN"}, {"-mix", "send_message=+Inf"}, {"-max-inflight", "0"}, {"-fixture", "private.json"}} {
		if _, err := parseFlags(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	if _, err := parseMix("get_presence=1e308,get_read_status=1e308"); err == nil {
		t.Fatal("overflowing mix")
	}
	// Fixture errors deliberately exclude their input, which contains credentials.
	f := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(f, []byte(`{"token":"secret-token","extra":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := readFixture(f, 2)
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatal(err)
	}
}

func TestFailedMediaStillRecordsRequestedSize(t *testing.T) {
	start := time.Now()
	r := newRaw(start)
	x := r.plan(0, 1, arrival{kind: mediaKind}, user{id: "sender"}, time.Second)
	r.send(x, start.Add(time.Millisecond))
	r.finish(x, start.Add(2*time.Millisecond), "rpc_error", "HTTP 503", 0)
	r.close(start.Add(time.Second), false, nil)
	c := config{rates: []float64{1}, step: time.Second, mediaBytes: 65536}
	dir := t.TempDir()
	if err := r.save(dir, c, start.Add(time.Second), nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Size   int      `json:"media_bytes"`
		Useful int      `json:"measurement_useful_bytes"`
		Kinds  []string `json:"kind_names"`
	}
	if err = json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Size != 65536 || meta.Useful != 0 || len(meta.Kinds) != 5 {
		t.Fatalf("lost input parameters after failure: %+v", meta)
	}
}
