[SYSTEM: Architecture & Schema]

## DAG Storage
SQLite database. Tables:
- nodes(id, parent_id, role, content, model, provider, timestamp, token_count)
- branches(id, name, head_node_id, forked_from)

Every message is an immutable node. Branches fork from any node. Nothing deleted.

## Context Management (3 layers, every turn)
1. Continuous compression (priority 50): older messages gradually truncated by age + importance. Last 10 kept verbatim.
2. Zone budgeting (priority 60): archive zone (25%) for old context, history zone (75%) for recent. Output tokens reserved.
3. Compaction (when threshold hit): LLM summarizes old messages onto a new DAG branch. Original preserved. Triggers at token % OR message count.

## Tools
bash, read, write, edit, glob, grep, spawn, delegate, recall_branch + MCP tools at runtime.

## Sub-agents
spawn (async) / delegate (sync). Types: builder, researcher, tester. Each on isolated DAG branch.

## Hooks
31 hook points. Pipelines — handlers run in priority order (lower first). Can observe, block, or transform.
