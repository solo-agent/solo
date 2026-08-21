package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
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

const defaultDaemonProfile = "default"

func handleDaemonCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "solo: daemon command required: connect, start, stop, restart, status, logs")
		return exitUsage
	}
	profile, profileExplicit, commandArgs, err := parseDaemonProfile(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: daemon: %v\n", err)
		return exitUsage
	}
	switch args[0] {
	case "connect":
		err = daemonConnect(commandArgs, profile, profileExplicit)
	case "start":
		err = startManagedDaemonProfile(profile, nil)
	case "stop":
		err = stopManagedDaemonProfile(profile)
	case "restart":
		if err = stopManagedDaemonProfile(profile); err == nil {
			err = startManagedDaemonProfile(profile, nil)
		}
	case "status":
		err = printDaemonStatusProfile(profile)
	case "logs":
		err = printDaemonLogsProfile(profile)
	default:
		err = fmt.Errorf("unknown daemon command %q", args[0])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "solo: daemon: %v\n", err)
		return exitUsage
	}
	return exitOK
}

func parseDaemonProfile(args []string) (string, bool, []string, error) {
	profile := defaultDaemonProfile
	explicit := false
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--profile" {
			if i+1 >= len(args) {
				return "", false, nil, errors.New("--profile requires a name")
			}
			profile = args[i+1]
			explicit = true
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--profile=") {
			profile = strings.TrimPrefix(args[i], "--profile=")
			explicit = true
			continue
		}
		result = append(result, args[i])
	}
	profile = strings.TrimSpace(profile)
	if profile == "" || len(profile) > 64 {
		return "", false, nil, errors.New("invalid Daemon profile name")
	}
	for _, r := range profile {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return "", false, nil, errors.New("Daemon profile may contain only letters, numbers, '-' and '_'")
		}
	}
	return profile, explicit, result, nil
}

func daemonStateDir() (string, error) {
	return daemonProfileStateDir(defaultDaemonProfile)
}

func daemonProfileStateDir(profile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if profile == defaultDaemonProfile {
		return filepath.Join(home, ".solo", "daemon"), nil
	}
	return filepath.Join(home, ".solo", "daemons", profile), nil
}

func daemonStatePath(name string) (string, error) {
	return daemonProfileStatePath(defaultDaemonProfile, name)
}

func daemonProfileStatePath(profile, name string) (string, error) {
	dir, err := daemonProfileStateDir(profile)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func managedCredentialPath() (string, error) {
	return managedProfileCredentialPath(defaultDaemonProfile)
}

func managedProfileCredentialPath(profile string) (string, error) {
	if path := strings.TrimSpace(os.Getenv("SOLO_DAEMON_CREDENTIAL_FILE")); path != "" {
		if profile != defaultDaemonProfile {
			return "", errors.New("SOLO_DAEMON_CREDENTIAL_FILE cannot be shared by named profiles")
		}
		return path, nil
	}
	return daemonProfileStatePath(profile, "credentials.json")
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
	return daemonProfilePID(defaultDaemonProfile)
}

func daemonProfilePID(profile string) (int, bool) {
	path, err := daemonProfileStatePath(profile, "daemon.pid")
	if err != nil {
		return 0, false
	}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw))); parseErr == nil && daemonProcessAlive(pid) {
			return pid, true
		}
		_ = os.Remove(path)
	}
	var lock struct {
		PID int `json:"pid"`
	}
	if raw, readErr := os.ReadFile(filepath.Join(filepath.Dir(path), "lock.json")); readErr == nil && json.Unmarshal(raw, &lock) == nil && daemonProcessAlive(lock.PID) {
		_ = os.WriteFile(path, []byte(strconv.Itoa(lock.PID)+"\n"), 0o600)
		return lock.PID, true
	}
	return 0, false
}

func daemonProcessAlive(pid int) bool {
	if pid <= 1 {
		return false
	}
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(syscall.Signal(0)) == nil
}

func startManagedDaemon(extraEnv []string) error {
	return startManagedDaemonProfile(defaultDaemonProfile, extraEnv)
}

