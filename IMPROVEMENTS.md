# Improvements

## Done

| # | Title | Version | Description |
|---|-------|---------|-------------|
| 1 | Exponential backoff on startup | v0.3.0 | Bridge retries internally (1s→60s cap). HTTP server returns 503 while connecting. |
| 2 | Full JSON-RPC error logging | v0.3.0 | RPCError includes code and optional data field per spec. |
| 3 | Raw ACP traffic logging | v0.4.0 | Verbose mode logs every JSON-RPC line in both directions. |
| 4 | JSON-RPC string/int IDs | v0.4.0 | RPCID type supports both per spec. |
| 5 | Handle session/request_permission | v0.4.0 | Responds with reject_once to prevent deadlocks. |
| 6 | Tool call annotations | v0.4.0 | Text annotations behind `KIRO_BRIDGE_SHOW_TOOLS`. |
| 7 | Map ACP stop reasons | v0.5.0 | end_turn→stop, max_tokens→length, etc. |
| 8 | Expose real models | v0.5.0 | `/v1/models` serves models from session/new response. |
| 9 | Declare clientCapabilities | v0.5.0 | Declares promptCapabilities.image: true. |
| 10 | Use `_meta.tool_name` | v0.5.0 | Tool annotations use actual tool name with title fallback. |
| 11 | session/cancel | v0.6.0 | Sends cancel notification with suppress-abort pattern. |
| 12 | Tool call content parsing | v0.6.0 | Accepts both single ContentBlock and array. |
| 13 | Method not found errors | v0.6.0 | Responds -32601 for unhandled agent requests. |
| 14 | Conversation history replay | v0.6.0 | Behind `KIRO_BRIDGE_REPLAY_HISTORY`. Flattens assistant messages into prompt. |
| 15 | Image passthrough | v0.6.0 | Behind `KIRO_BRIDGE_ENABLE_IMAGES`. Forwards image_url as ACP image blocks. |

## Next

All improvements completed. See [PROTOCOL_SUPPORT.md](PROTOCOL_SUPPORT.md) for remaining protocol limitations.
