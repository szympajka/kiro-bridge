package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
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
		default:
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
