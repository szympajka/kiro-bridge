package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNewBridgeFailsWhenSetModeFails(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=set-mode-error",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{
		CLIPath: "kiro-cli",
		CWD:     ".",
		Agent:   "kiro-bridge",
		Version: "test",
	})
	if err == nil {
		if b != nil {
			b.Close()
		}
		t.Fatal("expected NewBridge to fail")
	}
	if !strings.Contains(err.Error(), "set mode") {
		t.Fatalf("error = %q, want set mode context", err)
	}
}

func TestBridgeHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	mode := os.Getenv("KIRO_BRIDGE_HELPER_MODE")
	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	sessionCount := 0

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		var resp any
		switch req.Method {
		case "initialize":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": 1,
				},
			}
		case "session/new":
			sessionCount++
			if mode == "with-models" {
				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result": map[string]any{
						"sessionId": "sess-1",
						"models": map[string]any{
							"currentModelId": "claude-sonnet-4",
							"availableModels": []map[string]any{
								{"modelId": "auto", "name": "auto", "description": "Auto"},
								{"modelId": "claude-sonnet-4", "name": "claude-sonnet-4", "description": "Sonnet 4"},
							},
						},
					},
				}
				break
			}
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"sessionId": "sess-1",
				},
			}
		case "session/set_mode":
			if mode == "set-mode-error" {
				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error": map[string]any{
						"code":    -32000,
						"message": "mode activation failed",
					},
				}
				break
			}
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{},
			}
		case "session/prompt":
			if mode == "error-then-ok" {
				if sessionCount <= 1 {
					resp = map[string]any{
						"jsonrpc": "2.0",
						"id":      req.ID,
						"error":   map[string]any{"code": -32603, "message": "Internal error"},
					}
					break
				}
				chunk := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": "ok"},
						},
					},
				}
				d, _ := json.Marshal(chunk)
				writer.Write(append(d, '\n'))
				writer.Flush()
				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  map[string]any{"stopReason": "end_turn"},
				}
				break
			}
			if mode == "slow-prompt" {
				// Send first chunk
				chunk := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": "working..."},
						},
					},
				}
				d, _ := json.Marshal(chunk)
				writer.Write(append(d, '\n'))
				writer.Flush()

				// Wait for cancel notification from bridge
				if scanner.Scan() {
					// Got cancel — respond with cancelled stop reason
					resp = map[string]any{
						"jsonrpc": "2.0",
						"id":      req.ID,
						"result":  map[string]any{"stopReason": "cancelled"},
					}
				}
				break
			}
			if mode == "unknown-method" {
				// Send an unknown agent→client request
				unknownReq := map[string]any{
					"jsonrpc": "2.0",
					"id":      "unknown-1",
					"method":  "fs/read_text_file",
					"params":  map[string]any{"path": "/tmp/test.txt"},
				}
				d, _ := json.Marshal(unknownReq)
				writer.Write(append(d, '\n'))
				writer.Flush()

				// Wait for bridge to respond
				if !scanner.Scan() {
					os.Exit(2)
				}

				// Send text chunk and prompt response
				chunk := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": "ok"},
						},
					},
				}
				d, _ = json.Marshal(chunk)
				writer.Write(append(d, '\n'))
				writer.Flush()

				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  map[string]any{"stopReason": "end_turn"},
				}
				break
			}
			if mode == "tool-call-with-meta" {
				toolCall := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "tool_call",
							"toolCallId":    "call_meta",
							"title":         "Finding *.go",
							"kind":          "search",
							"_meta":         map[string]any{"tool_name": "glob"},
						},
					},
				}
				d, _ := json.Marshal(toolCall)
				writer.Write(append(d, '\n'))

				chunk := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": "done"},
						},
					},
				}
				d, _ = json.Marshal(chunk)
				writer.Write(append(d, '\n'))
				writer.Flush()

				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  map[string]any{"stopReason": "end_turn"},
				}
				break
			}
			if mode == "tool-call" {
				// Send tool_call notification
				toolCall := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "tool_call",
							"toolCallId":    "call_abc",
							"title":         "Reading main.go",
							"kind":          "read",
							"rawInput":      map[string]any{"path": "main.go"},
						},
					},
				}
				d, _ := json.Marshal(toolCall)
				writer.Write(append(d, '\n'))

				// Send tool_call_update notification
				toolUpdate := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "tool_call_update",
							"toolCallId":    "call_abc",
							"status":        "completed",
						},
					},
				}
				d, _ = json.Marshal(toolUpdate)
				writer.Write(append(d, '\n'))

				// Send text chunk
				chunk := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": "file contents here"},
						},
					},
				}
				d, _ = json.Marshal(chunk)
				writer.Write(append(d, '\n'))
				writer.Flush()

				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  map[string]any{"stopReason": "end_turn"},
				}
				break
			}
			if mode == "permission-request" {
				// Send a permission request (agent→client) before responding
				permReq := map[string]any{
					"jsonrpc": "2.0",
					"id":      "perm-uuid-001",
					"method":  "session/request_permission",
					"params": map[string]any{
						"sessionId": "sess-1",
						"toolCall":  map[string]any{"toolCallId": "call_1", "title": "Writing file"},
						"options": []map[string]any{
							{"optionId": "allow_once", "name": "Yes", "kind": "allow_once"},
							{"optionId": "reject_once", "name": "No", "kind": "reject_once"},
						},
					},
				}
				permData, _ := json.Marshal(permReq)
				writer.Write(append(permData, '\n'))
				writer.Flush()

				// Wait for the bridge to respond to the permission request
				if !scanner.Scan() {
					os.Exit(2)
				}

				// Now send a message chunk and the prompt response
				chunk := map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "sess-1",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": "done"},
						},
					},
				}
				chunkData, _ := json.Marshal(chunk)
				writer.Write(append(chunkData, '\n'))
				writer.Flush()

				resp = map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  map[string]any{"stopReason": "end_turn"},
				}
				break
			}
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"stopReason": "end_turn"},
			}
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{},
			}
		}

		data, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := writer.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	os.Exit(0)
}

