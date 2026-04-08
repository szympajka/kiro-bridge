package main

import "encoding/json"

// OpenAI Chat Completions API types

type ChatCompletionRequest struct {
	Model    string          `json:"model"`
	Messages []ChatMessage   `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
}

type ChatMessage struct {
	Role    string      `json:"role,omitempty"`
	Content ChatContent `json:"content"`
}

// ChatContent handles both "content": "string" and "content": [{"type":"text","text":"..."}]
type ChatContent struct {
	Text string
}

func (c *ChatContent) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Text = s
		return nil
	}
	// Try array of content parts
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &parts); err == nil {
		for _, p := range parts {
			if p.Type == "text" {
				c.Text += p.Text
			}
		}
		return nil
	}
	return nil
}

func (c ChatContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Text)
}

type ChatCompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatChoice       `json:"choices"`
}

type ChatChoice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatMessage `json:"delta,omitempty"`
	FinishReason *string      `json:"finish_reason"`
}
