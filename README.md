# Torus Agent

A terminal-native AI agent framework written in Go. Multi-provider, DAG-based conversation storage, continuous context compression, and a Bubble Tea TUI.

## Features

- **Multi-provider routing** — Anthropic, OpenRouter, Gemini, Azure OpenAI, Vertex AI. Weighted routing with automatic fallback chains.
- **DAG conversation storage** — SQLite-backed immutable message graph. Branch, fork, alias, and switch between conversation threads. Nothing is ever deleted.
- **3-layer context management** — Continuous compression (operation-aware scoring), zone budgeting (token budget allocation), and compaction (sliding window or LLM summarization).
- **6 built-in tools** — bash, read, write, edit, glob, grep. Secret scanning on write/edit.
- **Sub-agents** — Spawn isolated agents (builder, researcher, tester) with their own DAG branches.
- **MCP support** — Connect external tool servers via JSON-RPC over stdio.
- **Skills** — Load markdown files as slash commands.
- **31 hook points** — Observe, block, or transform at any stage of the agent loop.
- **Multiple channels** — TUI (Bubble Tea), Telegram bot, HTTP/REST API.
- **Anthropic OAuth** — PKCE flow for first-party authentication.

## Quick Start

```bash
# Clone
git clone https://github.com/OZmasterAI/torus-go-agent.git
cd torus-go-agent

# Build
go build -o torus_go_agent ./cmd

# Set an API key
export OPENROUTER_API_KEY=your-key-here
# Or: ANTHROPIC_API_KEY, GEMINI_API_KEY, etc.

# Run — startup screen lets you pick provider & model
./torus_go_agent
```

On first run, the interactive setup screen appears. Pick a provider and model, and the config is saved for future runs.

## Configuration

Config is resolved in order:

1. `$TORUS_CONFIG_DIR` (env override)
2. `./config` (if running from repo root)
3. `~/.config/torus_go_agent/`

Copy `.env.example` to `.env` for API keys:

```bash
cp .env.example .env
```

### Key settings (`config.json`)

| Setting | Default | Description |
|---------|---------|-------------|
| `provider` | — | Provider: `anthropic`, `openrouter`, `gemini`, `azure`, `vertex` |
| `model` | — | Model ID (e.g. `claude-sonnet-4-6`) |
| `maxTokens` | 8192 | Max output tokens per response |
| `compaction` | `llm` | Compaction mode: `off`, `sliding`, `llm` |
| `continuousCompression` | `true` | Per-turn gradual message compression |
| `zoneBudgeting` | `true` | Zone-based token budget allocation |
| `thinking` | `high` | Thinking level: `low`, `mid`, `high`, `max` (Anthropic only) |

## Channels

```bash
# TUI (default)
./torus_go_agent

# Telegram bot
export TELEGRAM_BOT_TOKEN=your-token
./torus_go_agent --telegram

# HTTP API
./torus_go_agent --http
```

## MCP Servers

Add external tool servers in `config.json`:

```json
{
  "mcpServers": {
    "memory": {
      "command": "python3",
      "args": ["/path/to/memory_server.py"],
      "env": {"MEMORY_DIR": "/home/user/data/memory"}
    },
    "web-search": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-web-search"]
    }
  }
}
```

Each server is launched as a subprocess. Tools are discovered via JSON-RPC over stdio and become available to the agent at runtime. Use `/mcp-tools` to list connected tools.

## Custom Skills

Skills are markdown files in the skills directory (`skillsDir` in config, or `./skills/` by default). Each `.md` file becomes a `/command`.

Create `skills/review.md`:

```markdown
# Code review assistant

Review the code the user points to. Check for:
- Security issues (injection, XSS, secrets)
- Error handling gaps
- Performance concerns

Be concise. List issues as bullet points.
```

This creates the `/review` command. The first `#` heading becomes the description, the rest is injected as the prompt.

## Hooks

31 hook points let you observe, block, or transform at any stage:

```go
hooks := core.NewHookRegistry()

// Log every tool call
hooks.Register(core.HookBeforeToolCall, "audit", func(ctx context.Context, data *core.HookData) error {
    log.Printf("tool=%s args=%v", data.ToolName, data.ToolArgs)
    return nil
})

// Block dangerous commands
hooks.RegisterPriority(core.HookBeforeToolCall, "safety", func(ctx context.Context, data *core.HookData) error {
    if data.ToolName == "bash" && strings.Contains(data.ToolArgs["command"].(string), "rm -rf") {
        data.Block = true
        data.BlockReason = "destructive command blocked"
    }
    return nil
}, 10) // priority 10 = runs before default (100)
```

### Available hook points

| Phase | Hooks |
|-------|-------|
| **LLM** | `before_llm_call`, `after_llm_call` |
| **Tools** | `before_tool_call`, `after_tool_call`, `after_tool_result` |
| **Context** | `before_context_build`, `after_context_build`, `on_token_count` |
| **Turn** | `on_turn_start`, `on_turn_end`, `on_user_input` |
| **Agent lifecycle** | `on_agent_start`, `on_agent_end`, `on_app_start`, `on_app_shutdown` |
| **Branching** | `before_new_branch`, `after_new_branch`, `on_branch_switch`, `on_node_added` |
| **Compaction** | `pre_compact`, `post_compact`, `pre_clear`, `post_clear` |
| **Skills** | `before_skill`, `after_skill` |
| **Sub-agents** | `before_spawn`, `after_spawn`, `on_subagent_complete` |
| **Errors** | `on_error`, `on_stop_failure`, `before_loop_exit` |

## Screenshot

> TODO: Add TUI screenshot or GIF here

## Slash Commands

`/new` `/clear` `/compact` `/fork` `/switch` `/branches` `/alias` `/messages` `/steering` `/stats` `/agents` `/mcp-tools` `/skills` `/exit`

## Architecture

```
cmd/main.go              Entry point, provider setup, hook wiring
internal/
  core/                  DAG, agent loop, hooks, compression, tokenizer
  features/              Skills, sub-agents, MCP, routing, workflows
  providers/             Multi-provider router, Anthropic, OpenRouter, Gemini, OAuth
  tools/                 bash, read, write, edit, glob, grep
  ui/                    Bubble Tea TUI
  channels/              Channel interface + Telegram, HTTP adapters
  config/                XDG config loading
  commands/              Slash command handlers
  safety/                Secret scanning
  types/                 Shared types
```

## Requirements

- Go 1.25+

## License

Apache 2.0 — see [LICENSE](LICENSE).
