package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagedDaemonConnectedRequiresReadyControlChannel(t *testing.T) {
	ready := false
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if ready {
			w.Write([]byte(`{"control_connected":true}`))
			return
		}
		w.Write([]byte(`{"control_connected":false}`))
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	defer server.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	t.Setenv("DAEMON_PORT", port)

	if managedDaemonConnected() {
		t.Fatal("Daemon reported connected before the remote control channel was ready")
	}
	ready = true
	if !managedDaemonConnected() {
		t.Fatal("Daemon did not report the ready remote control channel")
	}
}

func TestStartManagedDaemonUsesSoloStateDirectory(t *testing.T) {
	home := t.TempDir()
	callerDir := t.TempDir()
	observedCWD := filepath.Join(t.TempDir(), "cwd")
	observedCredential := filepath.Join(t.TempDir(), "credential")
	helper := filepath.Join(t.TempDir(), "solo-daemon")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ntrap 'exit 0' TERM INT\npwd > \"$OBSERVED_CWD\"\nprintf '%s\\n' \"$SOLO_DAEMON_CREDENTIAL_FILE\" > \"$OBSERVED_CREDENTIAL\"\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(callerDir, ".env"), []byte("DAEMON_SERVER_URL=http://wrong.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(callerDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCWD) })
	t.Setenv("HOME", home)
	t.Setenv("SOLO_DAEMON_BINARY", helper)
	t.Setenv("OBSERVED_CWD", observedCWD)
	t.Setenv("OBSERVED_CREDENTIAL", observedCredential)
	t.Setenv("SOLO_DAEMON_CREDENTIAL_FILE", "relative-credentials.json")

	if err := startManagedDaemon(nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if pid, running := daemonPID(); running {
			if process, findErr := os.FindProcess(pid); findErr == nil {
				_ = process.Kill()
			}
		}
		if pidPath, pathErr := daemonStatePath("daemon.pid"); pathErr == nil {
			_ = os.Remove(pidPath)
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	var raw []byte
	for time.Now().Before(deadline) {
		raw, err = os.ReadFile(observedCWD)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read child working directory: %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(home, ".solo", "daemon"))
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Clean(strings.TrimSpace(string(raw))); got != want {
		t.Fatalf("child working directory = %q, want %q", got, want)
	}
	raw, err = os.ReadFile(observedCredential)
	if err != nil {
		t.Fatal(err)
	}
	resolvedCallerDir, err := filepath.EvalSymlinks(callerDir)
	if err != nil {
		t.Fatal(err)
	}
	wantCredential := filepath.Join(resolvedCallerDir, "relative-credentials.json")
	if got := strings.TrimSpace(string(raw)); got != wantCredential {
		t.Fatalf("child credential path = %q, want %q", got, wantCredential)
	}
}

func TestDaemonProfilesUseIndependentState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	alpha, err := daemonProfileStateDir("alpha")
	if err != nil {
		t.Fatal(err)
	}
	beta, err := daemonProfileStateDir("beta")
	if err != nil {
		t.Fatal(err)
	}
	if alpha == beta || !strings.HasSuffix(alpha, filepath.Join("daemons", "alpha")) || !strings.HasSuffix(beta, filepath.Join("daemons", "beta")) {
		t.Fatalf("profiles share state: alpha=%q beta=%q", alpha, beta)
	}
	profile, remaining, err := parseDaemonProfile([]string{"--server", "https://solo.example", "--profile", "alpha"})
	if err != nil || profile != "alpha" || len(remaining) != 2 {
		t.Fatalf("profile parse = %q %#v %v", profile, remaining, err)
	}
}
