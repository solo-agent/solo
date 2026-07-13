// Command solo CLI integration tests.
//
// Because the CLI handlers call os.Exit, we intercept it via a package-level
// doExit variable. Tests override doExit to panic with the exit code, then
// recover the panic to assert expected exit codes. stdout/stderr are captured
// via os.Pipe redirection.
//
// Tests for main-level logic (--help, no args, no token, unknown command) call
// runCLI directly, which returns an exit code without calling doExit.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// exitPanic is a sentinel type used to communicate exit codes from doExit
// (when overridden in tests) to the test runner via panic/recover.
type exitPanic struct{ code int }

// captureAndRun replaces doExit with a panic-based implementation, redirects
// stdout/stderr to pipes, runs fn, and recovers the exit code. It returns the
// captured exit code, stdout, and stderr.
func captureAndRun(t *testing.T, fn func()) (exitCode int, stdout, stderr string) {
	t.Helper()

	origExit := doExit
	doExit = func(code int) { panic(exitPanic{code}) }
	defer func() { doExit = origExit }()

	// Capture stdout.
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = wOut

	// Capture stderr.
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = wErr

	// Restore os.Stdout/os.Stderr on exit before reading pipes.
	defer func() {
		os.Stdout = origStdout
		wOut.Close()
		os.Stderr = origStderr
		wErr.Close()
	}()

	// Run fn, recovering from doExit panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				if ep, ok := r.(exitPanic); ok {
					exitCode = ep.code
				} else {
					panic(r) // unexpected panic — let test fail
				}
			}
		}()
		fn()
	}()

	// Close writers so io.ReadAll can finish.
	wOut.Close()
	wErr.Close()

	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	stdout = string(outBytes)
	stderr = string(errBytes)
	return
}

// ---------------------------------------------------------------------------
// runCLI tests (main-level logic — no handlers invoked)
// ---------------------------------------------------------------------------

