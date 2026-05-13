package main

import (
	"encoding/json"
	"fmt"
)

// Anthropic Messages API types

type AnthropicMessagesRequest struct {
	Model     string                  `json:"model"`
	MaxTokens int                     `json:"max_tokens"`
	System    AnthropicMessageContent `json:"system,omitempty"`
	Messages  []AnthropicMessage      `json:"messages"`
	Stream    bool                    `json:"stream,omitempty"`
}

type AnthropicMessage struct {
	Role    string                  `json:"role"`
	Content AnthropicMessageContent `json:"content"`
}

// AnthropicMessageContent handles both "content": "string" and "content": [{"type":"text","text":"..."}]
type AnthropicMessageContent struct {
	Text string
}

func (c *AnthropicMessageContent) UnmarshalJSON(data []byte) error {
	c.Text = ""

	if string(data) == "null" {
		return nil
	}

	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Text = s
		return nil
	}
	// Try array of content blocks
	var parts []AnthropicContentBlock
	if err := json.Unmarshal(data, &parts); err == nil {
		for _, p := range parts {
			if p.Type == "text" {
				c.Text += p.Text
			}
		}
		return nil
	}

	return fmt.Errorf("unsupported content format")
}

func (c AnthropicMessageContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Text)
}

type AnthropicMessageResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []AnthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason,omitempty"`
	Usage      AnthropicUsage          `json:"usage"`
}

type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AnthropicMessageStartEvent struct {
	Type    string                   `json:"type"`
	Message AnthropicMessageResponse `json:"message"`
}

type AnthropicContentBlockStartEvent struct {
	Type         string                `json:"type"`
	Index        int                   `json:"index"`
	ContentBlock AnthropicContentBlock `json:"content_block"`
}

type AnthropicContentBlockDeltaEvent struct {
	Type  string                `json:"type"`
	Index int                   `json:"index"`
	Delta AnthropicContentDelta `json:"delta"`
}

type AnthropicContentDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type AnthropicMessageDeltaEvent struct {
	Type  string                `json:"type"`
	Delta AnthropicMessageDelta `json:"delta"`
	Usage AnthropicUsage        `json:"usage"`
}

type AnthropicMessageDelta struct {
	StopReason string `json:"stop_reason"`
}

type AnthropicMessageStopEvent struct {
	Type string `json:"type"`
}

type AnthropicErrorEvent struct {
	Type  string         `json:"type"`
	Error AnthropicError `json:"error"`
}

type AnthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
