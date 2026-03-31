# Evolve-Harness: Implementation Plans (Steps 1-5)

## Prerequisites

- [x] Step 0: Batch channel (`internal/channels/batch/batch.go`) — DONE
- [x] Branch: `evolve-harness`

---

## Step 1: Task Set

**Goal:** Create 55 evaluation tasks with automated scoring.

### 1A. Coding Tasks (30 tasks)

Each task lives in `evolve-harness/tasks/coding/NNN_name/` with:
- `prompt.txt` — user instruction
- `test.sh` — exits 0 on pass, 1 on fail
- `workspace/` — starting files (optional)

#### Easy (10 tasks) — single file, clear spec

```
001_fizzbuzz/          # "Write fizzbuzz.py that prints 1-100"
                       # test.sh: python3 fizzbuzz.py | diff - expected.txt
002_reverse_string/    # "Write reverse.go that reverses stdin"
                       # test.sh: echo "hello" | go run reverse.go | grep "olleh"
003_count_words/       # "Write count.py that counts words in input.txt"
004_json_parser/       # "Write parse.py that extracts 'name' field from data.json"
005_fibonacci/         # "Write fib.go that prints first 20 fibonacci numbers"
006_file_copy/         # "Copy all .txt files from src/ to dst/"
007_sort_csv/          # "Sort data.csv by the 'score' column descending"
008_http_status/       # "Write check.sh that returns HTTP status of a URL"
009_find_duplicates/   # "Find duplicate lines in input.txt"
010_calc_average/      # "Calculate average of numbers in numbers.txt"
```

#### Medium (10 tasks) — multi-file, read-then-modify

```
011_fix_syntax/        # workspace has broken.py with a syntax error. "Fix the bug"
                       # test.sh: python3 broken.py && echo "PASS"
012_add_tests/         # workspace has calc.go. "Add unit tests"
                       # test.sh: go test ./...
013_refactor_function/ # workspace has utils.py with a 50-line function. "Split into 3 functions"
                       # test.sh: python3 -m pytest test_utils.py
014_convert_format/    # workspace has data.xml. "Convert to JSON"
                       # test.sh: python3 -c "import json; json.load(open('data.json'))"
015_add_logging/       # workspace has server.go. "Add structured logging"
                       # test.sh: go build ./... && grep -q 'log.' server.go
016_fix_test/          # workspace has passing code but failing test. "Fix the test"
                       # test.sh: go test ./...
017_implement_interface/ # workspace has interface.go with a Go interface. "Implement it"
                       # test.sh: go build ./... && go test ./...
018_extract_config/    # workspace has hardcoded values. "Extract to config.json"
                       # test.sh: test -f config.json && python3 verify.py
019_add_error_handling/ # workspace has code that panics on bad input. "Add error handling"
                       # test.sh: echo "bad" | go run main.go; test $? -eq 0
020_document_api/      # workspace has undocumented functions. "Add godoc comments"
                       # test.sh: go doc ./... 2>&1 | grep -c "func" | awk '$1 >= 3'
```

#### Hard (10 tasks) — multi-step, ambiguous, error recovery

```
021_debug_crash/       # workspace has a Go binary that segfaults. "Find and fix the bug"
                       # test.sh: go test -race ./...
022_migrate_schema/    # workspace has SQLite DB + old schema. "Migrate to new schema"
                       # test.sh: python3 verify_migration.py
023_build_cli/         # "Build a CLI tool that greps across multiple files with context"
                       # test.sh: ./tool --pattern "foo" testdata/ | diff - expected.txt
024_optimize_slow/     # workspace has correct but O(n^2) code. "Optimize to O(n log n)"
                       # test.sh: time go run main.go < large_input.txt (must complete in <2s)
025_merge_configs/     # workspace has 3 YAML configs. "Merge into one with conflict resolution"
                       # test.sh: python3 verify_merge.py
026_fix_race/          # workspace has Go code with a data race. "Fix the race condition"
                       # test.sh: go test -race ./... -count=5
027_parse_protocol/    # workspace has a binary protocol spec + sample data. "Write a parser"
                       # test.sh: go run parser.go < sample.bin | diff - expected.txt
028_cross_file_refactor/ # workspace has 5 Go files. "Rename Widget to Component everywhere"
                       # test.sh: go build ./... && ! grep -r "Widget" *.go
029_api_client/        # "Build an HTTP client for the API described in api_spec.json"
                       # test.sh: go test ./...
030_fix_flaky/         # workspace has a flaky test. "Make it deterministic"
                       # test.sh: for i in $(seq 1 10); do go test ./... || exit 1; done
```

