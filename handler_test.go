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

type mockBridge struct {
	chunks               []string
	gotText              string
	promptErr            error
	promptErrAfterChunks bool
}

func (m *mockBridge) Prompt(text string, onEvent func(PromptEvent)) (string, error) {
	m.gotText = text
	if m.promptErr != nil && !m.promptErrAfterChunks {
		return "", m.promptErr
	}
	for _, c := range m.chunks {
		onEvent(PromptEvent{Type: EventText, Text: c})
	}
	if m.promptErr != nil {
		return "", m.promptErr
	}
	return "end_turn", nil
}

func (m *mockBridge) Close() error { return nil }

func TestBuildPromptTextWithContentParts(t *testing.T) {
	body := `{"messages":[{"role":"system","content":"Be helpful."},{"role":"user","content":[{"type":"text","text":"hello"}]}],"model":"kiro","stream":true}`
	var req ChatCompletionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	got := buildPromptText(req.Messages)
	if !strings.Contains(got, "hello") {
		t.Errorf("expected 'hello' in prompt, got: %q", got)
	}
}

func TestBuildPromptText(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		want     string
	}{
		{
			name:     "user only",
			messages: []ChatMessage{{Role: "user", Content: ChatContent{Text: "hello"}}},
			want:     "hello",
		},
		{
			name: "system + user",
			messages: []ChatMessage{
				{Role: "system", Content: ChatContent{Text: "You are helpful."}},
				{Role: "user", Content: ChatContent{Text: "hello"}},
			},
			want: "System: You are helpful.\n\nhello",
		},
		{
			name: "ignores assistant messages",
			messages: []ChatMessage{
				{Role: "user", Content: ChatContent{Text: "hi"}},
				{Role: "assistant", Content: ChatContent{Text: "hey"}},
				{Role: "user", Content: ChatContent{Text: "bye"}},
			},
			want: "hi\n\nbye",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPromptText(tt.messages)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleNonStream(t *testing.T) {
	mock := &mockBridge{chunks: []string{"Hello", " world"}}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp ChatCompletionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("object = %q, want %q", resp.Object, "chat.completion")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d, want 1", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content.Text != "Hello world" {
		t.Errorf("content = %q, want %q", resp.Choices[0].Message.Content.Text, "Hello world")
	}
	if *resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want %q", *resp.Choices[0].FinishReason, "stop")
	}
	if mock.gotText != "hi" {
		t.Errorf("prompt text = %q, want %q", mock.gotText, "hi")
	}
}

func TestHandleStream(t *testing.T) {
	mock := &mockBridge{chunks: []string{"Hello", " world"}}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want %q", ct, "text/event-stream")
	}

	// Parse SSE events
	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}

	// Should have: chunk1 (role+content), chunk2 (content), final (finish_reason), [DONE]
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4; got:\n%s", len(events), w.Body.String())
	}

	// First chunk should have role
	var first ChatCompletionResponse
	json.Unmarshal([]byte(events[0]), &first)
	if first.Choices[0].Delta.Role != "assistant" {
		t.Errorf("first chunk role = %q, want %q", first.Choices[0].Delta.Role, "assistant")
	}
	if first.Choices[0].Delta.Content.Text != "Hello" {
		t.Errorf("first chunk content = %q, want %q", first.Choices[0].Delta.Content.Text, "Hello")
	}

	// Second chunk should have content only
	var second ChatCompletionResponse
	json.Unmarshal([]byte(events[1]), &second)
	if second.Choices[0].Delta.Content.Text != " world" {
		t.Errorf("second chunk content = %q, want %q", second.Choices[0].Delta.Content.Text, " world")
	}

	// Third chunk should have finish_reason
	var final ChatCompletionResponse
	json.Unmarshal([]byte(events[2]), &final)
	if *final.Choices[0].FinishReason != "stop" {
		t.Errorf("final finish_reason = %q, want %q", *final.Choices[0].FinishReason, "stop")
	}

	// Last should be [DONE]
	if events[3] != "[DONE]" {
		t.Errorf("last event = %q, want %q", events[3], "[DONE]")
	}
}

func TestHandleEmptyMessages(t *testing.T) {
	mock := &mockBridge{}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleWrongMethod(t *testing.T) {
	mock := &mockBridge{}
	handler := handleChatCompletions(mock)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestLoggingWriterImplementsFlusher(t *testing.T) {
	inner := httptest.NewRecorder()
	lw := &loggingWriter{ResponseWriter: inner, status: 200}

	// Must implement http.Flusher for SSE streaming
	var w http.ResponseWriter = lw
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("loggingWriter does not implement http.Flusher")
	}
	flusher.Flush() // should not panic
}

func TestStreamThroughMiddleware(t *testing.T) {
	mock := &mockBridge{chunks: []string{"hi"}}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions(mock))
	server := httptest.NewServer(logMiddleware(mux))
	defer server.Close()

	body := `{"model":"kiro","messages":[{"role":"user","content":"test"}],"stream":true}`
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want %q", ct, "text/event-stream")
	}
}

func TestChatContentUnmarshalEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    string
		wantErr bool
	}{
		{"null content", `null`, "", false},
		{"empty string", `""`, "", false},
		{"empty array", `[]`, "", false},
		{"array with non-text type", `[{"type":"image_url","url":"http://x"}]`, "", false},
		{"array with mixed types", `[{"type":"image_url","url":"x"},{"type":"text","text":"hi"}]`, "hi", false},
		{"multiple text parts", `[{"type":"text","text":"a"},{"type":"text","text":"b"}]`, "ab", false},
		{"invalid object", `{"type":"text","text":"hi"}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c ChatContent
			err := json.Unmarshal([]byte(tt.json), &c)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected unmarshal error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if c.Text != tt.want {
				t.Errorf("got %q, want %q", c.Text, tt.want)
			}
		})
	}
}

func TestChatContentMarshalRoundtrip(t *testing.T) {
	c := ChatContent{Text: "hello"}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"hello"` {
		t.Errorf("marshal = %s, want %q", data, `"hello"`)
	}
}

func TestHandleInvalidJSON(t *testing.T) {
	mock := &mockBridge{}
	handler := handleChatCompletions(mock)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{not json`))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandlePromptError(t *testing.T) {
	mock := &mockBridge{promptErr: io.EOF}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestHandleStreamPromptError(t *testing.T) {
	mock := &mockBridge{promptErr: io.EOF}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	// Should return 502 since no chunks were sent before the error
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestHandleStreamPromptErrorAfterChunks(t *testing.T) {
	mock := &mockBridge{
		chunks:               []string{"Hello"},
		promptErr:            io.EOF,
		promptErrAfterChunks: true,
	}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}

	if len(events) != 3 {
		t.Fatalf("events = %d, want 3; got:\n%s", len(events), w.Body.String())
	}

	var final ChatCompletionResponse
	if err := json.Unmarshal([]byte(events[1]), &final); err != nil {
		t.Fatalf("unmarshal final event: %v", err)
	}
	if final.Choices[0].FinishReason == nil || *final.Choices[0].FinishReason != "error" {
		t.Fatalf("finish_reason = %v, want error", final.Choices[0].FinishReason)
	}
	if events[2] != "[DONE]" {
		t.Fatalf("last event = %q, want [DONE]", events[2])
	}
}

func TestHandleInvalidContentShape(t *testing.T) {
	mock := &mockBridge{}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":{"type":"text","text":"hi"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleModelsGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	handleModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"id":"kiro"`) {
		t.Errorf("body missing kiro model: %s", w.Body.String())
	}
}

func TestHandleModelsRejectsPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	w := httptest.NewRecorder()
	handleModels(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestNewCompletionIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newCompletionID()
		if seen[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		seen[id] = true
	}
}

func TestLogMiddlewareVerbose(t *testing.T) {
	// Enable verbose for this test
	old := verboseLog
	verboseLog = true
	defer func() { verboseLog = old }()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(logMiddleware(inner))
	defer server.Close()

	resp, err := http.Post(server.URL, "application/json", strings.NewReader(`{"test":true}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestToolCallMarshal(t *testing.T) {
	tc := ToolCall{
		Index:    0,
		ID:       "call_1",
		Type:     "function",
		Function: ToolCallFunction{Name: "read", Arguments: `{"path":"main.go"}`},
	}
	data, err := json.Marshal(tc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"id":"call_1"`) {
		t.Errorf("missing id: %s", s)
	}
	if !strings.Contains(s, `"type":"function"`) {
		t.Errorf("missing type: %s", s)
	}
	if !strings.Contains(s, `"name":"read"`) {
		t.Errorf("missing function name: %s", s)
	}
}

func TestToolCallInDelta(t *testing.T) {
	msg := ChatMessage{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			Index:    0,
			ID:       "call_1",
			Type:     "function",
			Function: ToolCallFunction{Name: "grep"},
		}},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"tool_calls"`) {
		t.Errorf("missing tool_calls: %s", s)
	}
	if !strings.Contains(s, `"name":"grep"`) {
		t.Errorf("missing function name: %s", s)
	}
}

func TestToolCallOmittedWhenEmpty(t *testing.T) {
	msg := ChatMessage{
		Role:    "assistant",
		Content: ChatContent{Text: "hello"},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "tool_calls") {
		t.Errorf("tool_calls should be omitted when empty: %s", data)
	}
}

type mockBridgeWithToolCalls struct {
	gotText string
}

func (m *mockBridgeWithToolCalls) Prompt(text string, onEvent func(PromptEvent)) (string, error) {
	m.gotText = text
	onEvent(PromptEvent{
		Type:       EventToolCall,
		ToolCallID: "call_1",
		ToolName:   "read",
		ToolInput:  `{"path":"main.go"}`,
	})
	onEvent(PromptEvent{
		Type:       EventToolCallUpdate,
		ToolCallID: "call_1",
		ToolStatus: "completed",
	})
	onEvent(PromptEvent{Type: EventText, Text: "Here are the contents."})
	return "end_turn", nil
}

func (m *mockBridgeWithToolCalls) Close() error { return nil }

func TestHandleStreamWithToolCalls(t *testing.T) {
	old := showToolAnnotations
	showToolAnnotations = true
	defer func() { showToolAnnotations = old }()

	mock := &mockBridgeWithToolCalls{}
	handler := handleChatCompletions(mock)

	body := `{"model":"kiro","messages":[{"role":"user","content":"read main.go"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}

	// Should have: tool annotation text, content text, finish chunk, [DONE]
	var hasToolText, hasContentText bool
	for _, ev := range events {
		if ev == "[DONE]" {
			continue
		}
		var resp ChatCompletionResponse
		if err := json.Unmarshal([]byte(ev), &resp); err != nil {
			continue
		}
		if len(resp.Choices) == 0 {
			continue
		}
		delta := resp.Choices[0].Delta
		if delta != nil && strings.Contains(delta.Content.Text, "🔧") {
			hasToolText = true
		}
		if delta != nil && delta.Content.Text == "Here are the contents." {
			hasContentText = true
		}
	}
	if !hasToolText {
		t.Errorf("no tool annotation found in events:\n%s", w.Body.String())
	}
	if !hasContentText {
		t.Errorf("no content text found in events:\n%s", w.Body.String())
	}
}
