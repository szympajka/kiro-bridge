package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewRequest(t *testing.T) {
	data, err := newRequest(1, "initialize", InitializeParams{
		ProtocolVersion: 1,
		ClientInfo:      ClientInfo{Name: "kiro-bridge", Title: "Kiro Bridge", Version: "0.1.0"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want %q", req.JSONRPC, "2.0")
	}
	if req.ID != 1 {
		t.Errorf("id = %d, want 1", req.ID)
	}
	if req.Method != "initialize" {
		t.Errorf("method = %q, want %q", req.Method, "initialize")
	}

	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.ProtocolVersion != 1 {
		t.Errorf("protocolVersion = %d, want 1", params.ProtocolVersion)
	}
	if params.ClientInfo.Name != "kiro-bridge" {
		t.Errorf("clientInfo.name = %q, want %q", params.ClientInfo.Name, "kiro-bridge")
	}
}

func TestSessionUpdateParsing(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantUpdate string
		wantText   string
	}{
		{
			name:       "agent message chunk",
			input:      `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hello"}}`,
			wantUpdate: "agent_message_chunk",
			wantText:   "hello",
		},
		{
			name:       "tool call",
			input:      `{"sessionUpdate":"tool_call","toolCallId":"call_1","status":"pending"}`,
			wantUpdate: "tool_call",
		},
		{
			name:       "tool call update completed",
			input:      `{"sessionUpdate":"tool_call_update","toolCallId":"call_1","status":"completed"}`,
			wantUpdate: "tool_call_update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var su SessionUpdate
			if err := json.Unmarshal([]byte(tt.input), &su); err != nil {
				t.Fatal(err)
			}
			if su.SessionUpdate != tt.wantUpdate {
				t.Errorf("sessionUpdate = %q, want %q", su.SessionUpdate, tt.wantUpdate)
			}
			if tt.wantText != "" && su.Content.Text != tt.wantText {
				t.Errorf("content.text = %q, want %q", su.Content.Text, tt.wantText)
			}
		})
	}
}

func TestIsResponse(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "response with id",
			line: `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}`,
			want: true,
		},
		{
			name: "notification without id",
			line: `{"jsonrpc":"2.0","method":"session/update","params":{}}`,
			want: false,
		},
		{
			name: "error response with id",
			line: `{"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":"fail"}}`,
			want: true,
		},
		{
			name: "invalid json",
			line: `not json`,
			want: false,
		},
		{
			name: "empty object",
			line: `{}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isResponse([]byte(tt.line))
			if got != tt.want {
				t.Errorf("isResponse(%s) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestSessionNewRequestIncludesMcpServers(t *testing.T) {
	data, err := newRequest(1, "session/new", NewSessionNewParams("/tmp"))
	if err != nil {
		t.Fatal(err)
	}

	// Must contain "mcpServers":[] not "mcpServers":null
	if !strings.Contains(string(data), `"mcpServers":[]`) {
		t.Errorf("expected mcpServers:[], got: %s", data)
	}
}
