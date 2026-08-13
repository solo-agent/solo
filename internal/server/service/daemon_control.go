package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/solo-ai/solo/pkg/metrics"
)

const (
	controlWriteWait        = 10 * time.Second
	controlPongWait         = 75 * time.Second
	controlPingPeriod       = 30 * time.Second
	controlLeaseGracePeriod = 45 * time.Second
	controlMaxFrame         = 1 << 20
)

var (
	ErrComputerOffline       = errors.New("computer is offline")
	ErrStaleControlLease     = errors.New("stale computer control lease")
	ErrControlRPCUnavailable = errors.New("computer RPC unavailable")
)

type ControlEnvelope struct {
	Type            string          `json:"type"`
	RequestID       string          `json:"request_id,omitempty"`
	ProtocolVersion int             `json:"protocol_version"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

type DaemonHello struct {
	DaemonID         string             `json:"daemon_id"`
	DaemonVersion    string             `json:"daemon_version"`
	RuntimeInventory json.RawMessage    `json:"runtime_inventory"`
	SystemInfo       ComputerSystemInfo `json:"system_info"`
	AgentIDs         []string           `json:"agent_ids,omitempty"`
	ActiveAttempts   []ActiveRunAttempt `json:"active_attempts,omitempty"`
}

type ActiveRunAttempt struct {
	RunID     string `json:"run_id"`
	AttemptID string `json:"execution_attempt_id"`
}

type DaemonControlHeartbeat struct {
	AgentIDs         []string           `json:"agent_ids,omitempty"`
	RuntimeInventory json.RawMessage    `json:"runtime_inventory,omitempty"`
	ActiveAttempts   []ActiveRunAttempt `json:"active_attempts,omitempty"`
}

type controlLeaseGrace struct {
	ConnectionID string
	Until        time.Time
}

type DaemonControlConnection struct {
	ID         string
	ComputerID string
	DaemonID   string
	conn       *websocket.Conn
	send       chan ControlEnvelope
	done       chan struct{}
	closeOnce  sync.Once
}

func (dm *DaemonManager) ServeControlConnection(ctx context.Context, conn *websocket.Conn, computerID string, computers *ComputerService) error {
	conn.SetReadLimit(controlMaxFrame)
	_ = conn.SetReadDeadline(time.Now().Add(controlPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(controlPongWait))
	})

	var first ControlEnvelope
	if err := conn.ReadJSON(&first); err != nil {
		return fmt.Errorf("read daemon hello: %w", err)
	}
	if first.Type != "hello" || first.ProtocolVersion != ComputerProtocolVersion {
		_ = conn.WriteJSON(ControlEnvelope{
			Type:            "error",
			ProtocolVersion: ComputerProtocolVersion,
			Payload:         mustControlJSON(map[string]any{"code": "protocol_mismatch", "supported": []int{ComputerProtocolVersion}}),
		})
		return fmt.Errorf("unsupported daemon protocol version %d", first.ProtocolVersion)
	}
	var hello DaemonHello
	if err := json.Unmarshal(first.Payload, &hello); err != nil || hello.DaemonID == "" {
		return fmt.Errorf("invalid daemon hello")
	}
	if err := computers.MarkConnected(ctx, computerID, hello.DaemonID, hello.DaemonVersion, first.ProtocolVersion, hello.RuntimeInventory, hello.SystemInfo, hello.AgentIDs); err != nil {
		return err
	}

	control := &DaemonControlConnection{
		ID:         uuid.NewString(),
		ComputerID: computerID,
		DaemonID:   hello.DaemonID,
		conn:       conn,
		send:       make(chan ControlEnvelope, 64),
		done:       make(chan struct{}),
	}
	ready := ControlEnvelope{
		Type:            "ready",
		ProtocolVersion: ComputerProtocolVersion,
		Payload: mustControlJSON(map[string]any{
			"connection_id":     control.ID,
			"heartbeat_seconds": int(controlPingPeriod / time.Second),
			"server_time":       time.Now().UTC().Format(time.RFC3339),
		}),
	}
	_ = conn.SetWriteDeadline(time.Now().Add(controlWriteWait))
	if err := conn.WriteJSON(ready); err != nil {
		return fmt.Errorf("write daemon ready: %w", err)
	}

	old := dm.registerControlConnection(control)
	if old != nil {
		old.replace()
	}
	// MarkConnected happens before registration. Refresh after registration so
	// this new connection wins if the replaced connection disconnects between
	// those two steps and marks the Computer offline.
	if err := computers.UpdateRemoteHeartbeat(ctx, computerID, hello.AgentIDs, hello.RuntimeInventory); err != nil {
		control.close()
		dm.unregisterControlConnectionAndMarkOffline(control, computers)
		return err
	}
	slog.Info("daemon control ready", "computer_id", computerID, "daemon_id", hello.DaemonID, "connection_id", control.ID)
	metrics.Global.IncDaemonConnects()
	dm.Register(&DaemonInfo{
		ID: computerID, ComputerID: computerID, Version: hello.DaemonVersion,
		Capabilities: []string{"llm"}, MaxConcurrent: 10, AgentTypes: availableRuntimeTypes(hello.RuntimeInventory),
	})
	dm.Heartbeat(computerID, int32(len(hello.ActiveAttempts)))
	go dm.reconcileRemoteConnection(context.Background(), computerID, hello.ActiveAttempts)

	writeErr := make(chan error, 1)
	go func() { writeErr <- control.writePump() }()
	readErr := control.readPump(ctx, dm, computers)
	control.close()
	dm.unregisterControlConnectionAndMarkOffline(control, computers)
	select {
	case err := <-writeErr:
		if readErr == nil {
			readErr = err
		}
	case <-time.After(time.Second):
	}
	return readErr
}

func (dm *DaemonManager) registerControlConnection(control *DaemonControlConnection) *DaemonControlConnection {
	dm.controlMu.Lock()
	defer dm.controlMu.Unlock()
	old := dm.controlConnections[control.ComputerID]
	dm.controlConnections[control.ComputerID] = control
	metrics.Global.SetDaemonControls(int64(len(dm.controlConnections)))
	delete(dm.controlGrace, control.ComputerID)
	return old
}

func (dm *DaemonManager) unregisterControlConnection(control *DaemonControlConnection) bool {
	dm.controlMu.Lock()
	defer dm.controlMu.Unlock()
	return dm.unregisterControlConnectionLocked(control)
}

func (dm *DaemonManager) unregisterControlConnectionAndMarkOffline(control *DaemonControlConnection, computers *ComputerService) {
	dm.controlMu.Lock()
	defer dm.controlMu.Unlock()
	if !dm.unregisterControlConnectionLocked(control) {
		return
	}
	// Keep this update inside controlMu. A replacement connection may have
	// authenticated already, but it cannot register until this transition is
	// persisted; its post-registration heartbeat then restores online status.
	disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := computers.MarkControlDisconnected(disconnectCtx, control.ComputerID); err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("mark daemon control disconnected", "computer_id", control.ComputerID, "error", err)
	}
}

func (dm *DaemonManager) unregisterControlConnectionLocked(control *DaemonControlConnection) bool {
	if dm.controlConnections[control.ComputerID] != control {
		return false
	}
	delete(dm.controlConnections, control.ComputerID)
	metrics.Global.SetDaemonControls(int64(len(dm.controlConnections)))
	dm.controlGrace[control.ComputerID] = controlLeaseGrace{
		ConnectionID: control.ID,
		Until:        time.Now().Add(controlLeaseGracePeriod),
	}
	slog.Info("daemon control disconnected", "computer_id", control.ComputerID, "connection_id", control.ID)
	return true
}

func (dm *DaemonManager) AuthorizeControlLease(computerID, connectionID string) error {
	dm.controlMu.RLock()
	defer dm.controlMu.RUnlock()
	if current := dm.controlConnections[computerID]; current != nil && current.ID == connectionID {
		return nil
	}
	if grace, ok := dm.controlGrace[computerID]; ok && grace.ConnectionID == connectionID && time.Now().Before(grace.Until) {
		return nil
	}
	return ErrStaleControlLease
}

func (dm *DaemonManager) NotifyRun(computerID, runID string) bool {
	dm.controlMu.RLock()
	control := dm.controlConnections[computerID]
	dm.controlMu.RUnlock()
	if control == nil {
		return false
	}
	return control.enqueue(ControlEnvelope{
		Type:            "run.available",
		RequestID:       uuid.NewString(),
		ProtocolVersion: ComputerProtocolVersion,
		Payload:         mustControlJSON(map[string]string{"run_id": runID}),
	})
}

func (dm *DaemonManager) DisconnectComputer(computerID string) {
	dm.controlMu.Lock()
	control := dm.controlConnections[computerID]
	delete(dm.controlConnections, computerID)
	delete(dm.controlGrace, computerID)
	metrics.Global.SetDaemonControls(int64(len(dm.controlConnections)))
	dm.controlMu.Unlock()
	if control != nil {
		control.close()
	}
}

func (dm *DaemonManager) CallControlRPC(ctx context.Context, computerID, method string, payload any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, controlRPCTimeout(method))
	defer cancel()
	requestID := uuid.NewString()
	waiter := make(chan ControlEnvelope, 1)
	dm.controlMu.Lock()
	control := dm.controlConnections[computerID]
	if control == nil {
		dm.controlMu.Unlock()
		return nil, ErrComputerOffline
	}
	dm.rpcWaiters[requestID] = waiter
	dm.controlMu.Unlock()
	defer func() {
		dm.controlMu.Lock()
		delete(dm.rpcWaiters, requestID)
		dm.controlMu.Unlock()
	}()
	if !control.enqueue(ControlEnvelope{
		Type:            "rpc.request",
		RequestID:       requestID,
		ProtocolVersion: ComputerProtocolVersion,
		Payload:         mustControlJSON(map[string]any{"method": method, "params": payload}),
	}) {
		return nil, ErrControlRPCUnavailable
	}
	select {
	case response := <-waiter:
		var result struct {
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(response.Payload, &result); err != nil {
			return nil, err
		}
		if result.Error != "" {
			return nil, errors.New(result.Error)
		}
		return result.Result, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			metrics.Global.IncRemoteRPCTimeout()
		}
		return nil, ctx.Err()
	case <-control.done:
		return nil, ErrControlRPCUnavailable
	}
}

func controlRPCTimeout(method string) time.Duration {
	if method == "agent.cleanup" || method == "thinking.cleanup" {
		return 30 * time.Second
	}
	return 10 * time.Second
}

func (control *DaemonControlConnection) readPump(ctx context.Context, dm *DaemonManager, computers *ComputerService) error {
	for {
		var envelope ControlEnvelope
		if err := control.conn.ReadJSON(&envelope); err != nil {
			return err
		}
		if envelope.ProtocolVersion != ComputerProtocolVersion {
			return fmt.Errorf("daemon protocol changed during connection")
		}
		switch envelope.Type {
		case "heartbeat":
			var heartbeat DaemonControlHeartbeat
			if err := json.Unmarshal(envelope.Payload, &heartbeat); err != nil {
				return fmt.Errorf("invalid daemon heartbeat: %w", err)
			}
			if err := computers.UpdateRemoteHeartbeat(ctx, control.ComputerID, heartbeat.AgentIDs, heartbeat.RuntimeInventory); err != nil {
				return err
			}
			dm.Heartbeat(control.ComputerID, int32(len(heartbeat.ActiveAttempts)))
			_ = control.enqueue(ControlEnvelope{Type: "heartbeat.ack", RequestID: envelope.RequestID, ProtocolVersion: ComputerProtocolVersion})
		case "rpc.response":
			dm.controlMu.RLock()
			waiter := dm.rpcWaiters[envelope.RequestID]
			dm.controlMu.RUnlock()
			if waiter != nil {
				select {
				case waiter <- envelope:
				default:
				}
			}
		case "pong":
			_ = control.conn.SetReadDeadline(time.Now().Add(controlPongWait))
		default:
			slog.Debug("ignoring unknown daemon control frame", "computer_id", control.ComputerID, "type", envelope.Type)
		}
	}
}

func (dm *DaemonManager) IsCurrentControlConnection(computerID, connectionID string) bool {
	dm.controlMu.RLock()
	defer dm.controlMu.RUnlock()
	current := dm.controlConnections[computerID]
	return current != nil && current.ID == connectionID
}

func availableRuntimeTypes(inventory json.RawMessage) []string {
	var statuses []struct {
		Type      string `json:"type"`
		Available bool   `json:"available"`
	}
	_ = json.Unmarshal(inventory, &statuses)
	types := make([]string, 0, len(statuses))
	for _, status := range statuses {
		if status.Available && status.Type != "" {
			types = append(types, status.Type)
		}
	}
	return types
}

func (control *DaemonControlConnection) writePump() error {
	ticker := time.NewTicker(controlPingPeriod)
	defer ticker.Stop()
	for {
		select {
		case envelope := <-control.send:
			_ = control.conn.SetWriteDeadline(time.Now().Add(controlWriteWait))
			if err := control.conn.WriteJSON(envelope); err != nil {
				return err
			}
		case <-ticker.C:
			_ = control.conn.SetWriteDeadline(time.Now().Add(controlWriteWait))
			if err := control.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return err
			}
		case <-control.done:
			return nil
		}
	}
}

func (control *DaemonControlConnection) enqueue(envelope ControlEnvelope) bool {
	select {
	case <-control.done:
		return false
	default:
	}
	select {
	case control.send <- envelope:
		return true
	default:
		return false
	}
}

func (control *DaemonControlConnection) replace() {
	_ = control.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "connection replaced"),
		time.Now().Add(controlWriteWait),
	)
	control.close()
}

func (control *DaemonControlConnection) close() {
	control.closeOnce.Do(func() {
		close(control.done)
		_ = control.conn.Close()
	})
}

func (dm *DaemonManager) closeControlConnections() {
	dm.controlMu.Lock()
	connections := make([]*DaemonControlConnection, 0, len(dm.controlConnections))
	for _, control := range dm.controlConnections {
		connections = append(connections, control)
	}
	dm.controlConnections = make(map[string]*DaemonControlConnection)
	metrics.Global.SetDaemonControls(0)
	dm.controlGrace = make(map[string]controlLeaseGrace)
	dm.controlMu.Unlock()
	for _, control := range connections {
		control.close()
	}
}

func mustControlJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