func TestCLIHelpShortFlag(t *testing.T) {
	code, stdout, stderr := captureAndRun(t, func() {
		doExit(runCLI([]string{"--help"}))
	})
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected stderr to contain Usage:, got %q", stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestCLIHelpDashH(t *testing.T) {
	code, stdout, stderr := captureAndRun(t, func() {
		doExit(runCLI([]string{"-h"}))
	})
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected stderr to contain Usage:, got %q", stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestCLINoArgs(t *testing.T) {
	code, stdout, stderr := captureAndRun(t, func() {
		doExit(runCLI([]string{}))
	})
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected stderr to contain Usage:, got %q", stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestCLINoToken(t *testing.T) {
	t.Setenv("SOLO_AUTH_TOKEN", "")
	t.Setenv("SOLO_TOKEN", "")

	code, stdout, stderr := captureAndRun(t, func() {
		doExit(runCLI([]string{"task", "list"}))
	})
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
	if !strings.Contains(stderr, "authentication failed") {
		t.Errorf("expected auth error in stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "SOLO_AUTH_TOKEN is missing") {
		t.Errorf("expected SOLO_AUTH_TOKEN mention in stderr, got %q", stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	t.Setenv("SOLO_AUTH_TOKEN", "test-token")

	code, stdout, stderr := captureAndRun(t, func() {
		doExit(runCLI([]string{"foobar"}))
	})
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
	if !strings.Contains(stderr, `unknown command "foobar"`) {
		t.Errorf("expected unknown command error, got %q", stderr)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected Usage in stderr, got %q", stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestHandleArtifactPublishPostsHTMLFile(t *testing.T) {
	htmlPath := filepath.Join(t.TempDir(), "artifact.html")
	if err := os.WriteFile(htmlPath, []byte("<!doctype html><html><body>ok</body></html>"), 0o644); err != nil {
		t.Fatalf("write artifact file: %v", err)
	}

	var capturedMethod, capturedPath, capturedAuth string
	var capturedBody struct {
		Mode string `json:"mode"`
		HTML string `json:"html"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"artifact-1","task_id":"task-1","url":"/api/v1/artifacts/artifact-1"}`))
	}))
	defer server.Close()

	code, stdout, stderr := captureAndRun(t, func() {
		handleArtifact([]string{"publish", "--task", "task-1", "--mode", "final", "--file", htmlPath}, server.URL, "test-token")
	})

	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, stderr)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/tasks/task-1/artifact/publish" {
		t.Errorf("expected artifact publish path, got %s", capturedPath)
	}
	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Bearer token, got %q", capturedAuth)
	}
	if capturedBody.Mode != "final" {
		t.Errorf("expected mode final, got %q", capturedBody.Mode)
	}
	if !strings.Contains(capturedBody.HTML, "<body>ok</body>") {
		t.Errorf("expected HTML body in request, got %q", capturedBody.HTML)
	}
	if !strings.Contains(stdout, `"artifact-1"`) {
		t.Errorf("expected response JSON in stdout, got %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// task list tests
// ---------------------------------------------------------------------------

func TestHandleTaskListWithChannel(t *testing.T) {
	var capturedMethod, capturedPath, capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "task-1", "task_number": 1, "title": "Test Task"}})
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleTaskList([]string{"-c", "550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodGet {
		t.Errorf("expected GET, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks" {
		t.Errorf("expected path /api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks, got %s", capturedPath)
	}
	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Bearer token, got %q", capturedAuth)
	}
	if !strings.Contains(stdout, "Test Task") {
		t.Errorf("expected stdout to contain Test Task, got %q", stdout)
	}
}

func TestHandleTaskListOutputJSON(t *testing.T) {
	var capturedMethod, capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"1","task_number":1}]`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleTaskList([]string{"-c", "550e8400-e29b-41d4-a716-446655440002", "--output", "json"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodGet {
		t.Errorf("expected GET, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440002/tasks" {
		t.Errorf("expected path /api/v1/channels/550e8400-e29b-41d4-a716-446655440002/tasks, got %s", capturedPath)
	}

	// Verify JSON envelope: {"ok":true,"data":...}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v\nraw: %s", err, stdout)
	}
	if !envelope.OK {
		t.Errorf("expected ok=true, got %v", envelope.OK)
	}
	if len(envelope.Data) == 0 {
		t.Errorf("expected data field, got empty")
	}
}

func TestHandleTaskListAllChannels(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	code, _, _ := captureAndRun(t, func() {
		handleTaskList([]string{}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedPath != "/api/v1/tasks" {
		t.Errorf("expected path /api/v1/tasks, got %s", capturedPath)
	}
}

func TestHandleTaskListWithStatusFilter(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	code, _, _ := captureAndRun(t, func() {
		handleTaskList([]string{"-c", "550e8400-e29b-41d4-a716-446655440002", "--status", "in_progress"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(capturedPath, "?status=in_progress") {
		t.Errorf("expected status query param, got %s", capturedPath)
	}
}

// ---------------------------------------------------------------------------
// task claim tests
// ---------------------------------------------------------------------------

func TestHandleTaskClaimSuccess(t *testing.T) {
	var capturedMethod, capturedPath, capturedAuth, capturedContentType string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedContentType = r.Header.Get("Content-Type")
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"claimed","claimed_by":"user-1"}`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleTaskClaim([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/claim" {
		t.Errorf("expected path /api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/claim, got %s", capturedPath)
	}
	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Bearer token, got %q", capturedAuth)
	}
	// POST with nil body should send no Content-Type or Content-Type: application/json with empty body.
	// In the current implementation, Content-Type is only set when reqBody != nil.
	// For claim, body is nil, so no Content-Type header is expected.
	if capturedContentType != "" {
		t.Errorf("expected no Content-Type header for nil body, got %q", capturedContentType)
	}
	if len(capturedBody) > 0 {
		t.Errorf("expected empty body for claim, got %q", string(capturedBody))
	}
	if !strings.Contains(stdout, "claimed") {
		t.Errorf("expected stdout to contain 'claimed', got %q", stdout)
	}
}

func TestHandleTaskClaimConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"Conflict","message":"task is already claimed"}`))
	}))
	defer server.Close()

	code, stdout, stderr := captureAndRun(t, func() {
		handleTaskClaim([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token")
	})

	if code != 1 {
		t.Errorf("expected exit 1 for 409 Conflict, got %d", code)
	}
	// Phase 5 output: 409 Conflict emits a structured failure
	// message to stdout (not stderr).
	if !strings.Contains(stdout, "already assigned") {
		t.Errorf("expected 'already assigned' in stdout, got %q", stdout)
	}
	if !strings.Contains(stdout, "Claim results") {
		t.Errorf("expected 'Claim results' in stdout, got %q", stdout)
	}
	if !strings.Contains(stdout, "FAILED") {
		t.Errorf("expected 'FAILED' in stdout, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr on 409 conflict, got %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// task update tests
// ---------------------------------------------------------------------------

func TestHandleTaskUpdate(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1","status":"in_review"}`))
	}))
	defer server.Close()

	code, _, stderr := captureAndRun(t, func() {
		handleTaskUpdate([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440001", "-s", "in_review"}, server.URL, "test-token")
	})

	if code != exitUsage {
		t.Errorf("expected exit %d, got %d", exitUsage, code)
	}
	if capturedMethod != "" || capturedPath != "" || len(capturedBody) != 0 {
		t.Errorf("task update should not send lifecycle PATCH, got %s %s %s", capturedMethod, capturedPath, capturedBody)
	}
	if !strings.Contains(stderr, "no longer changes lifecycle status") {
		t.Errorf("expected deprecation error, got %q", stderr)
	}
}

func TestHandleTaskLifecycleSubmit(t *testing.T) {
	var capturedMethod, capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1","status":"in_review"}`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleTaskLifecycle([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token", "submit")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/submit" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(stdout, "in_review") {
		t.Errorf("expected stdout to contain in_review, got %q", stdout)
	}
}

func TestHandleTaskLifecycleRejectReason(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1","status":"in_progress"}`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleTaskLifecycle([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440001", "--reason", "needs tests"}, server.URL, "test-token", "reject")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/reject" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	var body map[string]string
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v\nraw: %s", err, capturedBody)
	}
	if body["reason"] != "needs tests" {
		t.Errorf("expected reason body, got %#v", body)
	}
	if !strings.Contains(stdout, "in_progress") {
		t.Errorf("expected stdout to contain in_progress, got %q", stdout)
	}
}

func TestHandleTaskLifecycleCloseReopen(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1","status":"closed"}`))
	}))
	defer server.Close()

	for _, action := range []string{"close", "reopen"} {
		code, _, _ := captureAndRun(t, func() {
			handleTaskLifecycle([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token", action)
		})
		if code != 0 {
			t.Fatalf("%s: expected exit 0, got %d", action, code)
		}
	}

	want := []string{
		"/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/close",
		"/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/reopen",
	}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Errorf("unexpected paths: got %v want %v", paths, want)
	}
}

// ---------------------------------------------------------------------------
// task create tests
// ---------------------------------------------------------------------------

