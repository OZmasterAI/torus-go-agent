# Evolve-Harness Implementation Handoff

## Branch: `evolve-harness`

## Completed
- **Step 0**: Batch channel (`internal/channels/batch/batch.go`) — DONE (prior session)
- **Step 1A**: Multi-turn batch extension — DONE (this session)
  - Added `MultiTurn bool` to `Config` struct in batch.go
  - Added `--multi-turn` flag parsing in `cmd/main.go`
  - Added `runMultiTurn()` method with `TurnResult` and `MultiTurnResult` types
  - Supports both `["string", ...]` and `[{"role":"user","content":"..."}]` JSON formats
  - Fixed Go `json.Unmarshal` partial-fill bug (must reset slice to nil before fallback parse)
  - 6 batch tests passing, full `go test ./...` green
  - Commits: `c02f181` (auto: main changes), `c5bbe5c` (fix: nil reset)

## Remaining Tasks (Steps 1B through 4)

Read these three files for full context:
- `evolve-harness/CONTEXT.md` — background, architecture, what Step 0 built
- `evolve-harness/DESIGN.md` — system design, scoring, directory structure
- `evolve-harness/PLAN.md` — step-by-step implementation with file specs

### 1B: Create 5 MVP coding tasks
`evolve-harness/tasks/coding/` — pick 2 long-horizon, 2 tool-sensitive, 1 error-recovery from PLAN.md.
Each needs: `prompt.txt`, `test.sh` (executable, exits 0/1), `workspace/` with real code files.
Tasks must be **harness-sensitive** (compression quality, tool choice, context management matter).

### 1C: Create 3 MVP tool-use tasks
`evolve-harness/tasks/tool_use/` — each needs: `prompt.txt`, `verify.py` (reads result.json trace, checks tool sequence), `workspace/`.

### 1D: Create 2 MVP compression tasks
`evolve-harness/tasks/compression/` — each needs: `conversation.json` (20-30 turn realistic conversation as JSON array), `queries.json` (3-5 questions with keyword lists for retention scoring).

### 2: Write `evolve-harness/evaluate.py`
Full evaluator script — runs agent against all tasks, computes per-dimension scores and composite. See PLAN.md Step 2 for function signatures and scoring formulas.

### 3: Write `evolve-harness/proposer.md`
Instruction file for Claude Code as the proposer agent. See PLAN.md Step 3 for the outline.

### 4: Write `evolve-harness/run.sh`
Shell orchestrator loop: baseline -> proposer -> apply + build -> evaluate -> revert -> report. See PLAN.md Step 4 for the full script template.

## Commit Strategy
- Commit after 1B+1C+1D (all tasks together)
- Commit after Step 2 (evaluator)
- Commit after Steps 3+4 (proposer + orchestrator)

## Notes
- `config/models.json` has an unrelated uncommitted change (Hermes model entry) — leave it alone
- All work targets the `evolve-harness` branch