### 1B. Tool-Use Tasks (15 tasks)

Each task in `evolve-harness/tasks/tool_use/NNN_name/` with:
- `prompt.txt` — instruction
- `verify.py` — reads `result.json`, checks tool sequence
- `workspace/` — starting files
- Optional: `required_tools.json`, `banned_tools.json`

```
001_read_then_edit/    # "Change function name in main.go"
                       # Required: ["read", "edit"]. Banned: ["write"]
                       # verify: must read before edit, must not use write

002_search_codebase/   # "Find all functions that return error"
                       # Required: ["grep"]. Banned: ["bash"]
                       # verify: must use grep, not bash with grep/rg

003_find_files/        # "List all Go test files"
                       # Required: ["glob"]. Banned: ["bash"]
                       # verify: must use glob, not bash with find/ls

004_create_new_file/   # "Create a new config.json file"
                       # Required: ["write"]. Banned: ["bash"]
                       # verify: must use write tool, not bash echo/cat

005_multi_file_edit/   # "Update the function signature in 3 files"
                       # Required: ["glob", "read", "edit"]
                       # verify: must glob first, then read each, then edit each

006_run_tests/         # "Run the test suite and report results"
                       # Required: ["bash"]
                       # verify: must use bash with go test or pytest

007_read_no_reread/    # "Summarize main.go"
                       # verify: must read main.go exactly once (no redundant reads)

008_edit_not_rewrite/  # "Fix the typo on line 15 of main.go"
                       # Required: ["edit"]. Banned: ["write"]
                       # verify: must use edit (surgical), not write (full rewrite)

009_search_then_fix/   # "Find and fix all TODO comments"
                       # Required: ["grep", "edit"]
                       # verify: must grep for TODO, then edit each file

010_bash_for_system/   # "Check disk space and running processes"
                       # Required: ["bash"]
                       # verify: must use bash (no other tool can do this)

011_glob_pattern/      # "Find all files matching *.test.js in the project"
                       # Required: ["glob"]
                       # verify: glob with correct pattern, not bash find

012_sequential_ops/    # "Read config.json, update the port, write it back"
                       # Required: ["read", "edit"]
                       # verify: read → edit sequence, not write (preserves formatting)

013_error_recovery/    # workspace has a file that can't be read. "Try to read secret.txt"
                       # verify: agent handles error gracefully, doesn't retry 5+ times

014_minimal_tools/     # "What's in main.go?" (simple question)
                       # verify: uses exactly 1 read call, no unnecessary tools

015_skill_trigger/     # "Commit all changes" (should trigger /commit skill)
                       # verify: trace shows skill was used (check prompt prepend)
```

### 1C. Compression Tasks (10 tasks)

Each task in `evolve-harness/tasks/compression/NNN_name/` with:
- `conversation.json` — array of user messages (multi-turn)
- `queries.json` — questions to ask after compression
- `expected.json` — expected answers (key facts)

```
001_long_coding/       # 40-turn coding conversation. Queries: "What file had the bug?"
002_multi_topic/       # 30-turn conversation spanning 3 topics. Queries per topic.
003_error_chain/       # 25-turn debugging session. Queries: "What was the root cause?"
004_decision_trail/    # 20-turn decision discussion. Queries: "Why did we choose X?"
005_code_review/       # 35-turn code review. Queries: "What files were reviewed?"
006_refactor_history/  # 30-turn refactoring. Queries: "What was renamed?"
007_config_changes/    # 20-turn config discussion. Queries: "What port did we set?"
008_test_debugging/    # 25-turn test fixing. Queries: "Which tests were flaky?"
009_architecture/      # 30-turn architecture discussion. Queries: "What pattern was chosen?"
010_mixed_tools/       # 35-turn session with varied tool use. Queries about specific tool results.
```

