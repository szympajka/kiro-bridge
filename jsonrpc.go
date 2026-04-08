package main

import "encoding/json"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func isResponse(line []byte) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return false
	}
	_, hasID := raw["id"]
	_, hasResult := raw["result"]
	_, hasError := raw["error"]
	return hasID && (hasResult || hasError)
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ACP param/result types

type InitializeParams struct {
	ProtocolVersion    int        `json:"protocolVersion"`
	ClientCapabilities struct{}   `json:"clientCapabilities"`
	ClientInfo         ClientInfo `json:"clientInfo"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion   int             `json:"protocolVersion"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities"`
}

type SessionNewParams struct {
	CWD        string `json:"cwd"`
	MCPServers []any  `json:"mcpServers"`
}

func NewSessionNewParams(cwd string) SessionNewParams {
	return SessionNewParams{CWD: cwd, MCPServers: []any{}}
}

type SessionNewResult struct {
	SessionID string `json:"sessionId"`
}

type SessionPromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type SessionPromptResult struct {
	StopReason string `json:"stopReason"`
}

type SessionSetModeParams struct {
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId"`
}

type SessionUpdateParams struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

type SessionUpdate struct {
	SessionUpdate string       `json:"sessionUpdate"`
	Content       ContentBlock `json:"content,omitempty"`
	ToolCallID    string       `json:"toolCallId,omitempty"`
	Status        string       `json:"status,omitempty"`
}

func newRequest(id int, method string, params any) ([]byte, error) {
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Request{JSONRPC: "2.0", ID: id, Method: method, Params: p})
}
