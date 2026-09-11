package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const CodeGateCapability = "code_gate_v1"

var FullGitCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

type CodeGateCheck struct {
	RequirementID string   `json:"requirement_id"`
	Command       []string `json:"command"`
}
type CodeGateSpec struct {
	RepositoryPath string          `json:"repository_path"`
	BaseCommit     string          `json:"base_commit"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	Checks         []CodeGateCheck `json:"checks"`
}
type CodeReviewDispatch struct {
	SubmissionID    string       `json:"submission_id"`
	ArtifactVersion string       `json:"artifact_version"`
	Gate            CodeGateSpec `json:"gate"`
}
type CodeCheckResult struct {
	RequirementID string `json:"requirement_id"`
	Passed        bool   `json:"passed"`
	Output        string `json:"output"`
}
type CodeGateResult struct {
	Decision string            `json:"decision"`
	Reason   string            `json:"reason"`
	Worktree string            `json:"worktree"`
	Commit   string            `json:"commit"`
	Checks   []CodeCheckResult `json:"checks"`
}

type gateOutput struct{ bytes.Buffer }

func (w *gateOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 16000 - w.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = w.Buffer.Write(p)
	}
	return n, nil
}

// Gate programs run as processes, without shell parsing; cancellation also stops their child processes.
func gateCommand(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	configureGateProcess(cmd)
	output := &gateOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	return output.String(), err
}

func PrepareTaskWorktree(ctx context.Context, repository, path, commit string) (string, error) {
	if !filepath.IsAbs(repository) || !filepath.IsAbs(path) || !FullGitCommit.MatchString(commit) {
		return "", errors.New("absolute repository/worktree paths and a full lowercase Git commit are required")
	}
	root, err := gateCommand(ctx, repository, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repository unavailable: %s: %w", root, err)
	}
	repository = filepath.Clean(strings.TrimSpace(root))
	if _, err = os.Stat(path); err == nil {
		actual, err := gateCommand(ctx, path, "git", "rev-parse", "--show-toplevel")
		canonicalPath, pathErr := filepath.EvalSymlinks(path)
		canonicalRoot, rootErr := filepath.EvalSymlinks(strings.TrimSpace(actual))
		if err != nil || pathErr != nil || rootErr != nil || canonicalRoot != canonicalPath {
			return "", errors.New("existing path is not this Task's worktree")
		}
		repoCommon, repoErr := gateCommand(ctx, repository, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
		workCommon, workErr := gateCommand(ctx, path, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
		repoCommonPath, e1 := filepath.EvalSymlinks(strings.TrimSpace(repoCommon))
		workCommonPath, e2 := filepath.EvalSymlinks(strings.TrimSpace(workCommon))
		if repoErr != nil || workErr != nil || e1 != nil || e2 != nil || repoCommonPath != workCommonPath {
			return "", errors.New("existing worktree belongs to another repository")
		}
		if _, err := gateCommand(ctx, path, "git", "merge-base", "--is-ancestor", commit, "HEAD"); err != nil {
			return "", errors.New("existing worktree does not descend from the authorized base")
		}
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	output, err := gateCommand(ctx, repository, "git", "-c", "core.hooksPath=/dev/null", "worktree", "add", "--detach", path, commit)
	if err != nil {
		return "", fmt.Errorf("create worktree: %s: %w", output, err)
	}
	return path, nil
}

func verifyGateCommit(ctx context.Context, path, commit string) error {
	head, err := gateCommand(ctx, path, "git", "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(head) != commit {
		return errors.New("checked-out commit differs from submitted commit")
	}
	dirty, err := gateCommand(ctx, path, "git", "status", "--porcelain", "--untracked-files=no")
	if err != nil || strings.TrimSpace(dirty) != "" {
		return errors.New("tracked source changed during verification")
	}
	return nil
}

func RunCodeGate(ctx context.Context, dispatch CodeReviewDispatch, path string) CodeGateResult {
	result := CodeGateResult{Decision: "needs_human", Worktree: path, Commit: dispatch.ArtifactVersion, Checks: []CodeCheckResult{}}
	if dispatch.Gate.TimeoutSeconds < 1 || dispatch.Gate.TimeoutSeconds > 300 || !FullGitCommit.MatchString(dispatch.Gate.BaseCommit) || !FullGitCommit.MatchString(dispatch.ArtifactVersion) || len(dispatch.Gate.Checks) == 0 {
		result.Reason = "invalid code Gate configuration"
		return result
	}
	if _, err := uuid.Parse(dispatch.SubmissionID); err != nil {
		result.Reason = "invalid submission identity"
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(dispatch.Gate.TimeoutSeconds)*time.Second)
	defer cancel()
	if _, err := PrepareTaskWorktree(ctx, dispatch.Gate.RepositoryPath, path, dispatch.ArtifactVersion); err != nil {
		result.Reason = err.Error()
		return result
	}
	if output, err := gateCommand(ctx, path, "git", "merge-base", "--is-ancestor", dispatch.Gate.BaseCommit, dispatch.ArtifactVersion); err != nil {
		result.Reason = "submission does not descend from the authorized base: " + output
		return result
	}
	// Retain the submitted object even after a worktree or evaluation Agent is removed.
	ref := "refs/solo/submissions/" + dispatch.SubmissionID
	existing, refErr := gateCommand(ctx, path, "git", "rev-parse", "--verify", ref)
	if refErr == nil && strings.TrimSpace(existing) != dispatch.ArtifactVersion {
		result.Reason = "submission ref already names another commit"
		return result
	}
	if refErr != nil {
		if output, err := gateCommand(ctx, path, "git", "update-ref", ref, dispatch.ArtifactVersion, strings.Repeat("0", 40)); err != nil {
			result.Reason = "retain submission commit: " + output
			return result
		}
	}
	if err := verifyGateCommit(ctx, path, dispatch.ArtifactVersion); err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Decision = "accepted"
	result.Reason = "All configured checks passed at the submitted commit."
	for _, check := range dispatch.Gate.Checks {
		output, err := gateCommand(ctx, path, check.Command...)
		result.Checks = append(result.Checks, CodeCheckResult{RequirementID: check.RequirementID, Passed: err == nil, Output: fmt.Sprintf("commit: %s\ncommand: %q\n%s\nresult: %v", dispatch.ArtifactVersion, check.Command, output, err)})
		if err != nil {
			var exitErr *exec.ExitError
			if ctx.Err() != nil || !errors.As(err, &exitErr) {
				result.Decision = "needs_human"
				result.Reason = "Check environment failed or timed out: " + err.Error()
				return result
			}
			result.Decision = "rejected"
			result.Reason = "A configured check failed at the submitted commit."
		}
	}
	if err := verifyGateCommit(ctx, path, dispatch.ArtifactVersion); err != nil {
		result.Decision = "needs_human"
		result.Reason = err.Error()
	}
	return result
}
