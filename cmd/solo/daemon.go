package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type managedDaemonCredential struct {
	ServerURL  string `json:"server_url"`
	ComputerID string `json:"computer_id"`
	Credential string `json:"credential"`
}

func handleDaemonCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "solo: daemon command required: connect, start, stop, restart, status, logs")
		return exitUsage
	}
	var err error
	switch args[0] {
	case "connect":
		err = daemonConnect(args[1:])
	case "start":
		err = startManagedDaemon(nil)
	case "stop":
		err = stopManagedDaemon()
	case "restart":
		if err = stopManagedDaemon(); err == nil {
			err = startManagedDaemon(nil)
		}
	case "status":
		err = printDaemonStatus()
	case "logs":
		err = printDaemonLogs()
	default:
		err = fmt.Errorf("unknown daemon command %q", args[0])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: daemon: %v\n", err)
		return exitUsage
	}
	return exitOK
}

func daemonStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".solo", "daemon"), nil
}

func daemonStatePath(name string) (string, error) {
	dir, err := daemonStateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func managedCredentialPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("SOLO_DAEMON_CREDENTIAL_FILE")); path != "" {
		return path, nil
	}
	return daemonStatePath("credentials.json")
}

func daemonBinary() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("SOLO_DAEMON_BINARY")); configured != "" {
		return configured, nil
	}
	if current, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(current), "solo-daemon")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	path, err := exec.LookPath("solo-daemon")
	if err != nil {
		return "", errors.New("solo-daemon is not installed next to solo or on PATH")
	}
	return path, nil
}

func daemonPID() (int, bool) {
	path, err := daemonStatePath("daemon.pid")
	if err != nil {
		return 0, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 1 {
		return 0, false
	}
	process, err := os.FindProcess(pid)
	if err != nil || process.Signal(syscall.Signal(0)) != nil {
		_ = os.Remove(path)
		return 0, false
	}
	return pid, true
}

func startManagedDaemon(extraEnv []string) error {
	if pid, running := daemonPID(); running {
		fmt.Printf("Solo Daemon is already running (pid %d).\n", pid)
		return nil
	}
	binary, err := daemonBinary()
	if err != nil {
		return err
	}
	logPath, err := daemonStatePath("daemon.log")
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(binary)
	credentialPath, err := managedCredentialPath()
	if err != nil {
		return err
	}
	cmd.Env = append(os.Environ(), "SOLO_DAEMON_CREDENTIAL_FILE="+credentialPath)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	pidPath, err := daemonStatePath("daemon.pid")
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	_ = cmd.Process.Release()
	time.Sleep(350 * time.Millisecond)
	if _, running := daemonPID(); !running {
		return fmt.Errorf("failed to start; inspect %s", logPath)
	}
	fmt.Printf("Solo Daemon started (pid %d). Logs: %s\n", cmd.Process.Pid, logPath)
	return nil
}

func stopManagedDaemon() error {
	pid, running := daemonPID()
	if !running {
		fmt.Println("Solo Daemon is not running.")
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, running := daemonPID(); !running {
			fmt.Println("Solo Daemon stopped.")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("Daemon did not stop within 10 seconds")
}

func daemonConnect(args []string) error {
	fs := flag.NewFlagSet("solo daemon connect", flag.ContinueOnError)
	server := fs.String("server", "", "Solo Server URL")
	computerID := fs.String("computer-id", "", "Computer ID")
	token := fs.String("token", "", "one-time enrollment token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(*server), "/"))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1"))) {
		return errors.New("--server must be an https URL (http is allowed only for localhost)")
	}
	if _, err := uuid.Parse(strings.TrimSpace(*computerID)); err != nil {
		return errors.New("--computer-id must be a valid UUID")
	}
	if strings.TrimSpace(*token) == "" {
		return errors.New("--token is required")
	}
	credentialPath, err := managedCredentialPath()
	if err != nil {
		return err
	}
	oldCredential, _ := os.ReadFile(credentialPath)
	if err := stopManagedDaemon(); err != nil {
		return err
	}
	serverURL := strings.TrimRight(parsed.String(), "/")
	if err := startManagedDaemon([]string{
		"DAEMON_SERVER_URL=" + serverURL,
		"SOLO_COMPUTER_ID=" + strings.TrimSpace(*computerID),
		"SOLO_ENROLLMENT_TOKEN=" + strings.TrimSpace(*token),
	}); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		raw, readErr := os.ReadFile(credentialPath)
		var credential managedDaemonCredential
		if readErr == nil && !bytes.Equal(raw, oldCredential) && json.Unmarshal(raw, &credential) == nil && credential.ComputerID == strings.TrimSpace(*computerID) && strings.TrimRight(credential.ServerURL, "/") == serverURL && credential.Credential != "" && managedDaemonConnected() {
			fmt.Printf("Computer paired with %s. The Daemon will reconnect automatically.\n", serverURL)
			return nil
		}
		if _, running := daemonPID(); !running {
			logPath, _ := daemonStatePath("daemon.log")
			return fmt.Errorf("pairing failed; inspect %s", logPath)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("pairing timed out after 15 seconds")
}

func printDaemonStatus() error {
	pid, running := daemonPID()
	if !running {
		return errors.New("not running")
	}
	fmt.Printf("Solo Daemon is running (pid %d).\n", pid)
	fmt.Printf("Remote control: %s\n", map[bool]string{true: "connected", false: "connecting"}[managedDaemonConnected()])
	path, _ := managedCredentialPath()
	if raw, err := os.ReadFile(path); err == nil {
		var credential managedDaemonCredential
		if json.Unmarshal(raw, &credential) == nil {
			fmt.Printf("Server: %s\nComputer: %s\n", credential.ServerURL, credential.ComputerID)
		}
	}
	return nil
}

func managedDaemonConnected() bool {
	port := strings.TrimSpace(os.Getenv("DAEMON_PORT"))
	if port == "" {
		port = "8081"
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	var health struct {
		ControlConnected bool `json:"control_connected"`
	}
	return response.StatusCode == http.StatusOK && json.NewDecoder(response.Body).Decode(&health) == nil && health.ControlConnected
}

func printDaemonLogs() error {
	path, err := daemonStatePath("daemon.log")
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > 100 {
		lines = lines[len(lines)-100:]
	}
	fmt.Println(strings.Join(lines, "\n"))
	return nil
}
