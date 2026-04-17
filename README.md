# kiro-bridge

OpenAI-compatible HTTP proxy to [kiro-cli](https://kiro.dev) via the [Agent Client Protocol (ACP)](https://agentclientprotocol.com). Any tool that speaks the OpenAI API can use Kiro as a backend.

```
Client  ──POST /v1/chat/completions──▶  kiro-bridge  ──JSON-RPC/stdio──▶  kiro-cli acp
        ◀──SSE stream────────────────               ◀──session/update────
```

## What it does now

- Translates OpenAI `POST /v1/chat/completions` to ACP `session/prompt`
- Streaming (SSE) and non-streaming responses
- Pre-approved read-only tools (file search, grep, web search) run transparently inside Kiro
- Tool call annotations in responses (experimental, enable with `KIRO_BRIDGE_SHOW_TOOLS`)
- Exponential backoff on startup — retries connecting to kiro-cli instead of crashing, returns 503 while connecting
- Handles ACP permission requests — rejects by default to prevent unintended writes
- One persistent ACP session per bridge process; stateless mode is not implemented yet
- Configurable agent, working directory, body limit, and logging
- Localhost only, 1MB default body limit

> **Tool permissions:** Kiro requests permission for write/edit tools. The bridge rejects these by default. To allow writes, add the tools to `allowedTools` in your agent config so Kiro pre-approves them without asking.

### Protocol support

The bridge translates a subset of each protocol. See [PROTOCOL_SUPPORT.md](PROTOCOL_SUPPORT.md) for the full matrix.

| Area | Supported | Partial | Not supported |
|------|-----------|---------|---------------|
| OpenAI request fields | 2 | 3 | 11 |
| OpenAI message types | 1 | 2 | 2 |
| ACP agent methods | 4 | — | 3 |
| ACP session updates | 1 | 2 | 5 |
| ACP content types | 1 | — | 4 |
| JSON-RPC 2.0 | 5 | — | 2 |

## Planned

- Surface tool calls and reasoning steps as streaming events
- Conversation history replay from OpenAI messages array
- Stateless mode — new ACP session per request, no context carryover
- Session TTL — auto-prune stale sessions
- Linux systemd service example

## Prerequisites

- [kiro-cli](https://kiro.dev) installed and authenticated
- [Nix](https://nixos.org) (for building) or Go 1.24+

## Setup

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

The bridge exposes an OpenAI-compatible API at `http://127.0.0.1:11435/v1`. Point any OpenAI-compatible client at it.

Example for Raycast — add to `~/.config/raycast/ai/providers.yaml`:

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

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KIRO_BRIDGE_PORT` | `11435` | HTTP server port |
| `KIRO_BRIDGE_CWD` | current directory | Working directory for ACP sessions |
| `KIRO_CLI_PATH` | `kiro-cli` | Path to kiro-cli binary |
| `KIRO_BRIDGE_AGENT` | `kiro-bridge` | Kiro agent config to activate |
| `KIRO_BRIDGE_MAX_BODY` | `1048576` | Max request body size in bytes (default 1MB) |
| `KIRO_BRIDGE_VERBOSE` | unset | Set to enable debug logging |
| `KIRO_BRIDGE_SHOW_TOOLS` | unset | Set to show tool call annotations in responses (experimental) |

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
- Incoming OpenAI `/v1/chat/completions` requests are translated to ACP `session/prompt` calls.
- ACP `agent_message_chunk` notifications are streamed back as OpenAI SSE chunks.
- Kiro tool calls (file search, web fetch, etc.) happen transparently inside the ACP session — only the final text response is returned to the client.
- When Kiro requests permission for write tools, the bridge rejects by default. Pre-approved tools in the agent config bypass this.
- System and user messages from the current request are flattened into a single prompt; full conversation replay from `messages[]` is planned but not implemented yet.

---

Built by [szympajka](https://github.com/szympajka) with [Kiro](https://kiro.dev) and for the ❤️ of useful software.
