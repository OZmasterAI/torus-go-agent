---
paths:
- "internal/core/compression*"
- "internal/core/context*"
- "internal/core/loop*"
---
## Context Management (3 layers)

1. **Continuous compression**: operation-aware scoring by age + importance. Recent operations kept verbatim (keepLast boundary). Older ops tiered: keep, template, or archive as one-liners.
2. **Zone budgeting**: V1 (archive 30% / history 70%) or V2 3-zone (system+archive 25% / active ops 25% / headroom 50%). V2 dynamically rebalances unused Zone 1 into Zone 2, with per-operation 50% cap.
3. **Compaction** (when threshold hit): off, sliding, or LLM summarization. DAG-native mode forks to a new branch — original preserved. Triggers at token % OR message count.
