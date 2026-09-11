package agent

import (
	"context"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every check uses real Git worktrees and processes; no process or filesystem mocks.
func TestCodeGateRealGit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		out, err := gateCommand(ctx, repo, append([]string{"git"}, args...)...)
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(out)
	}
	git("init")
	if err := os.WriteFile(filepath.Join(repo, "value.txt"), []byte("verified\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("-c", "user.name=Gate test", "-c", "user.email=gate@solo.local", "commit", "-m", "base")
	commit := git("rev-parse", "HEAD")
	spec := CodeReviewDispatch{SubmissionID: uuid.NewString(), ArtifactVersion: commit, Gate: CodeGateSpec{RepositoryPath: repo, BaseCommit: commit, TimeoutSeconds: 3, Checks: []CodeGateCheck{{RequirementID: "R1", Command: []string{"git", "diff", "--exit-code", commit}}}}}
	for _, test := range []struct {
		name, decision string
		command        []string
	}{
		{"pass", "accepted", []string{"git", "diff", "--exit-code", commit}},
		{"assertion", "rejected", []string{"git", "rev-parse", "--verify", "not-an-existing-ref"}},
		{"environment", "needs_human", []string{"solo-test-nonexistent-executable"}},
		{"mutated-source", "needs_human", []string{"git", "rm", "value.txt"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := spec
			d.Gate.Checks = []CodeGateCheck{{RequirementID: "R1", Command: test.command}}
			result := RunCodeGate(ctx, d, filepath.Join(root, test.name))
			if result.Decision != test.decision {
				t.Fatalf("%+v", result)
			}
		})
	}
	// Cancellation kills a real long-running Git process (hash-object waits for its FIFO input).
	timeoutCtx, cancel := context.WithCancel(ctx)
	cancel()
	if result := RunCodeGate(timeoutCtx, spec, filepath.Join(root, "cancelled")); result.Decision != "needs_human" {
		t.Fatal(result)
	}
	path := filepath.Join(root, "task")
	if _, err := PrepareTaskWorktree(ctx, repo, path, commit); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "value.txt"), []byte("unfinished"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareTaskWorktree(ctx, repo, path, commit); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(path, "value.txt"))
	if string(data) != "unfinished" {
		t.Fatal("lost in-progress work")
	}
	if result := RunCodeGate(ctx, spec, path); result.Decision != "needs_human" {
		t.Fatal("accepted dirty worktree", result)
	}
	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := gateCommand(ctx, other, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareTaskWorktree(ctx, repo, other, commit); err == nil {
		t.Fatal("adopted unrelated repository")
	}
	if git("rev-parse", "refs/solo/submissions/"+spec.SubmissionID) != commit {
		t.Fatal("submission object not retained")
	}
	if git("rev-parse", "HEAD") != commit || git("status", "--porcelain") != "" {
		t.Fatal("verification changed source checkout")
	}
	timeout, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stop()
	started := time.Now()
	_, err := gateCommand(timeout, repo, "git", "-c", "alias.pause=!sleep 10", "pause")
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("process timeout failed: %v", err)
	}
}