### Implementation

**Files to create:**
- `evolve-harness/tasks/coding/NNN_name/{prompt.txt, test.sh, workspace/}` x 30
- `evolve-harness/tasks/tool_use/NNN_name/{prompt.txt, verify.py, workspace/}` x 15
- `evolve-harness/tasks/compression/NNN_name/{conversation.json, queries.json, expected.json}` x 10

**Tests:**
- `evolve-harness/tasks/validate_tasks.py` — ensures every task has required files, test.sh is executable, verify.py parses, conversation.json is valid JSON

**Approach:** Start with 5 coding + 3 tool-use + 2 compression tasks (minimal viable set). Expand after first end-to-end run proves the pipeline works.

---

## Step 2: Evaluator

**Goal:** Python script that runs the agent against all tasks and produces scores.

### File: `evolve-harness/evaluate.py`

```python
# evaluate.py --harness=000 --tasks=evolve-harness/tasks [--binary=./torus-agent]
```

### Functions

```python
def main(harness_id, tasks_dir, binary):
    """Entry point: evaluate harness against all tasks."""

def evaluate_coding(binary, task_dir, output_dir) -> CodingResult:
    """Run agent on one coding task.
    1. Copy workspace/ to temp dir
    2. Run: binary --no-setup --batch=prompt.txt --output=output_dir/
    3. Run: test.sh in temp dir
    4. Return {passed: bool, tokens_in: int, tokens_out: int, cost: float, duration_ms: int}
    """

def evaluate_tool_use(binary, task_dir, output_dir) -> ToolUseResult:
    """Run agent on one tool-use task.
    1. Copy workspace/ to temp dir
    2. Run: binary --no-setup --batch=prompt.txt --output=output_dir/
    3. Run: verify.py result.json
    4. Return {passed: bool, tool_score: float, details: dict}
    """

def evaluate_compression(binary, task_dir, output_dir) -> CompressionResult:
    """Run agent on one compression task.
    1. Run: binary --no-setup --batch=conversation.json --output=output_dir/ --multi-turn
    2. For each query in queries.json:
       Run: binary --no-setup --batch=query.txt --output=output_dir/query_N/
    3. Score answers against expected.json
    4. Return {retention: float, tokens_used: int}
    """

def compute_scores(coding_results, tool_results, compression_results) -> Scores:
    """Compute composite score.
    coding_pass_rate = sum(passed) / len(coding)
    efficiency = coding_pass_rate / (total_tokens / baseline_tokens)
    tool_precision = sum(tool_scores) / max_possible
    compression_retention = sum(correct) / total_queries
    composite = 0.35*coding + 0.20*efficiency + 0.25*tools + 0.20*compression
    """

def write_scores(harness_dir, scores, results):
    """Write scores.json and detailed_results.json."""
```

### Output: `search_db/harness_NNN/scores.json`

```json
{
  "coding_pass_rate": 0.73,
  "context_efficiency": 0.85,
  "tool_precision": 0.68,
  "compression_retention": 0.81,
  "composite": 0.764,
  "total_input_tokens": 452000,
  "total_output_tokens": 28000,
  "total_cost": 7.23,
  "total_duration_ms": 145000,
  "per_task": { ... }
}
```

### Tests

```python
# test_evaluate.py
def test_evaluate_coding_pass():    # mock agent run + passing test.sh
def test_evaluate_coding_fail():    # mock agent run + failing test.sh
def test_evaluate_tool_use():       # mock trace + verify.py
def test_compute_scores():          # known inputs → expected composite
def test_scores_json_format():      # validate output schema
```

### Multi-Turn Extension (for compression tasks)

Extend batch channel to accept `--multi-turn` flag:
- `Config.MultiTurn bool`
- When true, read `prompt.txt` as JSON array of strings
- Send each message sequentially via `agent.Run(ctx, msg)`, collecting traces per turn
- This naturally triggers compression when context exceeds threshold

