# kiro-bridge

Bring Kiro to every AI tool in your workflow — Raycast, Continue, Open WebUI, Claude Code, and anything that speaks the OpenAI or Anthropic API.

```
Client  ──POST /v1/chat/completions──▶  kiro-bridge  ──JSON-RPC/stdio──▶  kiro-cli acp
        ──POST /v1/messages──────────▶               ──session/prompt────▶
        ◀──SSE stream────────────────               ◀──session/update────
```

kiro-bridge is a lightweight HTTP server that translates between mainstream chat APIs — the [OpenAI Chat Completions API](https://platform.openai.com/docs/api-reference/chat) and the [Anthropic Messages API](https://docs.anthropic.com/en/api/messages) — and Kiro's [Agent Client Protocol (ACP)](https://agentclientprotocol.com). It spawns `kiro-cli acp` as a backend, so you get everything Kiro offers — models, tools, file access, web search — streamed back through a standard endpoint that your client already speaks.

## Quick start

### 1. Install

Download a prebuilt binary from [Releases](https://github.com/szympajka/kiro-bridge/releases):

```bash
# macOS Apple Silicon
curl -L https://github.com/szympajka/kiro-bridge/releases/latest/download/kiro-bridge_darwin_arm64.tar.gz | tar xz
```

On macOS, remove the quarantine flag before running:
```bash
xattr -d com.apple.quarantine kiro-bridge
```

Or build from source:

```bash
# with nix (reads .version automatically)
nix build

# or with go directly
go build -ldflags "-X main.version=$(cat .version)" -o kiro-bridge .
```

### 2. Install the agent config

Copy the agent config to your Kiro agents directory:

```bash
mkdir -p ~/.kiro/agents
cp agent.json ~/.kiro/agents/kiro-bridge.json
```

This pre-approves read-only tools (`fs_read`, `grep`, `glob`, `web_search`) so Kiro can use them without prompting for approval in headless mode.

Optionally, add `resources` to load steering docs into every session:

```json
"resources": [
  "file://~/.kiro/steering/coding.md",
  "file://~/.kiro/steering/workflow.md"
]
```

### 3. Run it

```bash
./result/bin/kiro-bridge

# or with go
go run .
```

That's it — the bridge is running at `http://127.0.0.1:11435/v1`. No background service needed to try it out. See [Running as a background service](#running-as-a-background-service) when you want it always-on.

### 4. Connect your client

The bridge exposes an OpenAI-compatible API at `http://127.0.0.1:11435/v1` and an Anthropic-compatible API at the same base URL (`/v1/messages`). Point any OpenAI- or Anthropic-compatible client at it.

The example below uses Raycast (an OpenAI-compatible client) — add to `~/.config/raycast/ai/providers.yaml`:

```yaml
providers:
  - id: kiro
    name: Kiro
    base_url: http://localhost:11435/v1
    models:
      - id: kiro
        name: "Kiro (Claude via ACP)"
        context: 200000
        abilities:
          temperature:
            supported: false
          vision:
            supported: false
          system_message:
            supported: true
          tools:
            supported: false
```

## Features

- **OpenAI-compatible API** — `POST /v1/chat/completions` with streaming (SSE) and non-streaming responses
- **Anthropic-compatible API** — `POST /v1/messages` with streaming and non-streaming responses, for clients that speak the Anthropic Messages format (experimental, see disclaimer below)
- **Dynamic model list** — `GET /v1/models` serves real models from Kiro (Claude Opus, Sonnet, Haiku, DeepSeek, and more)
- **Vision support** — forward images from OpenAI `image_url` content to Kiro (experimental)
- **Tool transparency** — Kiro's tools (file search, grep, web search) run inside the ACP session with optional annotations
- **Conversation replay** — include assistant message history for multi-turn context (experimental)
- **Token usage estimation** — approximate token counts from Kiro's context usage metadata
- **Health endpoint** — `GET /healthz` for monitoring
- **Resilient** — exponential backoff on startup, session reconnect after repeated errors, graceful cancellation

> **Tool permissions:** Kiro requests permission for write/edit tools. The bridge rejects these by default. To allow writes, add the tools to `allowedTools` in your agent config so Kiro pre-approves them without asking.

### Anthropic endpoint (`/v1/messages`)

The bridge can also speak the Anthropic Messages format so that Anthropic-format clients (such as Claude Code) can use Kiro as a backend. Requests still go to your local `kiro-cli` — the bridge never contacts Anthropic.

> **⚠️ At your own risk:** Pointing Anthropic's own client software (e.g. Claude Code) at a non-Anthropic backend may violate Anthropic's terms of service. This endpoint is provided for experimentation and interoperability. You are responsible for ensuring your usage complies with the terms of any client you connect.

Limitations of this endpoint:

- **No interactive tool use.** Kiro runs its tools internally, so the bridge never emits Anthropic `tool_use` content blocks or `stop_reason: "tool_use"`. Clients that expect to drive tool calls themselves won't receive that handshake. Kiro's own tools can be surfaced as text annotations via `KIRO_BRIDGE_SHOW_TOOLS`.
- **Approximate token usage.** Kiro exposes only an approximate total token count, not a prompt/completion split. The bridge reports this total in `output_tokens` and leaves `input_tokens` at `0`, so `input_tokens + output_tokens` sums to the approximate total. Treat these as estimates, not exact counts.
- **`max_tokens` is ignored.** The field is required by the Anthropic format and accepted, but Kiro manages its own output limits, so it is not forwarded.

## Configuration

Configuration is via environment variables, with optional command-line flags that take precedence.

| Variable | Default | Description |
|----------|---------|-------------|
| `KIRO_BRIDGE_PORT` | `11435` | HTTP server port |
| `KIRO_BRIDGE_CWD` | current directory | Working directory for ACP sessions |
| `KIRO_CLI_PATH` | `kiro-cli` | Path to kiro-cli binary |
| `KIRO_BRIDGE_AGENT` | `kiro-bridge` | Kiro agent config to activate |
| `KIRO_BRIDGE_MAX_BODY` | `1048576` | Max request body size in bytes (default 1MB) |
| `KIRO_BRIDGE_VERBOSE` | unset | Set to enable debug logging |
| `KIRO_BRIDGE_SHOW_TOOLS` | unset | Set to show tool call annotations in responses (experimental) |
| `KIRO_BRIDGE_REPLAY_HISTORY` | unset | Set to include assistant messages in prompt for conversation replay (experimental) |
| `KIRO_BRIDGE_ENABLE_IMAGES` | unset | Set to forward image content from OpenAI requests to ACP (experimental) |
| `KIRO_BRIDGE_CONTEXT_WINDOW` | `200000` | Context window size for token usage estimation |

### Command-line flags

Flags override the corresponding environment variable when set. Precedence is **flag > environment variable > default**.

| Flag | Env equivalent | Description |
|------|----------------|-------------|
| `--port` | `KIRO_BRIDGE_PORT` | HTTP server port |
| `--cwd` | `KIRO_BRIDGE_CWD` | Working directory for ACP sessions |
| `--cli` | `KIRO_CLI_PATH` | Path to kiro-cli binary |
| `--agent` | `KIRO_BRIDGE_AGENT` | Kiro agent config to activate |
| `--version` | — | Print version and exit |

> **Note:** As of v0.8.0 the binary parses command-line flags. Unknown arguments now cause it to exit with an error. Wrapper scripts or service definitions that pass unexpected arguments must be updated.

## Running as a background service

### Option A: Nix Darwin module (recommended)

Add the flake input and import the module:

```nix
# flake.nix
inputs.kiro-bridge.url = "github:szympajka/kiro-bridge";

# darwin configuration
imports = [ inputs.kiro-bridge.darwinModules.default ];
services.kiro-bridge = {
  enable = true;
  user = "youruser";
};
```

This sets up a launchd service with sensible defaults. Available options:

| Option | Default | Description |
|--------|---------|-------------|
| `user` | (required) | macOS username |
| `cwd` | `/Users/<user>` | Working directory for ACP sessions |
| `cliPath` | `~/.nix-profile/bin/kiro-cli` | Path to kiro-cli binary |
| `port` | `11435` | HTTP server port |
| `agent` | `kiro-bridge` | Kiro agent config to activate |
| `extraEnv` | `{}` | Extra environment variables |

### Option B: macOS launchd plist (without Nix)

Create `~/Library/LaunchAgents/com.kiro-bridge.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.kiro-bridge</string>
  <key>Program</key>
  <string>/path/to/kiro-bridge</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>KIRO_BRIDGE_CWD</key>
    <string>/Users/you</string>
    <key>KIRO_CLI_PATH</key>
    <string>/path/to/kiro-cli</string>
  </dict>
  <key>KeepAlive</key>
  <true/>
  <key>RunAtLoad</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/tmp/kiro-bridge.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/kiro-bridge.log</string>
</dict>
</plist>
```

Load with:

```bash
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.kiro-bridge.plist
```

## Performance

The bridge adds ~5µs of overhead per request. Model inference (500ms-5s) dominates latency by 100,000x.

| Operation | Time | Allocs |
|-----------|------|--------|
| Full stream handler (4 chunks) | 5.2µs | 85 |
| Full non-stream handler | 2.8µs | 54 |
| JSON-RPC request serialize | 0.5µs | 4 |
| Content unmarshal (string) | 0.2µs | 5 |

Run benchmarks with `go test -bench=. -benchmem ./...`

## Development

```bash
# enter dev shell
nix develop

# run tests
go test -v ./...

# run e2e tests (requires authenticated kiro-cli)
go test -tags e2e -timeout 60s -v ./...
```

## Release Helpers

```bash
# create annotated tag from .version
nix run .#tag-release

# create the tag, then push the current branch and tags
nix run .#release
```

## How it works

- The bridge spawns `kiro-cli acp` as a child process and communicates via JSON-RPC over stdio.
- On startup failure, it retries with exponential backoff (1s→60s cap) instead of crashing. The HTTP server starts immediately and returns 503 while connecting.
- It creates one ACP session at startup and reuses it for subsequent requests.
- Incoming requests on `/v1/chat/completions` (OpenAI) and `/v1/messages` (Anthropic) are translated to ACP `session/prompt` calls.
- ACP `agent_message_chunk` notifications are streamed back as SSE chunks in the format matching the endpoint the client called.
- Kiro tool calls (file search, web fetch, etc.) happen transparently inside the ACP session — only the final text response is returned to the client.
- When Kiro requests permission for write tools, the bridge rejects by default. Pre-approved tools in the agent config bypass this.
- System and user messages from the current request are flattened into a single prompt; full conversation replay from `messages[]` is planned but not implemented yet.

---

Built by [szympajka](https://github.com/szympajka) with [Kiro](https://kiro.dev) and for the ❤️ of useful software.
