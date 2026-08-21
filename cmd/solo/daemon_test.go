package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
	profile, explicit, remaining, err := parseDaemonProfile([]string{"--server", "https://solo.example", "--profile", "alpha"})
	if err != nil || profile != "alpha" || !explicit || len(remaining) != 2 {
		t.Fatalf("profile parse = %q %#v %v", profile, remaining, err)
	}
}

func TestDaemonConnectProfileUsesComputerIDUnlessExplicitOrLegacyDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	computerID := "11111111-1111-4111-8111-111111111111"
	if got := daemonConnectProfile(defaultDaemonProfile, computerID, false); got != computerID {
		t.Fatalf("new automatic profile = %q, want computer ID", got)
	}
	if got := daemonConnectProfile("work", computerID, true); got != "work" {
		t.Fatalf("explicit profile = %q, want work", got)
	}
	path, err := managedCredentialPath()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(managedDaemonCredential{ComputerID: computerID})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := daemonConnectProfile(defaultDaemonProfile, computerID, false); got != defaultDaemonProfile {
		t.Fatalf("legacy default profile = %q, want default", got)
	}
}

func TestDaemonProfilePIDRecoversFromLiveLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lockPath, err := daemonStatePath("lock.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]int{"pid": os.Getpid()})
	if err := os.WriteFile(lockPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if pid, running := daemonPID(); !running || pid != os.Getpid() {
		t.Fatalf("daemonPID() = %d, %v; want current process", pid, running)
	}
	pidPath, err := daemonStatePath("daemon.pid")
	if err != nil {
		t.Fatal(err)
	}
	if raw, err = os.ReadFile(pidPath); err != nil || strings.TrimSpace(string(raw)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("repaired pid record = %q, %v", raw, err)
	}
}
