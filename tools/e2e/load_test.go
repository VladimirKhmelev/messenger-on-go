package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigIsExplicitAndBounded(t *testing.T) {
	c, err := parseConfig([]string{"-load"})
	if err != nil || !c.load || c.loadPairs != 8 {
		t.Fatal(c, err)
	}
	for _, args := range [][]string{{"-load", "-load-profile", "unknown"}, {"-load", "-load-rate", "NaN"}, {"-load", "-load-rate", "+Inf"}, {"-load", "-load-pairs", "0"}, {"-load", "-load-rate", "10000"}, {"-load", "-load-duration", "1h"}} {
		if _, err := parseConfig(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestLoadValidatesEveryPlannedOperation(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "meta.json"), `{"complete":true,"planned":2}`)
	writeFixture(t, filepath.Join(dir, "calls.csv"), "operation_id,step,outcome,complete_ns\na,0,success,10\nb,0,deadline,100\n")
	details := map[string]any{}
	if err := validateLoadArtifacts(dir, details); err != nil {
		t.Fatal(err)
	}
	if details["planned"] != 2 || details["measurement_outcomes"].(map[string]int)["deadline"] != 1 {
		t.Fatal(details)
	}
	for _, text := range []string{"operation_id,step,outcome,complete_ns\na,0,success,10\n", "operation_id,step,outcome,complete_ns\na,0,success,10\na,0,success,10\n", "operation_id,step,outcome,complete_ns\na,0,pending,0\nb,0,success,10\n"} {
		writeFixture(t, filepath.Join(dir, "calls.csv"), text)
		if err := validateLoadArtifacts(dir, map[string]any{}); err == nil {
			t.Fatal("invalid raw artifact accepted")
		}
	}
}

func TestLoadCompletionMarkerRequiresSuccessfulCleanup(t *testing.T) {
	dir := t.TempDir()
	r := &report{}
	if err := writeLoadMeta(dir, r); err == nil {
		t.Fatal("incomplete stack accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); !os.IsNotExist(err) {
		t.Fatal("premature marker")
	}
	now := time.Now()
	r.Passed, r.Finished = true, &now
	if err := writeLoadMeta(dir, r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err = json.Unmarshal(b, &got); err != nil || !got.Passed {
		t.Fatal(err)
	}
}