func TestHandleTaskCreate(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"task-new","task_number":1,"title":"test"}`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleTaskCreate([]string{"-c", "550e8400-e29b-41d4-a716-446655440001", "--title", "test"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks" {
		t.Errorf("expected path /api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks, got %s", capturedPath)
	}
	if !strings.Contains(stdout, "test") {
		t.Errorf("expected stdout to contain 'test', got %q", stdout)
	}

	var body map[string]string
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v\nraw: %s", err, capturedBody)
	}
	if body["title"] != "test" {
		t.Errorf("expected title=test, got %s", body["title"])
	}
}

// ---------------------------------------------------------------------------
// task unclaim tests
// ---------------------------------------------------------------------------

func TestHandleTaskUnclaim(t *testing.T) {
	var capturedMethod, capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"unclaimed"}`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleTaskUnclaim([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/claim" {
		t.Errorf("expected path /api/v1/channels/550e8400-e29b-41d4-a716-446655440001/tasks/1/claim, got %s", capturedPath)
	}
	if !strings.Contains(stdout, "unclaimed") {
		t.Errorf("expected stdout to contain 'unclaimed', got %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// message send tests
// ---------------------------------------------------------------------------

func TestHandleMessageSend(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/server/info" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"channels":[{"id":"550e8400-e29b-41d4-a716-446655440001","name":"ch-abc"}]}`))
			return
		}
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"msg-1","content":"hello"}`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleMessageSend([]string{"-c", "hello", "--target", "#550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/messages" {
		t.Errorf("expected path /api/v1/channels/550e8400-e29b-41d4-a716-446655440001/messages, got %s", capturedPath)
	}
	// Phase 5 output: formatted send confirmation (not raw JSON).
	if !strings.Contains(stdout, "Message sent") {
		t.Errorf("expected stdout to contain 'Message sent', got %q", stdout)
	}
	if !strings.Contains(stdout, "msg-1") {
		t.Errorf("expected stdout to contain message ID, got %q", stdout)
	}

	var body map[string]string
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v\nraw: %s", err, capturedBody)
	}
	if body["content"] != "hello" {
		t.Errorf("expected content=hello, got %s", body["content"])
	}
	if _, ok := body["thread_id"]; ok {
		t.Errorf("expected no thread_id when -t not provided, got %v", body["thread_id"])
	}
}

func TestHandleMessageSendWithThread(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/server/info" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"channels":[{"id":"550e8400-e29b-41d4-a716-446655440001","name":"ch-abc"}]}`))
			return
		}
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"msg-2","content":"hello","thread_id":"th-1"}`))
	}))
	defer server.Close()

	code, _, _ := captureAndRun(t, func() {
		handleMessageSend([]string{"-c", "hello", "--target", "#ch-abc:th-1"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}

	var body map[string]string
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("failed to unmarshal body: %v\nraw: %s", err, capturedBody)
	}
	if body["content"] != "hello" {
		t.Errorf("expected content=hello, got %s", body["content"])
	}
	if body["thread_id"] != "th-1" {
		t.Errorf("expected thread_id=th-1, got %s", body["thread_id"])
	}
}

// ---------------------------------------------------------------------------
// channel members tests
// ---------------------------------------------------------------------------

func TestHandleChannelMembers(t *testing.T) {
	var capturedMethod, capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"u1","name":"Alice"},{"id":"u2","name":"Bob"}]`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleChannelMembers([]string{"-c", "550e8400-e29b-41d4-a716-446655440001"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if capturedMethod != http.MethodGet {
		t.Errorf("expected GET, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/channels/550e8400-e29b-41d4-a716-446655440001/members" {
		t.Errorf("expected path /api/v1/channels/550e8400-e29b-41d4-a716-446655440001/members, got %s", capturedPath)
	}
	if !strings.Contains(stdout, "Alice") || !strings.Contains(stdout, "Bob") {
		t.Errorf("expected stdout to contain Alice and Bob, got %q", stdout)
	}
}

func TestHandleChannelMembersOutputJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"u1","name":"Alice"}]`))
	}))
	defer server.Close()

	code, stdout, _ := captureAndRun(t, func() {
		handleChannelMembers([]string{"-c", "550e8400-e29b-41d4-a716-446655440002", "--output", "json"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}

	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v\nraw: %s", err, stdout)
	}
	if !envelope.OK {
		t.Errorf("expected ok=true, got %v", envelope.OK)
	}
	if len(envelope.Data) == 0 {
		t.Errorf("expected data field, got empty")
	}
}

// ---------------------------------------------------------------------------
// Flag parsing tests (short vs long flags)
// ---------------------------------------------------------------------------

func TestHandleTaskClaimLongFlags(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	channelUUID := "550e8400-e29b-41d4-a716-446655440001"
	code, _, _ := captureAndRun(t, func() {
		handleTaskClaim([]string{"--number", "42", "--channel", channelUUID}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	expectedPath := "/api/v1/channels/" + channelUUID + "/tasks/42/claim"
	if capturedPath != expectedPath {
		t.Errorf("expected path with long flags, got %s", capturedPath)
	}
}

func TestHandleTaskUpdateLongFlags(t *testing.T) {
	var capturedMethod string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	channelUUID := "550e8400-e29b-41d4-a716-446655440002"
	code, _, stderr := captureAndRun(t, func() {
		handleTaskUpdate([]string{"--number", "7", "--channel", channelUUID, "--status", "done"}, server.URL, "test-token")
	})

	if code != exitUsage {
		t.Errorf("expected exit %d, got %d", exitUsage, code)
	}
	if capturedMethod != "" || len(capturedBody) != 0 {
		t.Errorf("task update should not send PATCH, got %s %s", capturedMethod, capturedBody)
	}
	if !strings.Contains(stderr, "no longer changes lifecycle status") {
		t.Errorf("expected deprecation error, got %q", stderr)
	}
}

func TestHandleMessageSendLongFlags(t *testing.T) {
	var capturedPath string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/server/info" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"channels":[{"id":"550e8400-e29b-41d4-a716-446655440002","name":"ch-1"}]}`))
			return
		}
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	code, _, _ := captureAndRun(t, func() {
		handleMessageSend([]string{"-c", "hi", "--target", "#ch-1:th-2"}, server.URL, "test-token")
	})

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.HasPrefix(capturedPath, "/api/v1/channels/") {
		t.Errorf("expected path with long flags, got %s", capturedPath)
	}

	var body map[string]string
	json.Unmarshal(capturedBody, &body)
	if body["content"] != "hi" {
		t.Errorf("expected content=hi, got %s", body["content"])
	}
	if body["thread_id"] == "" {
		t.Errorf("expected thread_id=th-2, got %s", body["thread_id"])
	}
}

