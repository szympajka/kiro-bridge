package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var anthropicMessageCounter int64

func newMessageID() string {
	return fmt.Sprintf("msg_%d_%d", time.Now().Unix(), atomic.AddInt64(&anthropicMessageCounter, 1))
}

func handleAnthropicMessages(b Bridge) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req AnthropicMessagesRequest
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("error: decode request: %v", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if len(req.Messages) == 0 {
			log.Printf("error: empty messages")
			http.Error(w, "messages required", http.StatusBadRequest)
			return
		}

		messages := anthropicToChatMessages(req)
		promptText := buildPromptText(messages)
		promptBlocks := buildPromptBlocks(messages)
		messageID := newMessageID()
		model := req.Model
		if model == "" {
			model = "kiro"
		}

		debugf("prompt: stream=%v model=%q len=%d", req.Stream, model, len(promptText))

		if req.Stream {
			handleAnthropicStream(w, b, promptBlocks, messageID, model)
		} else {
			handleAnthropicNonStream(w, b, promptBlocks, messageID, model)
		}
	}
}

func anthropicToChatMessages(req AnthropicMessagesRequest) []ChatMessage {
	messages := make([]ChatMessage, 0, len(req.Messages)+1)
	if req.System.Text != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: ChatContent{Text: req.System.Text}})
	}
	for _, m := range req.Messages {
		messages = append(messages, ChatMessage{Role: m.Role, Content: ChatContent{Text: m.Content.Text}})
	}
	return messages
}

func mapAnthropicStopReason(acpReason string) string {
	if acpReason == "stop_sequence" {
		return "stop_sequence"
	}
	switch mapStopReason(acpReason) {
	case "length":
		return "max_tokens"
	case "stop":
		return "end_turn"
	}
	return "end_turn"
}

func writeAnthropicEvent(w http.ResponseWriter, flusher http.Flusher, event string, resp any) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("error: marshal chunk: %v", err)
		return
	}
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func writeAnthropicStreamStart(w http.ResponseWriter, flusher http.Flusher, id string, model string) {
	writeAnthropicEvent(w, flusher, "message_start", AnthropicMessageStartEvent{
		Type: "message_start",
		Message: AnthropicMessageResponse{
			ID:      id,
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{},
			Model:   model,
			Usage:   AnthropicUsage{},
		},
	})
	writeAnthropicEvent(w, flusher, "content_block_start", AnthropicContentBlockStartEvent{
		Type:         "content_block_start",
		Index:        0,
		ContentBlock: AnthropicContentBlock{Type: "text", Text: ""},
	})
}

func writeAnthropicStreamTerminal(w http.ResponseWriter, flusher http.Flusher, stopReason string, usage UsageInfo) {
	writeAnthropicEvent(w, flusher, "content_block_stop", AnthropicContentBlockStopEvent{Type: "content_block_stop", Index: 0})
	writeAnthropicEvent(w, flusher, "message_delta", AnthropicMessageDeltaEvent{
		Type:  "message_delta",
		Delta: AnthropicMessageDelta{StopReason: stopReason},
		Usage: AnthropicUsage{OutputTokens: usage.TotalTokens},
	})
	writeAnthropicEvent(w, flusher, "message_stop", AnthropicMessageStopEvent{Type: "message_stop"})
}

func writeAnthropicStreamError(w http.ResponseWriter, flusher http.Flusher, err error) {
	writeAnthropicEvent(w, flusher, "error", AnthropicErrorEvent{
		Type:  "error",
		Error: AnthropicError{Type: "api_error", Message: err.Error()},
	})
}

func handleAnthropicStream(w http.ResponseWriter, b Bridge, prompt []ContentBlock, id string, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	first := true
	stopReason, err := b.Prompt(prompt, func(ev PromptEvent) {
		var text string
		switch ev.Type {
		case EventText:
			text = ev.Text
		case EventToolCall:
			if !showToolAnnotations {
				return
			}
			text = fmt.Sprintf("\n\n🔧 %s\n\n---\n\n", ev.ToolName)
		default:
			return
		}
		if first {
			writeAnthropicStreamStart(w, flusher, id, model)
			first = false
		}
		writeAnthropicEvent(w, flusher, "content_block_delta", AnthropicContentBlockDeltaEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: AnthropicContentDelta{Type: "text_delta", Text: text},
		})
	})

	if err != nil {
		log.Printf("error: prompt failed: %v", err)
		if first {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeAnthropicStreamError(w, flusher, err)
		return
	}

	if first {
		writeAnthropicStreamStart(w, flusher, id, model)
	}
	writeAnthropicStreamTerminal(w, flusher, mapAnthropicStopReason(stopReason), b.Usage())
}

func handleAnthropicNonStream(w http.ResponseWriter, b Bridge, prompt []ContentBlock, id string, model string) {
	var full strings.Builder
	stopReason, err := b.Prompt(prompt, func(ev PromptEvent) {
		if ev.Type == EventText {
			full.WriteString(ev.Text)
		}
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	usage := b.Usage()
	resp := AnthropicMessageResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Content:    []AnthropicContentBlock{{Type: "text", Text: full.String()}},
		Model:      model,
		StopReason: mapAnthropicStopReason(stopReason),
		Usage:      AnthropicUsage{OutputTokens: usage.TotalTokens},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
