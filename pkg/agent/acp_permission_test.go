package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
)

func TestACPRequestPermissionSelectsProviderSessionOption(t *testing.T) {
	var stdin bytes.Buffer
	client := &acpClient{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		stdin:  &stdin,
	}

	client.handleLine(`{"jsonrpc":"2.0","id":7,"method":"session/request_permission","params":{"options":[{"optionId":"trae-deny","name":"Deny","kind":"reject_once"},{"optionId":"trae-once","name":"Allow once","kind":"allow_once"},{"optionId":"trae-session","name":"Allow for this session","kind":"allow_always"}]}}`)

	var response struct {
		Result struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdin.Bytes(), &response); err != nil {
		t.Fatalf("decode permission response: %v", err)
	}
	if response.Result.Outcome.Outcome != "selected" {
		t.Fatalf("outcome = %q, want selected", response.Result.Outcome.Outcome)
	}
	if response.Result.Outcome.OptionID != "trae-session" {
		t.Fatalf("optionId = %q, want trae-session", response.Result.Outcome.OptionID)
	}
}

func TestACPRequestPermissionFallsBackToAllowOnce(t *testing.T) {
	raw := json.RawMessage(`{"options":[{"optionId":"deny","kind":"reject_once"},{"optionId":"opaque-42","name":"Allow once","kind":"allow_once"}]}`)
	if got := selectACPPermissionOption(raw); got != "opaque-42" {
		t.Fatalf("selectACPPermissionOption() = %q, want opaque-42", got)
	}
}
