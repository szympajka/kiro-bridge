package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func buildPromptText(messages []ChatMessage) string {
	var parts []string
	for _, m := range messages {
		switch m.Role {
		case "system":
			parts = append(parts, "System: "+m.Content.Text)
		case "user":
			parts = append(parts, m.Content.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

var completionCounter int64

var maxBodyBytes int64 = 1 << 20 // 1MB default

func init() {
	if v := os.Getenv("KIRO_BRIDGE_MAX_BODY"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxBodyBytes = n
		}
	}
}

func newCompletionID() string {
	return fmt.Sprintf("chatcmpl-%d-%d", time.Now().Unix(), atomic.AddInt64(&completionCounter, 1))
}

func handleChatCompletions(b Bridge) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ChatCompletionRequest
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

		promptText := buildPromptText(req.Messages)
		completionID := newCompletionID()
		created := time.Now().Unix()
		model := req.Model
		if model == "" {
			model = "kiro"
		}

		debugf("prompt: stream=%v model=%q len=%d", req.Stream, model, len(promptText))

		if req.Stream {
			handleStream(w, b, promptText, completionID, created, model)
		} else {
			handleNonStream(w, b, promptText, completionID, created, model)
		}
	}
}

func writeStreamTerminal(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model, finishReason string) {
	finalJSON := fmt.Sprintf(`{"id":%q,"object":"chat.completion.chunk","created":%d,"model":%q,"choices":[{"index":0,"delta":{},"finish_reason":%q}]}`, id, created, model, finishReason)
	fmt.Fprintf(w, "data: %s\n\n", finalJSON)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func handleStream(w http.ResponseWriter, b Bridge, prompt, id string, created int64, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	first := true
	_, err := b.Prompt(prompt, func(chunk string) {
		delta := &ChatMessage{Content: ChatContent{Text: chunk}}
		if first {
			delta.Role = "assistant"
			first = false
		}
		resp := ChatCompletionResponse{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []ChatChoice{{Index: 0, Delta: delta}},
		}
		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("error: marshal chunk: %v", err)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	})

	if err != nil {
		log.Printf("error: prompt failed: %v", err)
		if first {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeStreamTerminal(w, flusher, id, created, model, "error")
		return
	}

	writeStreamTerminal(w, flusher, id, created, model, "stop")
}

func handleNonStream(w http.ResponseWriter, b Bridge, prompt, id string, created int64, model string) {
	var full strings.Builder
	_, err := b.Prompt(prompt, func(chunk string) {
		full.WriteString(chunk)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	stop := "stop"
	resp := ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []ChatChoice{{
			Index:        0,
			Message:      &ChatMessage{Role: "assistant", Content: ChatContent{Text: full.String()}},
			FinishReason: &stop,
		}},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
