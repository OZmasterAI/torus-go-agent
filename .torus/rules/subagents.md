---
paths:
- "internal/features/subagents*"
- "internal/features/workflows*"
---
## Sub-agents

SpawnWithProvider (async, isolated DAG branch). HookOnSubagentStart fires before launch — hooks can inject AdditionalContext into the subagent's system prompt.

Types:
- builder — all 6 tools
- researcher — read, glob, grep
- tester — bash, read, glob, grep

Workflows: RunSequential, RunParallel, RunLoop. All fire TaskCreated/TaskCompleted hooks.