func TestNewBridgeSetModeErrorIncludesCode(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=set-mode-error",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	_, err := NewBridge(BridgeConfig{
		CLIPath: "kiro-cli",
		CWD:     ".",
		Agent:   "kiro-bridge",
		Version: "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "code -32000") {
		t.Fatalf("error = %q, want error code -32000", err)
	}
	if !strings.Contains(err.Error(), "mode activation failed") {
		t.Fatalf("error = %q, want message", err)
	}
}

func TestBridgeRejectsPermissionRequest(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=permission-request",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{
		CLIPath: "kiro-cli",
		CWD:     ".",
		Agent:   "kiro-bridge",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer b.Close()

	var chunks []string
	_, err = b.Prompt([]ContentBlock{{Type: "text", Text: "create a file"}}, func(ev PromptEvent) {
		if ev.Type == EventText {
			chunks = append(chunks, ev.Text)
		}
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	// Prompt should complete without deadlocking — the rejection unblocks the turn
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

func TestBridgeEmitsToolCallEvents(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=tool-call",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{
		CLIPath: "kiro-cli",
		CWD:     ".",
		Agent:   "kiro-bridge",
		Version: "test",
	})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer b.Close()

	var events []PromptEvent
	_, err = b.Prompt([]ContentBlock{{Type: "text", Text: "list files"}}, func(ev PromptEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Expect: tool_call event, tool_call_update event, text chunk
	var hasToolCall, hasToolUpdate, hasText bool
	for _, ev := range events {
		switch ev.Type {
		case EventToolCall:
			hasToolCall = true
			if ev.ToolCallID == "" {
				t.Error("tool_call event missing ToolCallID")
			}
			if ev.ToolName == "" {
				t.Error("tool_call event missing ToolName")
			}
		case EventToolCallUpdate:
			hasToolUpdate = true
			if ev.ToolCallID == "" {
				t.Error("tool_call_update event missing ToolCallID")
			}
		case EventText:
			hasText = true
			if ev.Text == "" {
				t.Error("text event missing Text")
			}
		}
	}
	if !hasToolCall {
		t.Error("missing tool_call event")
	}
	if !hasToolUpdate {
		t.Error("missing tool_call_update event")
	}
	if !hasText {
		t.Error("missing text event")
	}
}

func TestHandleModelsReturnsRealModels(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=with-models",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{CLIPath: "kiro-cli", CWD: ".", Agent: "kiro-bridge", Version: "test"})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer b.Close()

	models := b.Models()
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].ID != "auto" {
		t.Errorf("models[0].ID = %q, want %q", models[0].ID, "auto")
	}
	if models[1].ID != "claude-sonnet-4" {
		t.Errorf("models[1].ID = %q, want %q", models[1].ID, "claude-sonnet-4")
	}
}

func TestInitializeParamsIncludesCapabilities(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: 1,
		ClientCapabilities: ClientCapabilities{
			PromptCapabilities: &PromptCapabilities{Image: true},
		},
		ClientInfo: ClientInfo{Name: "kiro-bridge", Title: "Kiro Bridge", Version: "test"},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"image":true`) {
		t.Errorf("should declare image support: %s", s)
	}
	if !strings.Contains(s, `"promptCapabilities"`) {
		t.Errorf("should have promptCapabilities: %s", s)
	}
}

func TestBridgeExtractsToolNameFromMeta(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=tool-call-with-meta",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{CLIPath: "kiro-cli", CWD: ".", Agent: "kiro-bridge", Version: "test"})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer b.Close()

	var toolNames []string
	b.Prompt([]ContentBlock{{Type: "text", Text: "test"}}, func(ev PromptEvent) {
		if ev.Type == EventToolCall {
			toolNames = append(toolNames, ev.ToolName)
		}
	})
	if len(toolNames) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(toolNames))
	}
	if toolNames[0] != "glob" {
		t.Errorf("tool name = %q, want %q (from _meta.tool_name)", toolNames[0], "glob")
	}
}

func TestBridgeRespondsMethodNotFound(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=unknown-method",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{CLIPath: "kiro-cli", CWD: ".", Agent: "kiro-bridge", Version: "test"})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer b.Close()

	// Prompt should complete — the unknown method gets a -32601 response, unblocking the agent
	var chunks []string
	_, err = b.Prompt([]ContentBlock{{Type: "text", Text: "test"}}, func(ev PromptEvent) {
		if ev.Type == EventText {
			chunks = append(chunks, ev.Text)
		}
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

func TestBridgeSendsCancelNotification(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=slow-prompt",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{CLIPath: "kiro-cli", CWD: ".", Agent: "kiro-bridge", Version: "test"})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer b.Close()

	// Start prompt in goroutine, cancel after first chunk
	done := make(chan error, 1)
	go func() {
		_, err := b.Prompt([]ContentBlock{{Type: "text", Text: "slow task"}}, func(ev PromptEvent) {
			if ev.Type == EventText {
				// Cancel after receiving first chunk
				b.Cancel()
			}
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Prompt should complete without error after cancel, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Prompt did not complete after cancel")
	}
}

func TestBridgeReconnectsAfterRepeatedErrors(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestBridgeHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"KIRO_BRIDGE_HELPER_MODE=error-then-ok",
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	b, err := NewBridge(BridgeConfig{CLIPath: "kiro-cli", CWD: ".", Agent: "kiro-bridge", Version: "test"})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer b.Close()

	// First 3 prompts return errors
	for i := 0; i < 3; i++ {
		_, err = b.Prompt([]ContentBlock{{Type: "text", Text: "test"}}, func(ev PromptEvent) {})
		if err == nil {
			t.Fatalf("prompt %d should error", i)
		}
	}

	// 4th prompt should succeed — bridge recreated session after 3 consecutive errors
	_, err = b.Prompt([]ContentBlock{{Type: "text", Text: "test"}}, func(ev PromptEvent) {})
	if err != nil {
		t.Fatalf("prompt after reconnect should succeed, got: %v", err)
	}
}
