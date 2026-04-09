package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

var execCommand = exec.Command

// Bridge manages the kiro-cli ACP process and provides prompt functionality.
type Bridge interface {
	Prompt(text string, onChunk func(string)) (string, error)
	Close() error
}

type bridge struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	mu      sync.Mutex
	nextID  int
	sessID  string
	cwd     string
	cliPath string
	agent   string
	version string
}

type BridgeConfig struct {
	CLIPath string
	CWD     string
	Agent   string
	Version string
}

func NewBridge(cfg BridgeConfig) (Bridge, error) {
	b := &bridge{
		cwd:     cfg.CWD,
		cliPath: cfg.CLIPath,
		agent:   cfg.Agent,
		version: cfg.Version,
		nextID:  1,
	}
	if err := b.start(); err != nil {
		b.Close()
		return nil, err
	}
	return b, nil
}

func (b *bridge) start() error {
	b.cmd = execCommand(b.cliPath, "acp")
	b.cmd.Stderr = &debugWriter{prefix: "[kiro-cli] "}

	var err error
	b.stdin, err = b.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := b.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	b.scanner = bufio.NewScanner(stdout)
	b.scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("start kiro-cli: %w", err)
	}

	if err := b.initialize(); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	if err := b.newSession(); err != nil {
		return fmt.Errorf("new session: %w", err)
	}

	if b.agent != "" {
		if err := b.setMode(); err != nil {
			return fmt.Errorf("set mode: %w", err)
		}
	}

	return nil
}

func (b *bridge) send(id int, method string, params any) error {
	data, err := newRequest(id, method, params)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = b.stdin.Write(data)
	return err
}

func (b *bridge) readResponse() (*Response, error) {
	return b.readResponseWithCallback(nil)
}

// readResponseWithCallback reads until a response arrives, calling onNotif for each notification.
func (b *bridge) readResponseWithCallback(onNotif func(*Notification)) (*Response, error) {
	for b.scanner.Scan() {
		line := b.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if isResponse(line) {
			var resp Response
			if err := json.Unmarshal(line, &resp); err != nil {
				debugf("debug: failed to parse response: %s", line)
				continue
			}
			return &resp, nil
		}

		var notif Notification
		if err := json.Unmarshal(line, &notif); err == nil && notif.Method != "" {
			if onNotif != nil {
				onNotif(&notif)
			}
			continue
		}

		debugf("debug: unrecognized line from acp: %s", line)
	}
	if err := b.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (b *bridge) initialize() error {
	id := b.nextID
	b.nextID++
	debugf("debug: sending initialize (id=%d)", id)
	err := b.send(id, "initialize", InitializeParams{
		ProtocolVersion: 1,
		ClientInfo:      ClientInfo{Name: "kiro-bridge", Title: "Kiro Bridge", Version: b.version},
	})
	if err != nil {
		return err
	}
	resp, err := b.readResponse()
	if err != nil {
		return fmt.Errorf("reading initialize response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}
	debugf("debug: initialize ok")
	return nil
}

func (b *bridge) newSession() error {
	id := b.nextID
	b.nextID++
	debugf("debug: sending session/new (id=%d, cwd=%s)", id, b.cwd)
	err := b.send(id, "session/new", NewSessionNewParams(b.cwd))
	if err != nil {
		return err
	}
	resp, err := b.readResponse()
	if err != nil {
		return fmt.Errorf("reading session/new response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("session/new error: %s", resp.Error.Message)
	}
	var result SessionNewResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return err
	}
	b.sessID = result.SessionID
	debugf("debug: session created: %s", b.sessID)
	return nil
}

func (b *bridge) setMode() error {
	id := b.nextID
	b.nextID++
	err := b.send(id, "session/set_mode", SessionSetModeParams{
		SessionID: b.sessID,
		ModeID:    b.agent,
	})
	if err != nil {
		return err
	}
	resp, err := b.readResponse()
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("set_mode error: %s", resp.Error.Message)
	}
	return nil
}

func (b *bridge) Prompt(text string, onChunk func(string)) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++

	err := b.send(id, "session/prompt", SessionPromptParams{
		SessionID: b.sessID,
		Prompt:    []ContentBlock{{Type: "text", Text: text}},
	})
	if err != nil {
		return "", err
	}

	resp, err := b.readResponseWithCallback(func(notif *Notification) {
		if notif.Method != "session/update" {
			return
		}
		var params SessionUpdateParams
		if err := json.Unmarshal(notif.Params, &params); err != nil {
			debugf("debug: unmarshal session/update params: %v", err)
			return
		}
		var update SessionUpdate
		if err := json.Unmarshal(params.Update, &update); err != nil {
			debugf("debug: unmarshal session update: %v", err)
			return
		}
		if update.SessionUpdate == "agent_message_chunk" && update.Content.Text != "" {
			onChunk(update.Content.Text)
		}
	})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("prompt error: %s", resp.Error.Message)
	}

	var result SessionPromptResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return result.StopReason, nil
	}
	return result.StopReason, nil
}

func (b *bridge) Close() error {
	if b.stdin != nil {
		b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- b.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			b.cmd.Process.Kill()
			<-done
		}
	}
	return nil
}
