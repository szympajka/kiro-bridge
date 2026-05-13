package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var enableFS = os.Getenv("KIRO_BRIDGE_ENABLE_FS") != ""
var enableTerminal = os.Getenv("KIRO_BRIDGE_ENABLE_TERMINAL") != ""

type AgentBridgeConfig struct {
	EnableFS       bool `json:"enableFS"`
	EnableTerminal bool `json:"enableTerminal"`
}

type AgentConfig struct {
	Bridge *AgentBridgeConfig `json:"bridge,omitempty"`
}

func loadAgentConfig(agent string) {
	paths := []string{
		filepath.Join(os.Getenv("HOME"), ".kiro", "agents", agent+".json"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg AgentConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			debugf("debug: failed to parse agent config %s: %v", path, err)
			continue
		}
		if cfg.Bridge != nil {
			if cfg.Bridge.EnableFS {
				enableFS = true
			}
			if cfg.Bridge.EnableTerminal {
				enableTerminal = true
			}
		}
		debugf("debug: loaded bridge config from %s (enableFS=%v, enableTerminal=%v)", path, enableFS, enableTerminal)
		return
	}
}

// --- fs/read_text_file ---

type FSReadParams struct {
	Path string `json:"path"`
}

func handleFSRead(params json.RawMessage, cwd string) (json.RawMessage, *RPCError) {
	if !enableFS {
		return nil, &RPCError{Code: -32601, Message: "fs/read_text_file disabled (set KIRO_BRIDGE_ENABLE_FS=1)"}
	}

	var p FSReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if p.Path == "" {
		return nil, &RPCError{Code: -32602, Message: "path is required"}
	}

	path := resolvePath(p.Path, cwd)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("read failed: %v", err)}
	}

	result, _ := json.Marshal(map[string]any{
		"contents": string(data),
	})
	return result, nil
}

// --- fs/write_text_file ---

type FSWriteParams struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
}

func handleFSWrite(params json.RawMessage, cwd string) (json.RawMessage, *RPCError) {
	if !enableFS {
		return nil, &RPCError{Code: -32601, Message: "fs/write_text_file disabled (set KIRO_BRIDGE_ENABLE_FS=1)"}
	}

	var p FSWriteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if p.Path == "" {
		return nil, &RPCError{Code: -32602, Message: "path is required"}
	}

	path := resolvePath(p.Path, cwd)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("mkdir failed: %v", err)}
	}

	if err := os.WriteFile(path, []byte(p.Contents), 0644); err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("write failed: %v", err)}
	}

	result, _ := json.Marshal(map[string]any{
		"success": true,
	})
	return result, nil
}

// --- terminal/* ---

type terminalSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	output strings.Builder
	mu     sync.Mutex
	done   chan struct{}
	err    error
}

var (
	terminals   = make(map[string]*terminalSession)
	terminalsMu sync.Mutex
	terminalSeq int
)

type TerminalCreateParams struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Cwd     string   `json:"cwd"`
}

