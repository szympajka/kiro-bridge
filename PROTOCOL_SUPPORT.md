# Protocol Support Matrix

Current translation coverage between OpenAI Chat Completions API and ACP (Agent Client Protocol) over JSON-RPC 2.0.

Legend: ✅ Supported | ⚠️ Partial | ❌ Not supported | 🔄 Custom handling | — Not applicable

## OpenAI Chat Completions → ACP

### Request fields

| Field | Status | Notes |
|-------|--------|-------|
| `model` | ⚠️ | Echoed in response. Not forwarded to ACP — Kiro selects model internally. |
| `messages` | ⚠️ | System + user flattened to single prompt. Assistant/tool messages dropped. No history replay. |
| `stream` | ✅ | Maps to SSE via `session/update` notifications. |
| `temperature` | ❌ | No ACP equivalent. Silently ignored. |
| `top_p` | ❌ | No ACP equivalent. Silently ignored. |
| `max_tokens` | ❌ | No ACP equivalent. Silently ignored. |
| `stop` | ❌ | No ACP equivalent. Silently ignored. |
| `tools` | ❌ | ACP tools are agent-side. Client tool definitions ignored. |
| `tool_choice` | ❌ | Kiro decides tool usage autonomously. |
| `n` | ❌ | ACP returns single response. |
| `response_format` | ❌ | No ACP structured output mode. |
| `stream_options` | ❌ | No ACP usage reporting. |
| `seed` | ❌ | No ACP equivalent. |
| `user` | ❌ | Not forwarded. |

### Message types

| Type | Status | Notes |
|------|--------|-------|
| `system` | ⚠️ | Prepended as "System: " text. Loses role structure. |
| `user` (string) | ✅ | Direct mapping. |
| `user` (content array) | ⚠️ | Text parts extracted. Image/audio/file parts dropped. |
| `assistant` | ❌ | Dropped. No conversation replay. |
| `tool` | ❌ | Dropped. ACP tools execute server-side. |

### Content parts

| Type | Status | Notes |
|------|--------|-------|
| `text` | ✅ | Direct mapping to ACP text ContentBlock. |
| `image_url` | ❌ | Kiro declares `image: true` but bridge doesn't pass through yet. |
| `input_audio` | ❌ | Kiro declares `audio: false`. |

### Response fields

| Field | Status | Notes |
|-------|--------|-------|
| `id` | 🔄 | Bridge-generated `chatcmpl-{ts}-{n}`. |
| `object` | 🔄 | `chat.completion` or `chat.completion.chunk`. |
| `created` | 🔄 | Bridge timestamp. |
| `model` | 🔄 | Echoed from request. |
| `choices[].message.content` | ✅ | From `agent_message_chunk`. |
| `choices[].message.tool_calls` | ❌ | Not mapped. Text annotations behind FF instead. |
| `choices[].finish_reason` | ✅ | Maps ACP stop reasons: end_turn→stop, max_tokens→length, etc. |
| `usage` | ❌ | ACP has no token counts. |
| `system_fingerprint` | ❌ | Not generated. |

### Streaming

| Feature | Status | Notes |
|---------|--------|-------|
| SSE `data:` format | ✅ | |
| `[DONE]` sentinel | ✅ | |
| `delta.content` | ✅ | From `agent_message_chunk`. |
| `delta.role` | ✅ | `"assistant"` on first chunk. |
| `delta.tool_calls` | ❌ | Not mapped. |

## ACP → OpenAI

### Agent methods (client → agent)

| Method | Status | Notes |
|--------|--------|-------|
| `initialize` | ✅ | Sends protocol version + client info. Capabilities declared as empty. |
| `authenticate` | ❌ | Not needed — kiro-cli handles auth. |
| `session/new` | ✅ | Creates session with CWD. Response parsed for sessionId only — modes/models not used. |
| `session/load` | ❌ | Not implemented. Kiro declares `loadSession: true`. |
| `session/prompt` | ✅ | Text content blocks only. |
| `session/set_mode` | ✅ | Activates agent config. |
| `session/list` | ❌ | Not implemented. |

### Agent notifications (client → agent)