// ---------------------------------------------------------------------------
// HTTP helper tests
// ---------------------------------------------------------------------------

func TestDoHTTPGET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("expected Bearer tok, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	code, body, err := doHTTP(http.MethodGet, server.URL+"/test", "tok", nil)
	if err != nil {
		t.Fatalf("doHTTP failed: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("expected 200, got %d", code)
	}
	if !bytes.Contains(body, []byte(`"ok":true`)) {
		t.Errorf("expected ok:true in body, got %s", body)
	}
}

func TestDoHTTPPOST(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"key":"value"`)) {
			t.Errorf("expected JSON body, got %s", body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`created`))
	}))
	defer server.Close()

	code, body, err := doHTTP(http.MethodPost, server.URL+"/test", "tok", []byte(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("doHTTP failed: %v", err)
	}
	if code != http.StatusCreated {
		t.Errorf("expected 201, got %d", code)
	}
	if !bytes.Equal(body, []byte("created")) {
		t.Errorf("expected 'created' body, got %s", body)
	}
}

func TestDoHTTPConnectionRefused(t *testing.T) {
	// Connect to a port that nothing is listening on.
	_, _, err := doHTTP(http.MethodGet, "http://127.0.0.1:19999/nope", "tok", nil)
	if err == nil {
		t.Error("expected connection error, got nil")
	}
	if !strings.Contains(err.Error(), "http request") {
		t.Errorf("expected http request error, got %v", err)
	}
}

func TestDoHTTPInvalidURL(t *testing.T) {
	_, _, err := doHTTP(http.MethodGet, "://invalid", "tok", nil)
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "create request") {
		t.Errorf("expected create request error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Error helpers tests
// ---------------------------------------------------------------------------

func TestExtractErrorMessageJSON(t *testing.T) {
	body := []byte(`{"error":"Conflict","message":"task already claimed by user-2"}`)
	msg := extractErrorMessage(body)
	if msg != "task already claimed by user-2" {
		t.Errorf("expected message text, got %q", msg)
	}
}

func TestExtractErrorMessageFallback(t *testing.T) {
	body := []byte("Internal Server Error")
	msg := extractErrorMessage(body)
	if msg != "Internal Server Error" {
		t.Errorf("expected fallback to raw body, got %q", msg)
	}
}

func TestExtractErrorMessageNoMessage(t *testing.T) {
	body := []byte(`{"error":"Not Found"}`)
	msg := extractErrorMessage(body)
	if msg != "Not Found" {
		t.Errorf("expected error text, got %q", msg)
	}
}

func TestPrintJSONEnvelope(t *testing.T) {
	// Redirect stdout to capture output.
	r, w, _ := os.Pipe()
	orig := os.Stdout
	os.Stdout = w

	printJSONEnvelope([]byte(`[{"id":"1"}]`))
	w.Close()
	os.Stdout = orig

	out, _ := io.ReadAll(r)
	var env struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Errorf("expected ok=true, got %v", env.OK)
	}
	if !bytes.Contains(env.Data, []byte(`"id"`)) {
		t.Errorf("expected data to contain id field, got %s", env.Data)
	}
}

func TestPrintJSONEnvelopePlainText(t *testing.T) {
	r, w, _ := os.Pipe()
	orig := os.Stdout
	os.Stdout = w

	printJSONEnvelope([]byte("plain text response"))
	w.Close()
	os.Stdout = orig

	out, _ := io.ReadAll(r)
	var env struct {
		OK   bool   `json:"ok"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Errorf("expected ok=true, got %v", env.OK)
	}
	if env.Data != "plain text response" {
		t.Errorf("expected data='plain text response', got %q", env.Data)
	}
}