func handleTerminalCreate(params json.RawMessage, cwd string) (json.RawMessage, *RPCError) {
	if !enableTerminal {
		return nil, &RPCError{Code: -32601, Message: "terminal/create disabled (set KIRO_BRIDGE_ENABLE_TERMINAL=1)"}
	}

	var p TerminalCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if p.Command == "" {
		return nil, &RPCError{Code: -32602, Message: "command is required"}
	}

	dir := cwd
	if p.Cwd != "" {
		dir = resolvePath(p.Cwd, cwd)
	}

	cmd := exec.Command(p.Command, p.Args...)
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("stdin pipe: %v", err)}
	}

	sess := &terminalSession{
		cmd:   cmd,
		stdin: stdin,
		done:  make(chan struct{}),
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("stdout pipe: %v", err)}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("stderr pipe: %v", err)}
	}

	if err := cmd.Start(); err != nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("start failed: %v", err)}
	}

	go func() {
		combined := io.MultiReader(stdout, stderr)
		buf := make([]byte, 4096)
		for {
			n, err := combined.Read(buf)
			if n > 0 {
				sess.mu.Lock()
				sess.output.Write(buf[:n])
				sess.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		sess.err = cmd.Wait()
		close(sess.done)
	}()

	terminalsMu.Lock()
	terminalSeq++
	id := fmt.Sprintf("term-%d", terminalSeq)
	terminals[id] = sess
	terminalsMu.Unlock()

	result, _ := json.Marshal(map[string]any{
		"terminalId": id,
	})
	return result, nil
}

type TerminalIDParams struct {
	TerminalID string `json:"terminalId"`
}

func handleTerminalOutput(params json.RawMessage) (json.RawMessage, *RPCError) {
	if !enableTerminal {
		return nil, &RPCError{Code: -32601, Message: "terminal/output disabled (set KIRO_BRIDGE_ENABLE_TERMINAL=1)"}
	}

	var p TerminalIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}

	sess := getTerminal(p.TerminalID)
	if sess == nil {
		return nil, &RPCError{Code: -32000, Message: "terminal not found: " + p.TerminalID}
	}

	sess.mu.Lock()
	out := sess.output.String()
	sess.output.Reset()
	sess.mu.Unlock()

	result, _ := json.Marshal(map[string]any{
		"output": out,
	})
	return result, nil
}

func handleTerminalWaitForExit(params json.RawMessage) (json.RawMessage, *RPCError) {
	if !enableTerminal {
		return nil, &RPCError{Code: -32601, Message: "terminal/wait_for_exit disabled (set KIRO_BRIDGE_ENABLE_TERMINAL=1)"}
	}

	var p TerminalIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}

	sess := getTerminal(p.TerminalID)
	if sess == nil {
		return nil, &RPCError{Code: -32000, Message: "terminal not found: " + p.TerminalID}
	}

	<-sess.done

	sess.mu.Lock()
	out := sess.output.String()
	sess.mu.Unlock()

	exitCode := 0
	if sess.err != nil {
		if exitErr, ok := sess.err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	result, _ := json.Marshal(map[string]any{
		"exitCode": exitCode,
		"output":   out,
	})
	return result, nil
}

func handleTerminalRelease(params json.RawMessage) (json.RawMessage, *RPCError) {
	if !enableTerminal {
		return nil, &RPCError{Code: -32601, Message: "terminal/release disabled (set KIRO_BRIDGE_ENABLE_TERMINAL=1)"}
	}

	var p TerminalIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}

	terminalsMu.Lock()
	sess, ok := terminals[p.TerminalID]
	if ok {
		delete(terminals, p.TerminalID)
	}
	terminalsMu.Unlock()

	if !ok {
		return nil, &RPCError{Code: -32000, Message: "terminal not found: " + p.TerminalID}
	}

	sess.stdin.Close()

	result, _ := json.Marshal(map[string]any{
		"success": true,
	})
	return result, nil
}

func handleTerminalKill(params json.RawMessage) (json.RawMessage, *RPCError) {
	if !enableTerminal {
		return nil, &RPCError{Code: -32601, Message: "terminal/kill disabled (set KIRO_BRIDGE_ENABLE_TERMINAL=1)"}
	}

	var p TerminalIDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}

	terminalsMu.Lock()
	sess, ok := terminals[p.TerminalID]
	if ok {
		delete(terminals, p.TerminalID)
	}
	terminalsMu.Unlock()

	if !ok {
		return nil, &RPCError{Code: -32000, Message: "terminal not found: " + p.TerminalID}
	}

	if sess.cmd.Process != nil {
		sess.cmd.Process.Kill()
	}

	result, _ := json.Marshal(map[string]any{
		"success": true,
	})
	return result, nil
}

// --- helpers ---

func resolvePath(path, cwd string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func getTerminal(id string) *terminalSession {
	terminalsMu.Lock()
	defer terminalsMu.Unlock()
	return terminals[id]
}