**File change:** `internal/channels/batch/batch.go` — add ~30 lines for multi-turn loop.

---

## Step 3: Proposer Skill

**Goal:** An instruction file that tells Claude Code how to read `search_db/` and propose modifications.

### File: `evolve-harness/proposer.md`

### Contents (outline)

```markdown
# Meta-Harness Proposer

## Your Task
You are optimizing go_sdk_agent to maximize a composite score:
  composite = 0.35*coding + 0.20*efficiency + 0.25*tools + 0.20*compression

## Search Database
All prior harness attempts are in evolve-harness/search_db/:
  search_db/harness_000/  (baseline)
  search_db/harness_001/  (your first attempt)
  ...

Each contains:
  src/           — snapshot of modified Go files
  scores.json    — composite + per-dimension scores
  traces/        — result.json per task (full turn-by-turn traces)

## What You Can Modify
ONLY these files (copy from internal/ to search_db/harness_NNN/src/):
  compression.go       — scoring weights, zone budgets, tool truncation limits
  compression_ops.go   — operation boundary detection, scoring signals
  context.go           — compaction strategy, keepLastN, threshold
  anthropic.go         — system prompt, cache breakpoint strategy
  tools.go             — tool descriptions (NOT implementations)
  routing.go           — smart routing thresholds and keywords

DO NOT modify: batch.go, loop.go, dag.go, providers, tool implementations.

## Process
1. Read search_db/ — start with scores.json for all prior harnesses
2. Identify the worst-performing dimension
3. Read traces from failing tasks in that dimension
4. Form a hypothesis about WHY those tasks fail
5. Propose a MINIMAL modification (prefer additive over destructive)
6. Write modified files to search_db/harness_NNN/src/
7. Write search_db/harness_NNN/hypothesis.md explaining your reasoning

## Rules
- State your hypothesis BEFORE writing code
- Change ONE thing per iteration (don't bundle multiple fixes)
- If prior iterations regressed, explain why before trying again
- Read raw traces (tool calls, turn-by-turn) — don't rely only on scores
- Prefer additive changes (Meta-Harness found 5/7 modification attempts regressed)
```

### Tests

No automated tests — this is a prompt artifact. Validate by manual review.

---

## Step 4: Orchestrator

**Goal:** Shell script that runs the full search loop.

### File: `evolve-harness/run.sh`

### Functions

```bash
snapshot_harness() {
    # Copy current optimizable Go files to search_db/harness_$1/src/
    HARNESS_DIR="$SEARCH_DB/harness_$1"
    mkdir -p "$HARNESS_DIR/src"
    cp internal/core/compression.go "$HARNESS_DIR/src/"
    cp internal/core/compression_ops.go "$HARNESS_DIR/src/"
    cp internal/core/context.go "$HARNESS_DIR/src/"
    cp internal/providers/anthropic.go "$HARNESS_DIR/src/"
    cp internal/tools/tools.go "$HARNESS_DIR/src/"
    cp internal/features/routing.go "$HARNESS_DIR/src/"
}

apply_harness() {
    # Copy modified files from search_db/harness_$1/src/ back to internal/
    HARNESS_DIR="$SEARCH_DB/harness_$1"
    for f in "$HARNESS_DIR/src/"*.go; do
        basename=$(basename "$f")
        # Route to correct package based on filename
        case "$basename" in
            compression*.go|context.go) cp "$f" internal/core/ ;;
            anthropic.go) cp "$f" internal/providers/ ;;
            tools.go) cp "$f" internal/tools/ ;;
            routing.go) cp "$f" internal/features/ ;;
        esac
    done
}

run_proposer() {
    # Launch Claude Code as the proposer
    claude -p "$(cat evolve-harness/proposer.md)

Current harness: $1
Search database: evolve-harness/search_db/
Write new harness to: evolve-harness/search_db/harness_$2/src/

Read the search_db to understand what's been tried. Propose your modification."
}
```

### Main loop