func TestPrintJSONError(t *testing.T) {
	r, w, _ := os.Pipe()
	orig := os.Stdout
	os.Stdout = w

	printJSONError(404, "task not found")
	w.Close()
	os.Stdout = orig

	out, _ := io.ReadAll(r)
	var errResp struct {
		OK      string `json:"ok"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(out, &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp.OK != "false" {
		t.Errorf("expected ok='false', got %q", errResp.OK)
	}
	if errResp.Code != "404" {
		t.Errorf("expected code=404, got %q", errResp.Code)
	}
	if errResp.Message != "task not found" {
		t.Errorf("expected message, got %q", errResp.Message)
	}
}

// ---------------------------------------------------------------------------
// Required arg validation tests
// ---------------------------------------------------------------------------

func TestHandleTaskClaimMissingChannel(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleTaskClaim([]string{"-n", "1"}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing channel, got %d", code)
	}
	if !strings.Contains(stderr, "-c") {
		t.Errorf("expected -c error, got %q", stderr)
	}
}

func TestHandleTaskClaimMissingNumber(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleTaskClaim([]string{"-c", "550e8400-e29b-41d4-a716-446655440002"}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing -n, got %d", code)
	}
	if !strings.Contains(stderr, "-n") {
		t.Errorf("expected -n error, got %q", stderr)
	}
}

func TestHandleTaskUpdateMissingStatus(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleTaskUpdate([]string{"-n", "1", "-c", "550e8400-e29b-41d4-a716-446655440002"}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing -s, got %d", code)
	}
	if !strings.Contains(stderr, "-s") {
		t.Errorf("expected -s error, got %q", stderr)
	}
}

func TestHandleTaskCreateMissingChannel(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleTaskCreate([]string{"--title", "test"}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing channel, got %d", code)
	}
	if !strings.Contains(stderr, "-c") {
		t.Errorf("expected -c error, got %q", stderr)
	}
}

func TestHandleTaskCreateMissingTitle(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleTaskCreate([]string{"-c", "550e8400-e29b-41d4-a716-446655440002"}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing --title, got %d", code)
	}
	if !strings.Contains(stderr, "--title") {
		t.Errorf("expected --title error, got %q", stderr)
	}
}

func TestHandleMessageSendMissingContent(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleMessageSend([]string{"--target", "#550e8400-e29b-41d4-a716-446655440002"}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing -c, got %d", code)
	}
	if !strings.Contains(stderr, "-c") {
		t.Errorf("expected -c error, got %q", stderr)
	}
}

func TestHandleMessageSendMissingChannel(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleMessageSend([]string{"-c", "hello"}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing target, got %d", code)
	}
	if !strings.Contains(stderr, "--target") {
		t.Errorf("expected --target error, got %q", stderr)
	}
}

func TestHandleChannelMembersMissingChannel(t *testing.T) {
	code, _, stderr := captureAndRun(t, func() {
		handleChannelMembers([]string{}, "http://localhost", "tok")
	})
	if code != 2 {
		t.Errorf("expected exit 2 for missing channel, got %d", code)
	}
	if !strings.Contains(stderr, "-c") {
		t.Errorf("expected -c error, got %q", stderr)
	}
}

func TestHandleTeamFormSendsDeclarativePlan(t *testing.T) {
	const channelID = "550e8400-e29b-41d4-a716-446655440099"
	const sourceMessageID = "a1b2c3d4"

	var gotAuth string
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/api/v1/team-formations" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			SourceChannelID string          `json:"source_channel_id"`
			SourceMessageID string          `json:"source_message_id"`
			Plan            json.RawMessage `json:"plan"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.SourceChannelID != channelID || req.SourceMessageID != sourceMessageID {
			t.Fatalf("unexpected source: channel=%q message=%q", req.SourceChannelID, req.SourceMessageID)
		}
		if !bytes.Contains(req.Plan, []byte(`"intent_summary":"Ship billing"`)) {
			t.Fatalf("plan missing intent: %s", req.Plan)
		}
		if !bytes.Contains(req.Plan, []byte(`"relationship_template":"dev-team"`)) || bytes.Contains(req.Plan, []byte(`"tasks"`)) {
			t.Fatalf("plan must select a relationship template without tasks: %s", req.Plan)
		}
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":"team formation is already in progress"}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"formation_id":"formation-1","channel_id":"team-1","channel_name":"billing-team","dashboard_url":"/dashboard?channel=team-1","members":[{"name":"Lead","role":"leader"},{"name":"Engineer","role":"engineer"}],"tasks":[],"relationship_template":"dev-team","relationship_override_count":1,"relationship_docs_ready":false,"warnings":["relationship docs pending"]}`)
	}))
	defer server.Close()

	planFile := filepath.Join(t.TempDir(), "plan.json")
	plan := `{"intent_summary":"Ship billing","channel":{"name":"billing-team","description":""},"relationship_template":"dev-team","members":[{"ref":"lead","role":"leader","name":"Lead","description":"Lead","instructions":"Coordinate"},{"ref":"engineer","role":"engineer","name":"Engineer","description":"Build","instructions":"Implement"}],"relationship_overrides":[]}`
	if err := os.WriteFile(planFile, []byte(plan), 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	t.Setenv("SOLO_AGENT_ID", "") // Exercise the direct API fallback.

	code, stdout, stderr := captureAndRun(t, func() {
		handleTeamForm([]string{"-c", channelID, "-m", sourceMessageID, "--plan", planFile}, server.URL, "test-token")
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, stderr)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
	for _, want := range []string{"Team formed: #billing-team", "@Lead (leader)", "Relationships: dev-team (+1 Lucy adjustment(s))", "Warning: relationship docs pending", "/dashboard?channel=team-1"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout %q does not contain %q", stdout, want)
		}
	}
}

