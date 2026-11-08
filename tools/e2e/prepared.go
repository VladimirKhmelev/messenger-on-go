package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type prepared struct {
	Revision     string            `json:"revision"`
	SourceSHA256 string            `json:"source_sha256"`
	PreparedAt   time.Time         `json:"prepared_at"`
	Files        map[string]string `json:"files_sha256"`
}

func digestFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func sourceDigest(root string) (string, error) {
	var paths []string
	for _, dir := range []string{"services", "pkg", "proto/gen", "tools/e2e"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") && name != "go.mod" && name != "go.sum" {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, rel)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	paths = append(paths, "scripts/e2e.sh", "scripts/quic-certs.sh", "test/e2e/compose.yaml", "scripts/netem.Dockerfile")
	sort.Strings(paths)
	h := sha256.New()
	for _, rel := range paths {
		sum, err := digestFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%s\n", rel, sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func preparedFiles() []string {
	return []string{"bin/auth-service", "bin/chat-service", "bin/ws-gateway", "bin/media-service", "bin/notification-worker", "bin/e2e", "certs/auth-service/ca.pem", "certs/auth-service/cert.pem", "certs/auth-service/key.pem", "certs/chat-service/ca.pem", "certs/chat-service/cert.pem", "certs/chat-service/key.pem", "certs/ws-gateway/ca.pem", "certs/media-service/ca.pem", "certs/notification-worker/ca.pem"}
}
func (c config) seal() error {
	top, err := exec.Command("git", "-C", c.root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return err
	}
	if filepath.Clean(strings.TrimSpace(string(top))) != filepath.Clean(c.root) {
		return fmt.Errorf("root must be the messenger Git repository root")
	}
	source, err := sourceDigest(c.root)
	if err != nil {
		return err
	}
	revision, err := exec.Command("git", "-C", c.root, "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	manifest := prepared{Revision: strings.TrimSpace(string(revision)), SourceSHA256: source, PreparedAt: time.Now().UTC(), Files: map[string]string{}}
	for _, rel := range preparedFiles() {
		sum, err := digestFile(filepath.Join(c.work, rel))
		if err != nil {
			return err
		}
		manifest.Files[rel] = sum
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.work, "prepared.json"), append(data, '\n'), 0600)
}
func (c config) loadPrepared() (prepared, error) {
	var manifest prepared
	data, err := os.ReadFile(filepath.Join(c.work, "prepared.json"))
	if err != nil {
		return manifest, fmt.Errorf("run scripts/e2e.sh prepare first: %w", err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	source, err := sourceDigest(c.root)
	if err != nil {
		return manifest, err
	}
	if source != manifest.SourceSHA256 {
		return manifest, fmt.Errorf("source files changed after preparation; run scripts/e2e.sh prepare again")
	}
	for _, rel := range preparedFiles() {
		sum, err := digestFile(filepath.Join(c.work, rel))
		if err != nil {
			return manifest, err
		}
		if manifest.Files[rel] != sum {
			return manifest, fmt.Errorf("prepared file changed: %s; prepare again", rel)
		}
	}
	return manifest, nil
}
