# config.json Reference

Configuration file for the Torus Go agent. All fields are optional — sensible defaults are applied.

## Top-Level Keys

| Key | Type | Description |
|-----|------|-------------|
| `telegram` | object | Telegram bot credentials and access control |
| `agent` | object | Model selection, routing, compaction, and compression settings |
| `data` | object | Data directory path |
| `mcpServers` | object | Named MCP server definitions |
| `skillsDir` | string | Path to skills directory |

---

## `telegram`

| Field | Type | Default | Env Override | Description |
|-------|------|---------|-------------|-------------|
| `botToken` | string | `""` | `TELEGRAM_BOT_TOKEN` | Telegram Bot API token |
| `allowedUsers` | int64[] | `[]` | — | Telegram user IDs permitted to interact with the bot |

---

## `agent`

### Model & Provider

| Field | Type | Default | Env Override | Description |
|-------|------|---------|-------------|-------------|
| `provider` | string | `""` | `AGENT_PROVIDER` | LLM provider key (e.g. `"anthropic"`, `"openai"`, `"openrouter"`, `"nvidia"`, `"azure"`, `"vertex"`, `"xai"`, `"gemini"`) |
| `model` | string | `""` | `AGENT_MODEL` | Model ID to use (e.g. `"claude-sonnet-4-20250514"`) |
| `maxTokens` | int | `8192` | — | Maximum output tokens per response |
| `contextWindow` | int | `128000` | — | Model's full context window size in tokens. Used by compaction threshold calculations |

### Routing

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `routing` | RoutingEntry[] | `[]` | Weighted multi-provider routing. Each entry has `provider`, `model`, and `weight` (relative integer). Requests are distributed proportionally across entries |
| `fallbackOrder` | string[] | `[]` | Ordered list of `"provider:model"` keys. If the primary provider fails, the next entry is tried |
| `smartRouting` | bool | `false` | Enable LLM-based routing that picks the best model per request |
| `smartRoutingModel` | string | `""` | Model used to make smart routing decisions. Empty = use the main model |

### Compaction (full context rewrite)

Compaction triggers when the conversation approaches the context window limit. It rewrites the conversation into a compressed summary.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `compaction` | string | `"llm"` | Compaction strategy. `"llm"` uses a language model to summarize |
| `compactionModel` | string | `""` | Model used for LLM compaction. Empty = use the main model |
| `compactionTrigger` | string | `"both"` | What triggers compaction: `"tokens"` (threshold %), `"messages"` (max count), or `"both"` (whichever fires first) |
| `compactionThreshold` | int | `65` | Percentage of `contextWindow` that triggers token-based compaction |
| `compactionMaxMessages` | int | `0` | Message count that triggers compaction. `0` = disabled (token-based only) |
| `compactionKeepLastN` | int | `10` | Number of recent messages kept verbatim (not summarized) after compaction |

### Continuous Compression (per-turn gradual)

Continuous compression runs every turn, gradually compressing older messages so the conversation degrades gracefully instead of hitting a hard compaction wall.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `continuousCompression` | bool | `true` | Enable per-turn gradual compression of older messages |
| `compressionKeepLast` | int | `10` | Number of recent messages always kept verbatim by compression |
| `compressionMinMessages` | int | `0` | Don't start compressing until this many messages exist. `0` = compress from `keepLast + 1` onward |

### Zone Budgeting

Divides the usable context into zones (archive, working, recent) with configurable budget splits.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `zoneBudgeting` | bool | `true` | Enable zone-based token budget allocation |
| `zoneArchivePercent` | int | `25` | Percentage of usable budget allocated to the archive zone |

### Steering & Thinking

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `steeringMode` | string | `""` | System prompt steering intensity. `"mild"` (default behavior) or `"aggressive"` |
| `persistThinking` | bool | `false` | Store model thinking/reasoning blocks as DAG nodes for later inspection |

### Azure-Specific

| Field | Type | Default | Env Override |
|-------|------|---------|-------------|
| `azureResource` | string | `""` | `AZURE_RESOURCE` |
| `azureDeployment` | string | `""` | `AZURE_DEPLOYMENT` |
| `azureApiVersion` | string | `""` | `AZURE_API_VERSION` |

### Vertex AI-Specific

| Field | Type | Default | Env Override |
|-------|------|---------|-------------|
| `vertexProject` | string | `""` | `VERTEX_PROJECT` |
| `vertexRegion` | string | `""` | `VERTEX_REGION` |

---

## `data`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dir` | string | `""` | Data directory path. Relative paths resolve from the config directory. Empty falls back to `~/.local/share/torus_go_agent` |

---

## `mcpServers`

A map of server name to server config. Each server:

| Field | Type | Description |
|-------|------|-------------|
| `command` | string | Executable to launch |
| `args` | string[] | CLI arguments |
| `env` | object | Environment variables injected into the server process |

Example:

```json
{
  "mcpServers": {
    "memory": {
      "command": "python",
      "args": ["-m", "memory_server"],
      "env": { "DB_PATH": "/tmp/memory.db" }
    }
  }
}
```

---

## API Keys

API keys are **not** stored in config.json. They are read from environment variables at call time:

| Provider | Env Variable |
|----------|-------------|
| `anthropic` | `ANTHROPIC_API_KEY` |
| `openai` | `OPENAI_API_KEY` |
| `openrouter` | `OPENROUTER_API_KEY` |
| `nvidia` | `NVIDIA_API_KEY` |
| `xai` | `XAI_API_KEY` |
| `gemini` | `GEMINI_API_KEY` |
| `azure` | `AZURE_OPENAI_API_KEY` |
| `vertex` | `VERTEX_ACCESS_TOKEN` |

---

## Routing Entry

Used in the `routing` array:

```json
{
  "routing": [
    { "provider": "anthropic", "model": "claude-sonnet-4-20250514", "weight": 70 },
    { "provider": "openai", "model": "gpt-4o", "weight": 30 }
  ]
}
```

`weight` is a relative integer — 70/30 means ~70% of requests go to Anthropic, ~30% to OpenAI.

---

## Minimal Example

```json
{
  "agent": {
    "provider": "anthropic",
    "model": "claude-sonnet-4-20250514"
  }
}
```

Everything else uses defaults. Set `ANTHROPIC_API_KEY` in your environment and you're ready to go.
