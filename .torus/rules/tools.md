---
paths:
- "internal/tools/*"
- "internal/safety/*"
---
## Tools

6 built-in tools via BuildDefaultTools():
- **bash**: shell command, 30s timeout, returns stdout+stderr
- **read**: file contents by path, supports offset/limit
- **write**: create/overwrite file at path
- **edit**: old_string→new_string replacement (must be unique match)
- **glob**: file pattern matching (e.g. "**/*.go")
- **grep**: ripgrep-based content search with regex

Safety: ScanSecrets runs on write/edit — blocks .env, keys, credentials. CheckSafety validates tool inputs.

MCP tools added at runtime via features/mcp.go (stdio JSON-RPC).