func startManagedDaemonProfile(profile string, extraEnv []string) error {
	if pid, running := daemonProfilePID(profile); running {
		fmt.Printf("Solo Daemon %q is already running (pid %d).\n", profile, pid)
		return nil
	}
	binary, err := daemonBinary()
	if err != nil {
		return err
	}
	logPath, err := daemonProfileStatePath(profile, "daemon.log")
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(binary)
	credentialPath, err := managedProfileCredentialPath(profile)
	if err != nil {
		return err
	}
	credentialPath, err = filepath.Abs(credentialPath)
	if err != nil {
		return err
	}
	// A managed Daemon belongs to Solo, not to the project directory where the
	// user happened to invoke the CLI. Keep config.LoadDotenv in the child from
	// loading an unrelated project .env and overriding the persisted pairing.
	cmd.Dir = filepath.Dir(logPath)
	port, err := daemonProfilePort(profile)
	if err != nil {
		return err
	}
	stateDir := filepath.Dir(logPath)
	cmd.Env = append(os.Environ(),
		"SOLO_DAEMON_CREDENTIAL_FILE="+credentialPath,
		"SOLO_DAEMON_STATE_DIR="+stateDir,
		"SOLO_DAEMON_PROFILE="+profile,
		"DAEMON_ID=daemon-"+profile,
		"DAEMON_PORT="+strconv.Itoa(port),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	pidPath, err := daemonProfileStatePath(profile, "daemon.pid")
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	_ = cmd.Process.Release()
	time.Sleep(350 * time.Millisecond)
	if _, running := daemonProfilePID(profile); !running {
		return fmt.Errorf("failed to start; inspect %s", logPath)
	}
	fmt.Printf("Solo Daemon %q started (pid %d, port %d). Logs: %s\n", profile, pid, port, logPath)
	return nil
}

func stopManagedDaemon() error {
	return stopManagedDaemonProfile(defaultDaemonProfile)
}

func stopManagedDaemonProfile(profile string) error {
	pid, running := daemonProfilePID(profile)
	if !running {
		fmt.Printf("Solo Daemon %q is not running.\n", profile)
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
		if _, running := daemonProfilePID(profile); !running {
			fmt.Printf("Solo Daemon %q stopped.\n", profile)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("Daemon did not stop within 10 seconds")
}

func daemonConnect(args []string, profile string, profileExplicit bool) error {
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
	computer := strings.TrimSpace(*computerID)
	if _, err := uuid.Parse(computer); err != nil {
		return errors.New("--computer-id must be a valid UUID")
	}
	if strings.TrimSpace(*token) == "" {
		return errors.New("--token is required")
	}
	profile = daemonConnectProfile(profile, computer, profileExplicit)
	credentialPath, err := managedProfileCredentialPath(profile)
	if err != nil {
		return err
	}
	oldCredential, _ := os.ReadFile(credentialPath)
	if err := stopManagedDaemonProfile(profile); err != nil {
		return err
	}
	serverURL := strings.TrimRight(parsed.String(), "/")
	if err := startManagedDaemonProfile(profile, []string{
		"DAEMON_SERVER_URL=" + serverURL,
		"SOLO_COMPUTER_ID=" + computer,
		"SOLO_ENROLLMENT_TOKEN=" + strings.TrimSpace(*token),
	}); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		raw, readErr := os.ReadFile(credentialPath)
		var credential managedDaemonCredential
		if readErr == nil && !bytes.Equal(raw, oldCredential) && json.Unmarshal(raw, &credential) == nil && credential.ComputerID == computer && strings.TrimRight(credential.ServerURL, "/") == serverURL && credential.Credential != "" && managedDaemonConnectedProfile(profile) {
			fmt.Printf("Computer paired with %s. The Daemon will reconnect automatically.\n", serverURL)
			return nil
		}
		if _, running := daemonProfilePID(profile); !running {
			logPath, _ := daemonProfileStatePath(profile, "daemon.log")
			return fmt.Errorf("pairing failed; inspect %s", logPath)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("pairing timed out after 15 seconds")
}

func daemonConnectProfile(profile, computerID string, explicit bool) string {
	if explicit {
		return profile
	}
	path, err := managedProfileCredentialPath(defaultDaemonProfile)
	if err == nil {
		var credential managedDaemonCredential
		if raw, readErr := os.ReadFile(path); readErr == nil && json.Unmarshal(raw, &credential) == nil && credential.ComputerID == computerID {
			return defaultDaemonProfile
		}
	}
	return computerID
}

func printDaemonStatus() error {
	return printDaemonStatusProfile(defaultDaemonProfile)
}

func printDaemonStatusProfile(profile string) error {
	pid, running := daemonProfilePID(profile)
	if !running {
		return errors.New("not running")
	}
	fmt.Printf("Solo Daemon %q is running (pid %d).\n", profile, pid)
	fmt.Printf("Remote control: %s\n", map[bool]string{true: "connected", false: "connecting"}[managedDaemonConnectedProfile(profile)])
	path, _ := managedProfileCredentialPath(profile)
	if raw, err := os.ReadFile(path); err == nil {
		var credential managedDaemonCredential
		if json.Unmarshal(raw, &credential) == nil {
			fmt.Printf("Server: %s\nComputer: %s\n", credential.ServerURL, credential.ComputerID)
		}
	}
	return nil
}

func managedDaemonConnected() bool {
	return managedDaemonConnectedProfile(defaultDaemonProfile)
}

func managedDaemonConnectedProfile(profile string) bool {
	port, err := daemonProfilePort(profile)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health")
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
	return printDaemonLogsProfile(defaultDaemonProfile)
}

func printDaemonLogsProfile(profile string) error {
	path, err := daemonProfileStatePath(profile, "daemon.log")
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

func daemonProfilePort(profile string) (int, error) {
	if profile == defaultDaemonProfile {
		if raw := strings.TrimSpace(os.Getenv("DAEMON_PORT")); raw != "" {
			port, err := strconv.Atoi(raw)
			if err != nil || port < 1 || port > 65535 {
				return 0, errors.New("invalid DAEMON_PORT")
			}
			return port, nil
		}
		return 8081, nil
	}
	path, err := daemonProfileStatePath(profile, "port")
	if err != nil {
		return 0, err
	}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		port, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
		if parseErr == nil && port > 0 && port <= 65535 {
			return port, nil
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate Daemon profile port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if err := os.WriteFile(path, []byte(strconv.Itoa(port)+"\n"), 0o600); err != nil {
		return 0, err
	}
	return port, nil
}