func TestHandleTeamFormRejectsInvalidPlan(t *testing.T) {
	planFile := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(planFile, []byte(`[]`), 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	code, _, stderr := captureAndRun(t, func() {
		handleTeamForm([]string{
			"-c", "550e8400-e29b-41d4-a716-446655440099",
			"-m", "a1b2c3d4",
			"--plan", planFile,
		}, "http://localhost", "test-token")
	})
	if code != exitUsage || !strings.Contains(stderr, "JSON object") {
		t.Fatalf("expected JSON object usage error, code=%d stderr=%q", code, stderr)
	}
}

func TestProxyRequestTimeoutAllowsTeamFormationToFinish(t *testing.T) {
	if got := proxyRequestTimeout("team_form"); got != 60*time.Second {
		t.Fatalf("team_form timeout = %s, want 60s", got)
	}
	if got := proxyRequestTimeout("message_send"); got != 30*time.Second {
		t.Fatalf("message_send timeout = %s, want 30s", got)
	}
}

func TestIsTeamFormationInProgress(t *testing.T) {
	if !isTeamFormationInProgress(http.StatusConflict, []byte(`{"error":"team formation is already in progress"}`)) {
		t.Fatal("expected in-progress response to be retryable")
	}
	if isTeamFormationInProgress(http.StatusBadRequest, []byte(`{"error":"already in progress"}`)) {
		t.Fatal("bad request must not be retried")
	}
}
