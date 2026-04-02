---
paths:
- "*.go"
- "internal/core/dag*"
- "internal/core/context*"
---
## DAG Storage

SQLite database. Tables:
- nodes(id, parent_id, role, content, model, provider, timestamp, token_count)
- branches(id, name, head_node_id, forked_from)
- node_aliases(alias, node_id)

Every message is an immutable node. Branches fork from any node. Nothing deleted. Assistant nodes auto-aliased (a1, a2, a3...).