| Notification | Status | Notes |
|-------------|--------|-------|
| `session/cancel` | ❌ | Not sent on client disconnect. |

### Client methods (agent → client)

| Method | Status | Notes |
|--------|--------|-------|
| `session/request_permission` | ✅ | Responds with `reject_once`. |
| `fs/read_text_file` | ❌ | Not implemented. |
| `fs/write_text_file` | ❌ | Not implemented. |
| `terminal/create` | ❌ | Out of scope. |
| `terminal/output` | ❌ | Out of scope. |
| `terminal/release` | ❌ | Out of scope. |
| `terminal/wait_for_exit` | ❌ | Out of scope. |
| `terminal/kill` | ❌ | Out of scope. |

### Session update notifications (agent → client)

| Subtype | Status | Notes |
|---------|--------|-------|
| `agent_message_chunk` | ✅ | Streamed as SSE text content. |
| `tool_call` | ⚠️ | Parsed. Text annotation behind `KIRO_BRIDGE_SHOW_TOOLS`. |
| `tool_call_update` | ⚠️ | Parsed. Not surfaced to client. |
| `plan` | ❌ | Dropped. Never observed from Kiro. |
| `thought_message_chunk` | ❌ | Dropped. Never observed from Kiro. |
| `user_message_chunk` | ❌ | Dropped. |
| `mode_change` | ❌ | Dropped. |
| `available_commands` | ❌ | Dropped. |

### Content block types

| Type | Status | Notes |
|------|--------|-------|
| `text` | ✅ | |
| `image` | ❌ | Not passed through in prompts or responses. |
| `audio` | ❌ | Kiro declares unsupported. |
| `resource` (embedded) | ❌ | Kiro declares unsupported. |
| `resource_link` | ❌ | Not handled. |

### Stop reasons

| ACP Reason | OpenAI Mapping | Status |
|------------|---------------|--------|
| `end_turn` | `stop` | ✅ |
| `max_tokens` | `length` | ✅ |
| `max_turn_requests` | `stop` | ✅ |
| `refusal` | `stop` | ✅ |
| `cancelled` | `stop` | ✅ |

### Tool call fields

| Field | Status | Notes |
|-------|--------|-------|
| `toolCallId` | ✅ | Parsed. |
| `title` | ✅ | Used as tool name in annotations. |
| `kind` | ❌ | Parsed but not surfaced. |
| `status` | ✅ | Parsed. |
| `rawInput` | ✅ | Parsed as ToolInput. |
| `rawOutput` | ❌ | Not surfaced. |
| `content` (array) | ❌ | Parse fails — expects single ContentBlock, ACP sends array. |
| `locations` | ❌ | Not surfaced. |

## JSON-RPC 2.0

| Feature | Status | Notes |
|---------|--------|-------|
| Request/response | ✅ | |
| Notifications (send) | ❌ | Bridge doesn't send `session/cancel`. |
| Notifications (receive) | ✅ | `session/update` handled. |
| Error object (code + message + data) | ✅ | |
| ID as integer | ✅ | Used for bridge → agent requests. |
| ID as string | ✅ | Handled for agent → client requests. |
| Batch requests | ❌ | Not needed for stdio. |
| Method not found error (-32601) | ❌ | Bridge doesn't send proper error for unhandled methods. |

## Actionable gaps (priority order)

1. **Expose real models** — parse session/new response, serve in `/v1/models`
2. **Declare clientCapabilities** — tell agent what bridge supports (Zed declares fs, terminal, auth; we send empty `{}`)
3. **Use `_meta.tool_name` for tool annotations** — currently using `title` ("Finding *.go") instead of actual tool name (`glob`). Zed extracts from `_meta.tool_name`.
4. **Conversation history** — replay messages[] or flatten with full context
5. **Image passthrough** — Kiro supports it, bridge just needs to forward
6. **session/cancel on disconnect** — send notification when client drops SSE. Suppress subsequent abort error (Zed pattern).
7. **Tool call content parsing** — fix array vs single ContentBlock
8. **Method not found errors** — respond -32601 for unhandled agent requests (Zed does this)
