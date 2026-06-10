# Protocol Support Matrix

Current translation coverage between mainstream chat APIs (OpenAI Chat Completions, Anthropic Messages) and ACP (Agent Client Protocol) over JSON-RPC 2.0.

Legend: ✅ Supported | ⚠️ Partial | ❌ Not supported | 🔄 Custom handling | — Not applicable

## OpenAI Chat Completions → ACP

### Request fields

| Field | Status | Notes |
|-------|--------|-------|
| `model` | ⚠️ | Echoed in response. Not forwarded to ACP — Kiro selects model internally. |
| `messages` | ⚠️ | System + user + assistant flattened to prompt. Assistant included when `KIRO_BRIDGE_REPLAY_HISTORY` enabled. |
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
| `image_url` | ⚠️ | Parsed and forwarded as ACP image block when `KIRO_BRIDGE_ENABLE_IMAGES` set. |
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
| `usage` | ✅ | Estimated from `_kiro.dev/metadata` contextUsagePercentage × 200k context window. |
| `system_fingerprint` | ❌ | Not generated. |

### Streaming

| Feature | Status | Notes |
|---------|--------|-------|
| SSE `data:` format | ✅ | |
| `[DONE]` sentinel | ✅ | |
| `delta.content` | ✅ | From `agent_message_chunk`. |
| `delta.role` | ✅ | `"assistant"` on first chunk. |
| `delta.tool_calls` | ❌ | Not mapped. |

## Anthropic Messages → ACP

