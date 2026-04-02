---
paths:
- "internal/providers/*"
- "internal/core/loop*"
---
## Providers

Weighted multi-provider routing with fallback chains. Streaming token-by-token output.

Provider interface: Complete() (non-streaming) and StreamComplete() (streaming). Run() uses Complete by default; RunStream() uses StreamComplete. ForceStream config overrides Run() to stream.

Router: AddProvider with weights, SelectProvider for weighted random, fallback chain on error. RewardRouter wraps Router with async reward-model scoring.
