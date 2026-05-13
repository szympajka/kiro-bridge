package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicNonStream(t *testing.T) {
	mock := &mockBridge{chunks: []string{"Hello", " world"}}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp AnthropicMessageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Type != "message" {
		t.Errorf("type = %q, want %q", resp.Type, "message")
	}
	if resp.Role != "assistant" {
		t.Errorf("role = %q, want %q", resp.Role, "assistant")
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].Text != "Hello world" {
		t.Errorf("content = %q, want %q", resp.Content[0].Text, "Hello world")
	}
	if resp.StopReason == nil || *resp.StopReason != "end_turn" {
		t.Errorf("stop_reason = %v, want %q", resp.StopReason, "end_turn")
	}
	if resp.StopSequence != nil {
		t.Errorf("stop_sequence = %v, want nil", resp.StopSequence)
	}
	if mock.gotText != "hi" {
		t.Errorf("prompt text = %q, want %q", mock.gotText, "hi")
	}
}

func TestAnthropicNonStreamStopReasonNull(t *testing.T) {
	mock := &mockBridge{chunks: []string{"ok"}}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	raw := w.Body.String()
	if !strings.Contains(raw, `"stop_reason"`) {
		t.Errorf("response should contain stop_reason field: %s", raw)
	}
	if !strings.Contains(raw, `"stop_sequence":null`) {
		t.Errorf("response should contain stop_sequence:null: %s", raw)
	}
}

func TestAnthropicStream(t *testing.T) {
	mock := &mockBridge{chunks: []string{"Hello", " world"}}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"stream":true,"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want %q", ct, "text/event-stream")
	}

	type sseEvent struct {
		event string
		data  string
	}
	var events []sseEvent
	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			events = append(events, sseEvent{event: currentEvent, data: strings.TrimPrefix(line, "data: ")})
		}
	}

	expectedEvents := []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	if len(events) != len(expectedEvents) {
		t.Fatalf("events = %d, want %d; got:\n%s", len(events), len(expectedEvents), w.Body.String())
	}
	for i, want := range expectedEvents {
		if events[i].event != want {
			t.Errorf("event[%d] = %q, want %q", i, events[i].event, want)
		}
	}

	var msgStart AnthropicMessageStartEvent
	json.Unmarshal([]byte(events[0].data), &msgStart)
	if msgStart.Message.StopReason != nil {
		t.Errorf("message_start.stop_reason should be null, got %v", msgStart.Message.StopReason)
	}
	if msgStart.Message.StopSequence != nil {
		t.Errorf("message_start.stop_sequence should be null, got %v", msgStart.Message.StopSequence)
	}
	if msgStart.Message.Role != "assistant" {
		t.Errorf("message_start.role = %q, want %q", msgStart.Message.Role, "assistant")
	}

	var delta1 AnthropicContentBlockDeltaEvent
	json.Unmarshal([]byte(events[2].data), &delta1)
	if delta1.Delta.Text != "Hello" {
		t.Errorf("delta[0].text = %q, want %q", delta1.Delta.Text, "Hello")
	}

	var delta2 AnthropicContentBlockDeltaEvent
	json.Unmarshal([]byte(events[3].data), &delta2)
	if delta2.Delta.Text != " world" {
		t.Errorf("delta[1].text = %q, want %q", delta2.Delta.Text, " world")
	}

	var msgDelta AnthropicMessageDeltaEvent
	json.Unmarshal([]byte(events[5].data), &msgDelta)
	if msgDelta.Delta.StopReason != "end_turn" {
		t.Errorf("message_delta.stop_reason = %q, want %q", msgDelta.Delta.StopReason, "end_turn")
	}
	if msgDelta.Delta.StopSequence != nil {
		t.Errorf("message_delta.stop_sequence should be null, got %v", msgDelta.Delta.StopSequence)
	}
}

func TestAnthropicStreamError(t *testing.T) {
	mock := &mockBridge{promptErr: io.EOF}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"stream":true,"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestAnthropicStreamErrorAfterChunks(t *testing.T) {
	mock := &mockBridge{
		chunks:               []string{"partial"},
		promptErr:            io.EOF,
		promptErrAfterChunks: true,
	}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"stream":true,"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	if !strings.Contains(w.Body.String(), "error") {
		t.Errorf("expected error event in stream: %s", w.Body.String())
	}
}

func TestAnthropicEmptyMessages(t *testing.T) {
	mock := &mockBridge{}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","messages":[],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAnthropicWrongMethod(t *testing.T) {
	mock := &mockBridge{}
	handler := handleAnthropicMessages(mock)

	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestAnthropicInvalidJSON(t *testing.T) {
	mock := &mockBridge{}
	handler := handleAnthropicMessages(mock)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{not json`))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAnthropicSystemMessage(t *testing.T) {
	mock := &mockBridge{chunks: []string{"ok"}}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","system":"Be helpful.","messages":[{"role":"user","content":"hi"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(mock.gotText, "System: Be helpful.") {
		t.Errorf("prompt should contain system message, got: %q", mock.gotText)
	}
}

func TestAnthropicContentArray(t *testing.T) {
	mock := &mockBridge{chunks: []string{"ok"}}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":" world"}]}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(mock.gotText, "hello world") {
		t.Errorf("prompt should contain concatenated text, got: %q", mock.gotText)
	}
}

func TestAnthropicPromptError(t *testing.T) {
	mock := &mockBridge{promptErr: io.EOF}
	handler := handleAnthropicMessages(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"max_tokens":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestMapAnthropicStopReason(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"end_turn", "end_turn"},
		{"max_tokens", "max_tokens"},
		{"stop_sequence", "stop_sequence"},
		{"cancelled", "end_turn"},
		{"refusal", "end_turn"},
		{"", "end_turn"},
		{"unknown", "end_turn"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapAnthropicStopReason(tt.input)
			if got != tt.want {
				t.Errorf("mapAnthropicStopReason(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnthropicMessageContentUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    string
		wantErr bool
	}{
		{"string", `"hello"`, "hello", false},
		{"null", `null`, "", false},
		{"array with text", `[{"type":"text","text":"hi"}]`, "hi", false},
		{"array with multiple text", `[{"type":"text","text":"a"},{"type":"text","text":"b"}]`, "ab", false},
		{"array with non-text", `[{"type":"image","source":{}}]`, "", false},
		{"invalid", `123`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c AnthropicMessageContent
			err := json.Unmarshal([]byte(tt.json), &c)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.Text != tt.want {
				t.Errorf("got %q, want %q", c.Text, tt.want)
			}
		})
	}
}

func TestNewMessageIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newMessageID()
		if seen[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		seen[id] = true
	}
	id := newMessageID()
	if !strings.HasPrefix(id, "msg_") {
		t.Errorf("id should start with msg_, got: %s", id)
	}
}