The Anthropic endpoint (`/v1/messages`) reuses the same ACP translation path as the OpenAI endpoint (see [ACP → OpenAI](#acp--openai) below for the shared agent-side mapping). It is experimental — see the README disclaimer on connecting Anthropic client software to a non-Anthropic backend.

### Request fields

| Field | Status | Notes |
|-------|--------|-------|
| `model` | ⚠️ | Echoed in response. Not forwarded to ACP — Kiro selects model internally. Defaults to `kiro` when empty. |
| `messages` | ⚠️ | User + assistant flattened to prompt. Assistant included when `KIRO_BRIDGE_REPLAY_HISTORY` enabled. |
| `system` | ⚠️ | Prepended as "System: " text. Accepts string or content-block array. |
| `stream` | ✅ | Maps to SSE via `session/update` notifications. |
| `max_tokens` | ❌ | Required by the Anthropic format and accepted, but not forwarded — Kiro manages its own output limits. |
| `temperature` | ❌ | No ACP equivalent. Silently ignored. |
| `top_p` | ❌ | No ACP equivalent. Silently ignored. |
| `top_k` | ❌ | No ACP equivalent. Silently ignored. |
| `stop_sequences` | ❌ | No ACP equivalent. Silently ignored. |
| `tools` | ❌ | ACP tools are agent-side. Client tool definitions ignored. |
| `tool_choice` | ❌ | Kiro decides tool usage autonomously. |
| `thinking` | ❌ | No ACP equivalent. Kiro manages its own reasoning. Silently ignored. |
| `service_tier` | ❌ | Not applicable — requests go to local Kiro, not Anthropic. Silently ignored. |
| `metadata` | ❌ | Not forwarded. |
| `cache_control` | ❌ | No ACP equivalent. Silently ignored. |
| `container` | ❌ | No ACP equivalent. Silently ignored. |
| `inference_geo` | ❌ | Not applicable — requests go to local Kiro. Silently ignored. |
| `output_config` | ❌ | No ACP structured output mode. Silently ignored. |

### Message content

| Type | Status | Notes |
|------|--------|-------|
| `content` (string) | ✅ | Direct mapping. |
| `content` (block array) | ⚠️ | `text` blocks concatenated. Non-text blocks (image, etc.) dropped. |

### Response fields

| Field | Status | Notes |
|-------|--------|-------|
| `id` | 🔄 | Bridge-generated `msg_{ts}_{n}`. |
| `type` | 🔄 | `message`. |
| `role` | 🔄 | `assistant`. |
| `content[].text` | ✅ | From `agent_message_chunk`. Single text block. |
| `content[].tool_use` | ❌ | Never emitted — Kiro runs tools internally. |
| `model` | 🔄 | Echoed from request. |
| `stop_reason` | ✅ | Maps ACP stop reasons: end_turn→end_turn, max_tokens→max_tokens, stop_sequence→stop_sequence, others→end_turn. Never `tool_use`. |
| `stop_sequence` | 🔄 | Always `null`. |
| `usage.input_tokens` | ⚠️ | Always `0` — Kiro exposes no prompt/completion split. |
| `usage.output_tokens` | ⚠️ | Approximate total from `_kiro.dev/metadata`, not a true output-only count. `input + output` sums to the approximate total. |

### Streaming

| Event | Status | Notes |
|-------|--------|-------|
| `message_start` | ✅ | Empty `content`, `stop_reason`/`stop_sequence` null, `usage` present. |
| `content_block_start` | ✅ | Single text block at index 0. |
| `content_block_delta` (`text_delta`) | ✅ | From `agent_message_chunk`. |
| `content_block_delta` (`input_json_delta`) | ❌ | No tool_use blocks emitted. |
| `content_block_stop` | ✅ | |
| `message_delta` | ✅ | Carries `stop_reason`, `stop_sequence: null`, cumulative `usage`. |
| `message_stop` | ✅ | |
| `ping` | — | Not emitted (optional per spec). |
| `error` | ✅ | Emitted on prompt failure after streaming has started. |

## ACP → OpenAI

### Agent methods (client → agent)

| Method | Status | Notes |
|--------|--------|-------|
| `initialize` | ✅ | Declares promptCapabilities.image. |
| `authenticate` | ❌ | Not needed — kiro-cli handles auth. |
| `session/new` | ✅ | Creates session with CWD. Parses models from response. |
| `session/load` | ❌ | Not implemented. Kiro declares `loadSession: true`. |
| `session/prompt` | ✅ | Text content blocks only. |
| `session/set_mode` | ✅ | Activates agent config. |
| `session/list` | ❌ | Not implemented. |

### Agent notifications (client → agent)

| Notification | Status | Notes |
|-------------|--------|-------|
| `session/cancel` | ✅ | Sends cancel notification. Suppresses subsequent abort error (Zed pattern). |

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
| `content` (array) | ✅ | Accepts both single ContentBlock and array via json.RawMessage. |
| `locations` | ❌ | Not surfaced. |

## JSON-RPC 2.0

| Feature | Status | Notes |
|---------|--------|-------|
| Request/response | ✅ | |
| Notifications (send) | ✅ | `session/cancel` sent on cancel. |
| Notifications (receive) | ✅ | `session/update` handled. |
| Error object (code + message + data) | ✅ | |
| ID as integer | ✅ | Used for bridge → agent requests. |
| ID as string | ✅ | Handled for agent → client requests. |
| Batch requests | ❌ | Not needed for stdio. |
| Method not found error (-32601) | ✅ | Responds -32601 for unhandled agent requests. |

## Actionable gaps (priority order)

- **Anthropic `usage` accuracy** — `input_tokens` is always `0` and `output_tokens` carries an approximate total, since Kiro exposes no prompt/completion split. Blocked on richer usage data from the ACP backend.
- **Conversation replay** — assistant history from `messages[]` is only included when `KIRO_BRIDGE_REPLAY_HISTORY` is set; full multi-turn replay is not the default.

All other translatable features are implemented (some behind feature flags).

## Known limitations

### Client-defined tools not supported

OpenAI's tool protocol is **bidirectional** — model proposes a tool call, client executes it, client returns the result. ACP's tool protocol is **unidirectional** — the agent executes tools internally, the client only observes.

These cannot be cleanly bridged. Client-defined tools (e.g. Raycast's `@calculator`, `@location`) require the model to output structured `tool_calls` that the client executes. Kiro doesn't know about client tools and has no mechanism to invoke them.

Kiro's own tools (read, grep, glob, web_search, etc.) run transparently inside the ACP session. They can be surfaced as text annotations via `KIRO_BRIDGE_SHOW_TOOLS` but cannot be rendered as interactive tool call UI in either OpenAI- or Anthropic-compatible clients. On the Anthropic endpoint this means no `tool_use` content blocks and no `stop_reason: "tool_use"` are ever emitted.

**Raycast config:** Set `tools.supported: false` to avoid errors when using tool-dependent extensions.