```bash
#!/bin/bash
set -euo pipefail
SEARCH_DB="evolve-harness/search_db"
TASKS="evolve-harness/tasks"
MAX_ITER=${MAX_ITER:-10}
BINARY=${BINARY:-./torus-agent}

# Baseline
if [ ! -f "$SEARCH_DB/harness_000/scores.json" ]; then
    snapshot_harness 000
    go build -o "$BINARY" ./cmd/
    python3 evolve-harness/evaluate.py --harness=000 --tasks="$TASKS" --binary="$BINARY" --search-db="$SEARCH_DB"
fi

# Search loop
for i in $(seq 1 $MAX_ITER); do
    ID=$(printf "%03d" $i)
    echo "=== Iteration $ID ==="

    # 1. Proposer
    run_proposer "$(($i - 1))" "$ID"

    # 2. Apply + build
    apply_harness "$ID"
    if ! go build -o "$BINARY" ./cmd/; then
        echo "Build failed for harness $ID — skipping"
        echo '{"error": "build_failed"}' > "$SEARCH_DB/harness_$ID/scores.json"
        # Revert
        apply_harness "000"
        continue
    fi

    # 3. Evaluate
    python3 evolve-harness/evaluate.py --harness="$ID" --tasks="$TASKS" --binary="$BINARY" --search-db="$SEARCH_DB"

    # 4. Revert source to baseline (proposer reads from search_db, not live source)
    apply_harness "000"

    # 5. Report
    echo "Harness $ID: $(python3 -c "import json; print(json.dumps(json.load(open('$SEARCH_DB/harness_$ID/scores.json')), indent=2))")"
done

# Summary
echo "=== Search Complete ==="
python3 -c "
import json, glob, os
scores = []
for f in sorted(glob.glob('$SEARCH_DB/*/scores.json')):
    s = json.load(open(f))
    if 'composite' in s:
        hid = os.path.basename(os.path.dirname(f))
        scores.append((s['composite'], hid, s))
scores.sort(reverse=True)
for composite, hid, s in scores[:5]:
    print(f'{hid}: composite={composite:.3f} coding={s[\"coding_pass_rate\"]:.2f} tools={s[\"tool_precision\"]:.2f} compression={s[\"compression_retention\"]:.2f}')
"
```

### Tests

```bash
# test_run.sh — verify snapshot/apply roundtrip
snapshot_harness test_000
apply_harness test_000
go build ./cmd/  # must succeed
diff internal/core/compression.go evolve-harness/search_db/harness_test_000/src/compression.go  # must match
```

---

## Step 5: End-to-End Integration

**Goal:** Run the full pipeline once to validate everything works together.

### Checklist

- [ ] Build baseline binary
- [ ] Run evaluator on 10 tasks (5 coding + 3 tool-use + 2 compression)
- [ ] Verify scores.json is correct
- [ ] Verify traces are readable and contain expected events
- [ ] Run proposer once — verify it reads search_db/ and writes a valid harness
- [ ] Build with proposer's modifications — verify it compiles
- [ ] Re-evaluate — verify new scores.json
- [ ] Run orchestrator for 2 iterations — verify full loop works
- [ ] Review proposer's hypothesis.md — verify it shows causal reasoning

### Minimal Viable Run (MVP)

Start with just 10 tasks (5 coding + 3 tool-use + 2 compression) and 3 iterations. This validates the full pipeline end-to-end before scaling to 55 tasks and 10 iterations.

---

## Implementation Order

| Step | What | Depends On | Est. Files |
|------|------|-----------|------------|
| 1A   | 10 coding tasks (easy) | Step 0 | 30 files |
| 1B   | 5 tool-use tasks | Step 0 | 15 files |
| 1C   | 2 compression tasks | Multi-turn batch extension | 6 files |
| 2    | evaluate.py | Steps 1A-C | 2 files |
| 1D   | Multi-turn batch extension | Step 0 | 1 file edit |
| 3    | proposer.md | Step 2 | 1 file |
| 4    | run.sh | Steps 2-3 | 1 file |
| 5    | End-to-end test | Steps 1-4 | validation only |
| 1E   | Remaining 40 tasks | Step 5 validated | 120 files |

Start with the MVP path: 1A(10) → 1B(5) → 2 → 3 → 4 → 5 → then expand tasks.
