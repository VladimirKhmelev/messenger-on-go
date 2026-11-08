package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigAndPreparationDoNotInvokeDocker(t *testing.T) {
	for _, args := range [][]string{{"-transport", "typo"}, {"-observe", "0s"}, {"-observe", "11s"}, {"-timeout", "0s"}, {"unexpected"}} {
		if _, err := parseConfig(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	c, err := parseConfig([]string{"-root", t.TempDir(), "-work", t.TempDir(), "-seal"})
	if err != nil || !c.sealOnly {
		t.Fatal(err)
	}
	// All required source paths and prepared files are fixtures, not real services.
	for _, dir := range []string{"services", "pkg", "proto/gen", "tools/e2e"} {
		if err := os.MkdirAll(filepath.Join(c.root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"scripts/e2e.sh", "scripts/quic-certs.sh", "test/e2e/compose.yaml", "scripts/netem.Dockerfile", "services/sample/main.go"} {
		writeFixture(t, filepath.Join(c.root, rel), "fixture")
	}
	source, err := sourceDigest(c.root)
	if err != nil {
		t.Fatal(err)
	}
	manifest := prepared{SourceSHA256: source, Files: map[string]string{}}
	for _, rel := range preparedFiles() {
		p := filepath.Join(c.work, rel)
		writeFixture(t, p, "prepared")
		manifest.Files[rel], err = digestFile(p)
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(c.work, "prepared.json"), string(data))
	if _, err := c.loadPrepared(); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(c.work, "bin/chat-service"), "tampered")
	if _, err := c.loadPrepared(); err == nil {
		t.Fatal("changed executable accepted")
	}
	writeFixture(t, filepath.Join(c.work, "bin/chat-service"), "prepared")
	writeFixture(t, filepath.Join(c.root, "services/sample/main.go"), "new source")
	if _, err := c.loadPrepared(); err == nil {
		t.Fatal("stale build accepted")
	}
}
func writeFixture(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestReportPreservesUnrunCasesAndPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	r := &report{Cases: []caseResult{{Name: "not_started", Outcome: "not_run"}}}
	if err := r.save(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Passed || got.Finished != nil || got.Cases[0].Outcome != "not_run" {
		t.Fatalf("incomplete run reported as finished: %+v", got)
	}
	info, err := os.Stat(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}
