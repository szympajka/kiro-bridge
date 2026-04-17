# Improvements

| # | Title | Status | Severity | Description |
|---|-------|--------|----------|-------------|
| 1 | Exponential backoff on startup | ✅ Done | High | Bridge retries internally (1s→60s cap) instead of crashing. HTTP server returns 503 while connecting. *(v0.3.0)* |
| 2 | Full JSON-RPC error logging | ✅ Done | High | RPCError includes code and optional data field per spec. All error sites propagate full context. *(v0.3.0)* |
| 3 | Raw ACP traffic logging | ✅ Done | High | Verbose mode logs every JSON-RPC line in both directions (`acp >>>` / `acp <<<`). *(v0.4.0)* |
| 4 | JSON-RPC string/int IDs | ✅ Done | High | RPCID type supports both string and integer IDs per spec. Kiro uses string UUIDs for permission requests. *(v0.4.0)* |
| 5 | Handle session/request_permission | ✅ Done | High | Bridge responds with reject_once to prevent deadlocks. Use agent config pre-approved tools for write access. *(v0.4.0)* |
| 6 | Surface tool calls to clients | ✅ Done | High | Tool calls streamed as text annotations. Experimental, gated behind `KIRO_BRIDGE_SHOW_TOOLS`. *(v0.4.0)* |
| 7 | Always log kiro-cli stderr | 🔲 Todo | High | Child process errors silently dropped unless KIRO_BRIDGE_VERBOSE is set. Startup failures should always surface kiro-cli stderr. |
| 8 | Session health checks / reconnect | 🔲 Todo | High | No stale session detection. Repeated prompt failures should trigger session recreation. Prevents silent degradation. |
| 9 | Expose real models | 🔲 Todo | Medium | Bridge hardcodes single "kiro" model. Kiro exposes 12 models via session/new response. |
| 10 | Health endpoint | 🔲 Todo | Medium | `GET /healthz` returning 200/503 based on bridge state. Enables external monitoring. |
| 11 | Log rotation | 🔲 Todo | Medium | `/tmp/kiro-bridge.log` is append-only. Add newsyslog config or size-based rotation. |
| 12 | Launchd ThrottleInterval | 🔲 Todo | Low | Secondary safety net for real process crashes. Launchd still restarts aggressively on unexpected exits. |
