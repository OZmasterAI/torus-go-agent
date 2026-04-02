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
      loop.go                    — agent loop: RunStream, tool exec, auto-alias
      hooks.go                   — 31 hook points, HookRegistry
      context.go                 — compaction pipeline (sliding/LLM)
      compression.go             — continuous compression, zone budgets
      compression_ops.go         — operation detection, semantic scoring
      helpers.go                 — AgentEvent types
      tokenizer.go               — token estimation
    features/
      skills.go                  — skill registry (.md → /commands)
      subagents.go               — SubAgentManager
      mcp.go                     — MCP stdio JSON-RPC
      telemetry.go               — token/cost tracking
      routing.go                 — message routing helper
      workflows.go               — sequential/parallel agent orchestration
    providers/
      provider.go                — Router: weighted routing, fallback chains
      anthropic.go openrouter.go gemini.go — LLM providers
      oauth.go                   — Anthropic OAuth PKCE flow
    safety/safety.go             — ScanSecrets, CheckSafety
    tools/tools.go               — 6 tools: read, write, edit, bash, glob, grep
    tui/shared/
      thinking.go                — thinking block render, verbosity cycling
    types/types.go               — shared types: Provider, Message, Usage, Tool
    ui/
      tui.go                     — Bubble Tea TUI-A: chat, sidebar, streaming
      tui_commands.go            — TUI-A command handlers
      startup.go                 — interactive setup screen