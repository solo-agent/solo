package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskArtifactRoundTripPreservesExactImmutableFile(t *testing.T) {
	t.Chdir(t.TempDir())
	source := "# 中文\r\ndef f():\n    return 'line\\n'\n\n"
	if err := os.WriteFile("solution.py", []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"expected_task_version":1,"idempotency_key":"v1","handoff":{"summary":"ready"},"evidence":[{"id":"source","description":"Complete source","uri":"file:///stale","sha256":"stale"},{"id":"verification","description":"Actual checks","content":"executed"}]}`)
	prepared, err := withTaskArtifact(body, "solution.py", "source")
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		ArtifactVersion string             `json:"artifact_version"`
		Evidence        []taskFileEvidence `json:"evidence"`
	}
	if err := json.Unmarshal(prepared, &request); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(source))
	want := hex.EncodeToString(digest[:])
	if request.ArtifactVersion != want || request.Evidence[0].Content != source || request.Evidence[0].SHA256 != want || request.Evidence[0].URI != "" || request.Evidence[1].Content != "executed" {
		t.Fatalf("file or verification changed: %+v", request)
	}
	submissions, _ := json.Marshal([]any{map[string]any{"id": "original", "evidence": request.Evidence}})
	if err := os.WriteFile("solution.py", []byte("changed later"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := exportTaskEvidence(submissions, "original", "source", "review.py")
	if err != nil || got != want {
		t.Fatalf("export: %s %v", got, err)
	}
	read, err := os.ReadFile("review.py")
	if err != nil || string(read) != source {
		t.Fatalf("export used mutable file or changed whitespace: %q %v", read, err)
	}
	if _, err := exportTaskEvidence(submissions, "different", "source", "wrong.py"); err == nil {
		t.Fatal("exported a different submission")
	}
	if _, err := exportTaskEvidence(submissions, "original", "source", "review.py"); err == nil {
		t.Fatal("overwrote an existing review file")
	}
	request.Evidence[0].SHA256 = strings.Repeat("0", 64)
	submissions, _ = json.Marshal([]any{map[string]any{"id": "original", "evidence": request.Evidence}})
	if _, err := exportTaskEvidence(submissions, "original", "source", "tampered.py"); err == nil {
		t.Fatal("exported tampered evidence")
	}
}

func TestTaskEvidenceRejectsUnsafeFilesAndMissingSource(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	outside := filepath.Join(t.TempDir(), "private.py")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, "link.py"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{outside, "link.py", "."} {
		if _, err := withTaskArtifact([]byte(`{}`), path, "source"); err == nil {
			t.Fatalf("read unsafe file %s", path)
		}
	}
	for _, content := range [][]byte{nil, {0xff}, []byte(strings.Repeat("x", 64001))} {
		if err := os.WriteFile("invalid.py", content, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := withTaskArtifact([]byte(`{}`), "invalid.py", "source"); err == nil {
			t.Fatal("accepted invalid source")
		}
	}
	body := []byte(`[{"id":"s","evidence":[{"id":"E1","description":"link","uri":"file:///somewhere","sha256":"abc"}]}]`)
	if _, err := exportTaskEvidence(body, "s", "E1", "missing.py"); err == nil {
		t.Fatal("URI-only evidence counted as source")
	}
	if _, err := os.Stat("missing.py"); !os.IsNotExist(err) {
		t.Fatal("wrote missing source")
	}
}
