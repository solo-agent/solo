package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/config"
	"github.com/solo-ai/solo/pkg/version"
)

const (
	daemonProtocolVersion = 1
	maxMachineResponse    = 16 << 20
	maxRemoteEventData    = 480 << 10
)

var (
	errControlConnectionReplaced = errors.New("control connection replaced")
	errControlConnectionRejected = errors.New("control connection rejected")
)

type daemonCredential struct {
	ServerURL  string `json:"server_url"`
	ComputerID string `json:"computer_id"`
	Secret     string `json:"credential"`
}

type daemonControlEnvelope struct {
	Type            string          `json:"type"`
	RequestID       string          `json:"request_id,omitempty"`
	ProtocolVersion int             `json:"protocol_version"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

type daemonControlClient struct {
	serverURL  string
	daemonID   string
	computer   daemonCredential
	handler    *daemonHandler
	tasks      *taskManager
	httpClient *http.Client

	mu           sync.RWMutex
	connectionID string
	conn         *websocket.Conn
	writeMu      sync.Mutex
}

func newDaemonControlClient(cfg *config.Config, handler *daemonHandler, tasks *taskManager) (*daemonControlClient, error) {
	credential, _ := loadDaemonCredential()
	if cfg.ComputerID != "" {
		credential.ComputerID = cfg.ComputerID
	}
	if cfg.ComputerKey != "" {
		credential.Secret = cfg.ComputerKey
	}
	_, serverURLExplicit := os.LookupEnv("DAEMON_SERVER_URL")
	configuredServerURL := strings.TrimRight(cfg.ServerURL, "/")
	if serverURLExplicit && credential.ServerURL != "" && strings.TrimRight(credential.ServerURL, "/") != configuredServerURL && cfg.ComputerKey == "" && cfg.EnrollToken == "" {
		credential = daemonCredential{ServerURL: configuredServerURL}
	}
	if credential.ServerURL == "" || serverURLExplicit {
		credential.ServerURL = configuredServerURL
	}
	client := &daemonControlClient{
		serverURL:  credential.ServerURL,
		daemonID:   cfg.DaemonID,
		computer:   credential,
		handler:    handler,
		tasks:      tasks,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	if cfg.EnrollToken != "" {
		if client.computer.ComputerID == "" {
			return nil, errors.New("SOLO_COMPUTER_ID is required with SOLO_ENROLLMENT_TOKEN")
		}
		if err := client.enroll(context.Background(), cfg.EnrollToken); err != nil {
			return nil, err
		}
	}
	if client.computer.ComputerID == "" || client.computer.Secret == "" {
		return nil, errors.New("Computer is not paired; set SOLO_COMPUTER_ID and SOLO_ENROLLMENT_TOKEN")
	}
	return client, nil
}

func daemonCredentialPath() string {
	if path := strings.TrimSpace(os.Getenv("SOLO_DAEMON_CREDENTIAL_FILE")); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".solo", "daemon", "credentials.json")
}

func loadDaemonCredential() (daemonCredential, error) {
	var credential daemonCredential
	raw, err := os.ReadFile(daemonCredentialPath())
	if err != nil {
		return credential, err
	}
	if err := json.Unmarshal(raw, &credential); err != nil {
		return credential, fmt.Errorf("decode daemon credential: %w", err)
	}
	return credential, nil
}

func saveDaemonCredential(credential daemonCredential) error {
	path := daemonCredentialPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// A caller-provided file may intentionally live under a shared parent such
	// as /tmp. The credential file itself remains private; only harden the
	// default directory that Solo owns.
	if os.Getenv("SOLO_DAEMON_CREDENTIAL_FILE") == "" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(credential, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (client *daemonControlClient) enroll(ctx context.Context, token string) error {
	payload, _ := json.Marshal(map[string]string{
		"computer_id": client.computer.ComputerID, "enrollment_token": token,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.serverURL+"/internal/v1/daemon/enroll", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("enroll Computer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("enroll Computer: server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		ComputerID string `json:"computer_id"`
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Credential == "" {
		return errors.New("enroll Computer: invalid server response")
	}
	client.computer = daemonCredential{ServerURL: client.serverURL, ComputerID: result.ComputerID, Secret: result.Credential}
	if err := saveDaemonCredential(client.computer); err != nil {
		return fmt.Errorf("persist Computer credential: %w", err)
	}
	return nil
}

func (client *daemonControlClient) Run(ctx context.Context) error {
	backoff := time.Second
	for ctx.Err() == nil {
		err := client.connect(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if !shouldReconnectControl(err) {
			return err
		}
		slog.Warn("daemon control disconnected", "error", err, "retry_in", backoff)
		timer := time.NewTimer(backoff + time.Duration(rand.IntN(500))*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
	return nil
}

func (client *daemonControlClient) connect(ctx context.Context) error {
	controlURL, err := url.Parse(client.serverURL)
	if err != nil {
		return err
	}
	switch controlURL.Scheme {
	case "http":
		controlURL.Scheme = "ws"
	case "https":
		controlURL.Scheme = "wss"
	default:
		return fmt.Errorf("unsupported server URL scheme %q", controlURL.Scheme)
	}
	controlURL.Path = "/internal/v1/daemon/connect"
	controlURL.RawQuery = ""
	header := http.Header{}
	header.Set("Authorization", "Computer "+client.computer.Secret)
	header.Set("X-Solo-Computer-ID", client.computer.ComputerID)
	header.Set("X-Solo-Protocol-Version", fmt.Sprint(daemonProtocolVersion))
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, controlURL.String(), header)
	if err != nil {
		if resp != nil {
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return fmt.Errorf("%w: server returned %d", errControlConnectionRejected, resp.StatusCode)
			}
			return fmt.Errorf("connect control socket: server returned %d", resp.StatusCode)
		}
		return fmt.Errorf("connect control socket: %w", err)
	}
	defer conn.Close()
	client.mu.Lock()
	client.conn = conn
	client.connectionID = ""
	client.mu.Unlock()
	defer func() {
		client.mu.Lock()
		if client.conn == conn {
			client.conn = nil
			client.connectionID = ""
		}
		client.mu.Unlock()
	}()

	hello := map[string]any{
		"daemon_id":         client.daemonID,
		"daemon_version":    version.Version,
		"runtime_inventory": agent.GlobalRegistry().Detect(),
		"system_info":       collectSystemInfo(),
		"agent_ids":         client.handler.cachedSessionAgentIDs(),
		"active_attempts":   client.tasks.ActiveAttempts(),
	}
	if err := client.write(conn, daemonControlEnvelope{Type: "hello", ProtocolVersion: daemonProtocolVersion, Payload: controlJSON(hello)}); err != nil {
		return err
	}
	var ready daemonControlEnvelope
	if err := conn.ReadJSON(&ready); err != nil {
		return err
	}
	if ready.Type != "ready" || ready.ProtocolVersion != daemonProtocolVersion {
		return fmt.Errorf("unexpected control handshake %q", ready.Type)
	}
	var readyPayload struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.Unmarshal(ready.Payload, &readyPayload); err != nil || readyPayload.ConnectionID == "" {
		return errors.New("invalid control ready payload")
	}
	client.mu.Lock()
	client.connectionID = readyPayload.ConnectionID
	client.mu.Unlock()
	controlReady.Store(true)
	defer controlReady.Store(false)
	slog.Info("daemon control ready", "computer_id", client.computer.ComputerID, "connection_id", readyPayload.ConnectionID)
	go client.pollPendingRuns(ctx)

	heartbeatDone := make(chan struct{})
	go client.heartbeatLoop(ctx, conn, heartbeatDone)
	defer close(heartbeatDone)
	for {
		var envelope daemonControlEnvelope
		if err := conn.ReadJSON(&envelope); err != nil {
			return err
		}
		if envelope.ProtocolVersion != daemonProtocolVersion {
			return errors.New("server changed control protocol during connection")
		}
		switch envelope.Type {
		case "run.available":
			var payload struct {
				RunID string `json:"run_id"`
			}
			if json.Unmarshal(envelope.Payload, &payload) == nil && payload.RunID != "" {
				go client.handleRunAvailable(ctx, payload.RunID)
			}
		case "rpc.request":
			go client.handleRPC(ctx, conn, envelope)
		case "connection.replaced":
			return errControlConnectionReplaced
		case "heartbeat.ack":
		default:
			slog.Debug("ignoring unknown server control frame", "type", envelope.Type)
		}
	}
}

func shouldReconnectControl(err error) bool {
	if errors.Is(err, errControlConnectionReplaced) || errors.Is(err, errControlConnectionRejected) {
		return false
	}
	var closeErr *websocket.CloseError
	return !errors.As(err, &closeErr) || closeErr.Text != "connection replaced"
}

func (client *daemonControlClient) heartbeatLoop(ctx context.Context, conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			payload := map[string]any{
				"agent_ids":         client.handler.cachedSessionAgentIDs(),
				"runtime_inventory": agent.GlobalRegistry().Detect(),
				"active_attempts":   client.tasks.ActiveAttempts(),
			}
			if err := client.write(conn, daemonControlEnvelope{Type: "heartbeat", RequestID: uuid.NewString(), ProtocolVersion: daemonProtocolVersion, Payload: controlJSON(payload)}); err != nil {
				_ = conn.Close()
				return
			}
			go client.pollPendingRuns(ctx)
		case <-ctx.Done():
			return
		case <-done:
			return
		}
	}
}

func (client *daemonControlClient) pollPendingRuns(ctx context.Context) {
	body, err := client.machineRequest(ctx, http.MethodGet, "/internal/v1/daemon/runs/pending", nil)
	if err != nil {
		slog.Debug("poll pending remote Runs failed", "error", err)
		return
	}
	var response struct {
		RunIDs []string `json:"run_ids"`
	}
	if json.Unmarshal(body, &response) != nil {
		return
	}
	for _, runID := range response.RunIDs {
		go client.handleRunAvailable(ctx, runID)
	}
}

func (client *daemonControlClient) write(conn *websocket.Conn, envelope daemonControlEnvelope) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(envelope)
}

func (client *daemonControlClient) handleRunAvailable(ctx context.Context, runID string) {
	connectionID := client.ConnectionID()
	if connectionID == "" {
		return
	}
	payload, _ := json.Marshal(map[string]string{"connection_id": connectionID})
	body, err := client.machineRequest(ctx, http.MethodPost, "/internal/v1/daemon/runs/"+url.PathEscape(runID)+"/accept", payload)
	if err != nil {
		slog.Warn("accept remote Run failed", "run_id", runID, "error", err)
		return
	}
	var delivery struct {
		RunID      string          `json:"run_id"`
		TaskID     string          `json:"task_id"`
		AttemptID  string          `json:"execution_attempt_id"`
		Payload    json.RawMessage `json:"payload"`
		AgentToken string          `json:"agent_token"`
	}
	if err := json.Unmarshal(body, &delivery); err != nil {
		slog.Warn("decode remote Run failed", "run_id", runID, "error", err)
		return
	}
	var task runTaskRequest
	if err := json.Unmarshal(delivery.Payload, &task); err != nil {
		slog.Warn("decode remote Run payload failed", "run_id", runID, "error", err)
		return
	}
	task.AgentToken = delivery.AgentToken
	if err := client.handler.startTask(task, delivery.AttemptID); err != nil {
		slog.Warn("start remote Run failed", "run_id", runID, "error", err)
		return
	}
	if client.tasks.BeginForward(delivery.TaskID, delivery.AttemptID) {
		go client.forwardTaskEvents(ctx, delivery.RunID, delivery.TaskID, delivery.AttemptID)
	}
}

func (client *daemonControlClient) forwardTaskEvents(ctx context.Context, runID, taskID, attemptID string) {
	delivered := false
	defer func() { client.tasks.EndForward(taskID, attemptID, delivered) }()
	var deliveredSeq int64
	for ctx.Err() == nil {
		events := client.tasks.EventsAfter(taskID, deliveredSeq)
		if len(events) == 0 {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		for _, event := range events {
			for ctx.Err() == nil {
				connectionID := client.ConnectionID()
				if connectionID == "" {
					time.Sleep(time.Second)
					continue
				}
				data := json.RawMessage(event.Data)
				if !json.Valid(data) {
					data, _ = json.Marshal(map[string]string{"value": event.Data})
				}
				if len(data) > maxRemoteEventData {
					data, _ = json.Marshal(map[string]any{"truncated": true, "original_bytes": len(data)})
				}
				payload, _ := json.Marshal(map[string]any{
					"task_id": taskID, "execution_attempt_id": attemptID, "connection_id": connectionID,
					"source_seq": event.Seq, "event": event.Event, "data": data,
				})
				_, err := client.machineRequest(ctx, http.MethodPost, "/internal/v1/daemon/runs/"+url.PathEscape(runID)+"/events", payload)
				if err == nil {
					deliveredSeq = event.Seq
					if event.Event == "done" {
						delivered = true
						return
					}
					break
				}
				slog.Warn("upload remote Run event failed", "run_id", runID, "source_seq", event.Seq, "error", err)
				time.Sleep(time.Second)
			}
		}
	}
}

func (client *daemonControlClient) machineRequest(ctx context.Context, method, path string, payload []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, client.serverURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Computer "+client.computer.Secret)
	request.Header.Set("X-Solo-Computer-ID", client.computer.ComputerID)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxMachineResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxMachineResponse {
		return nil, errors.New("server response exceeds 16 MiB")
	}
	if response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("server returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (client *daemonControlClient) handleRPC(ctx context.Context, conn *websocket.Conn, request daemonControlEnvelope) {
	var call struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(request.Payload, &call); err != nil {
		_ = client.write(conn, daemonControlEnvelope{Type: "rpc.response", RequestID: request.RequestID, ProtocolVersion: daemonProtocolVersion, Payload: controlJSON(map[string]string{"error": "invalid RPC request"})})
		return
	}
	payload, err := client.handler.handleControlRPC(ctx, call.Method, call.Params)
	response := map[string]any{"result": json.RawMessage(payload)}
	if err != nil {
		response = map[string]any{"error": err.Error()}
	}
	_ = client.write(conn, daemonControlEnvelope{Type: "rpc.response", RequestID: request.RequestID, ProtocolVersion: daemonProtocolVersion, Payload: controlJSON(response)})
}

func (client *daemonControlClient) ConnectionID() string {
	client.mu.RLock()
	defer client.mu.RUnlock()
	return client.connectionID
}

func controlJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
