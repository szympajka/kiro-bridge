package main

import (
	"encoding/json"
	"fmt"
)

// RPCID represents a JSON-RPC 2.0 id that can be a string or integer.
type RPCID struct {
	Str string
	Num int
}

func (id RPCID) MarshalJSON() ([]byte, error) {
	if id.Str != "" {
		return json.Marshal(id.Str)
	}
	return json.Marshal(id.Num)
}

func (id *RPCID) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &id.Str)
	}
	return json.Unmarshal(data, &id.Num)
}

func IntID(n int) RPCID  { return RPCID{Num: n} }
func StrID(s string) RPCID { return RPCID{Str: s} }

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      RPCID           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      RPCID           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type messageType int

const (
	messageResponse     messageType = iota
	messageRequest
	messageNotification
	messageUnknown
)

func (m messageType) String() string {
	switch m {
	case messageResponse:
		return "response"
	case messageRequest:
		return "request"
	case messageNotification:
		return "notification"
	default:
		return "unknown"
	}
}

// classifyMessage determines if a JSON-RPC message is a response, request, or notification.
// Response: has id + (result or error), no method
// Request: has id + method (expects a response)
// Notification: has method, no id
func classifyMessage(line []byte) messageType {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return messageUnknown
	}
	_, hasID := raw["id"]
	_, hasMethod := raw["method"]
	_, hasResult := raw["result"]
	_, hasError := raw["error"]

	if hasID && (hasResult || hasError) {
		return messageResponse
	}
	if hasID && hasMethod {
		return messageRequest
	}
	if hasMethod && !hasID {
		return messageNotification
	}
	return messageUnknown
}

func isResponse(line []byte) bool {
	return classifyMessage(line) == messageResponse
}

type RPCError struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    json.RawMessage  `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("code %d: %s (data: %s)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("code %d: %s", e.Code, e.Message)
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ACP param/result types

type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         ClientInfo         `json:"clientInfo"`
}

type ClientCapabilities struct {
	PromptCapabilities *PromptCapabilities `json:"promptCapabilities,omitempty"`
}

type PromptCapabilities struct {
	Image bool `json:"image"`
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
	SessionID string           `json:"sessionId"`
	Models    *SessionModels   `json:"models,omitempty"`
}

type SessionModels struct {
	CurrentModelID  string         `json:"currentModelId"`
	AvailableModels []SessionModel `json:"availableModels"`
}

type SessionModel struct {
	ModelID     string `json:"modelId"`
	Name        string `json:"name"`
	Description string `json:"description"`
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
	Title         string       `json:"title,omitempty"`
	Status        string       `json:"status,omitempty"`
}

func newRequest(id int, method string, params any) ([]byte, error) {
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Request{JSONRPC: "2.0", ID: IntID(id), Method: method, Params: p})
}

func newResponse(id RPCID, result json.RawMessage) ([]byte, error) {
	return json.Marshal(Response{JSONRPC: "2.0", ID: id, Result: result})
}
