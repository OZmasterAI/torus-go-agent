---
paths:
- "*.go"
- "cmd/*"
- "internal/**"
---
## Schema

cmd/main.go                    — entry point, provider setup, hook wiring
  internal/
    channels/
      channel.go                 — Channel interface, registry, auto-select
      tui/tui.go                 — TUI-A channel adapter
      tui-b/tui.go               — TUI-B channel adapter
      telegram/telegram.go       — Telegram bot channel
      http/http.go               — HTTP/REST channel
    commands/commands.go         — slash commands: /fork /switch /alias /new /clear /compact...
    config/config.go             — XDG config loading
    core/
      dag.go                     — SQLite DAG: nodes, branches, aliases, migrations
      loop.go                    — agent loop: Run/RunStream, Complete/StreamComplete, tool exec
      hooks.go                   — 41 hook points, HookRegistry, AdditionalContext
      context.go                 — compaction pipeline (sliding/LLM)
      compression.go             — continuous compression, zone budgets
      compression_ops.go         — operation detection, semantic scoring
      helpers.go                 — AgentEvent types
      tokenizer.go               — token estimation
      reload.go                  — PromptReloader: file watcher for hot-reload
      instructions.go            — multi-tier instruction discovery, @include, frontmatter
    features/
      skills.go                  — skill registry (.md → /commands)
      subagents.go               — SubAgentManager, context injection
      mcp.go                     — MCP stdio JSON-RPC
      telemetry.go               — token/cost tracking
      routing.go                 — message routing helper
      workflows.go               — sequential/parallel/loop agent orchestration
    providers/
      provider.go                — Router: weighted routing, fallback chains
      anthropic.go openrouter.go gemini.go — LLM providers
      oauth.go                   — Anthropic OAuth PKCE flow
      reward_router.go           — async reward-model scoring wrapper
    safety/safety.go             — ScanSecrets, CheckSafety
    tools/tools.go               — 6 tools: read, write, edit, bash, glob, grep
    tui/shared/
      thinking.go                — thinking block render, verbosity cycling
    types/types.go               — shared types: Provider, Message, Usage, Tool
    ui/
      tui.go                     — Bubble Tea TUI-A: chat, sidebar, streaming
      tui_commands.go            — TUI-A command handlers
      startup.go                 — interactive setup screen (21 config fields)
    ui-b/
      model.go                   — TUI-B Bubble Tea model
      startup.go                 — TUI-B setup screen (21 config fields)
