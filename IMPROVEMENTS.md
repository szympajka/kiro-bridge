# Improvements

| # | Title | Status | Severity | Description |
|---|-------|--------|----------|-------------|
| 1 | Exponential backoff on startup | ✅ Done | High | Bridge retries internally (1s→60s cap) instead of crashing. HTTP server returns 503 while connecting. |
| 2 | Full JSON-RPC error logging | ✅ Done | High | RPCError includes code and optional data field per spec. All error sites propagate full context. |
| 3 | Always log kiro-cli stderr | 🔲 Todo | High | Child process errors silently dropped unless KIRO_BRIDGE_VERBOSE is set. Startup failures should always surface kiro-cli stderr. |
| 4 | Session health checks / reconnect | 🔲 Todo | High | No stale session detection. Repeated prompt failures should trigger session recreation. Prevents silent degradation. |
| 5 | Health endpoint | 🔲 Todo | Medium | `GET /healthz` returning 200/503 based on bridge state. Enables external monitoring. |
| 6 | Log rotation | 🔲 Todo | Medium | `/tmp/kiro-bridge.log` is append-only. Add newsyslog config or size-based rotation. |
| 7 | Launchd ThrottleInterval | 🔲 Todo | Low | Secondary safety net for real process crashes. Launchd still restarts aggressively on unexpected exits. |
