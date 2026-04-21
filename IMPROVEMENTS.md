# Improvements

| # | Title | Status | Severity | Description |
|---|-------|--------|----------|-------------|
| 1 | Exponential backoff on startup | ✅ Done | High | Bridge retries internally (1s→60s cap) instead of crashing. HTTP server returns 503 while connecting. *(v0.3.0)* |
| 2 | Full JSON-RPC error logging | ✅ Done | High | RPCError includes code and optional data field per spec. All error sites propagate full context. *(v0.3.0)* |
| 3 | Raw ACP traffic logging | ✅ Done | High | Verbose mode logs every JSON-RPC line in both directions (`acp >>>` / `acp <<<`). *(v0.4.0)* |
| 4 | JSON-RPC string/int IDs | ✅ Done | High | RPCID type supports both string and integer IDs per spec. *(v0.4.0)* |
| 5 | Handle session/request_permission | ✅ Done | High | Bridge responds with reject_once to prevent deadlocks. *(v0.4.0)* |
| 6 | Surface tool calls to clients | ✅ Done | High | Tool calls streamed as text annotations. Gated behind `KIRO_BRIDGE_SHOW_TOOLS`. *(v0.4.0)* |
| 7 | Map ACP stop reasons | ✅ Done | Medium | end_turn→stop, max_tokens→length, etc. *(v0.4.0)* |
| 8 | Declare clientCapabilities | 🔲 Todo | High | Bridge sends empty `{}`. Should declare what it supports so agents can adapt. Zed declares fs, terminal, auth. |
| 9 | Use `_meta.tool_name` for annotations | 🔲 Todo | Medium | Currently using `title` ("Finding *.go") instead of actual tool name. Zed extracts from `_meta.tool_name`. |
| 10 | Expose real models | 🔲 Todo | Medium | Bridge hardcodes single "kiro" model. Kiro exposes 12 models via session/new response. |
| 11 | Conversation history replay | 🔲 Todo | High | messages[] flattened, assistant/tool messages dropped. No multi-turn replay. |
| 12 | Image passthrough | 🔲 Todo | Medium | Kiro declares `image: true` but bridge doesn't forward image content blocks. |
| 13 | session/cancel on disconnect | 🔲 Todo | Medium | Bridge doesn't send cancel when client drops SSE. Need suppress-abort-error pattern (from Zed). |
| 14 | Method not found errors | 🔲 Todo | Low | Bridge should respond -32601 for unhandled agent requests. |
| 15 | Always log kiro-cli stderr | 🔲 Todo | High | Child process errors silently dropped unless KIRO_BRIDGE_VERBOSE is set. |
| 16 | Session health / reconnect | 🔲 Todo | High | No stale session detection. Use `_kiro.dev/metadata` context usage %. |
| 17 | Health endpoint | 🔲 Todo | Medium | `GET /healthz` returning 200/503 based on bridge state. |
| 18 | Log rotation | 🔲 Todo | Medium | `/tmp/kiro-bridge.log` is append-only. Add newsyslog config or size-based rotation. |
| 19 | Launchd ThrottleInterval | 🔲 Todo | Low | Secondary safety net for real process crashes. |
