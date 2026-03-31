#!/bin/bash
set -euo pipefail

SEARCH_DB="evolve-harness/search_db"
TASKS="evolve-harness/tasks"
MAX_ITER=${MAX_ITER:-10}
BINARY=${BINARY:-./torus-agent}
EVAL_FLAGS=${EVAL_FLAGS:-""}

snapshot_harness() {
    local ID=$1
    mkdir -p "$SEARCH_DB/harness_$ID/src/"{core,providers,tools,features}
    cp internal/core/compression.go internal/core/compression_ops.go internal/core/context.go \
       "$SEARCH_DB/harness_$ID/src/core/"
    cp internal/providers/anthropic.go "$SEARCH_DB/harness_$ID/src/providers/"
    cp internal/tools/tools.go "$SEARCH_DB/harness_$ID/src/tools/"
    cp internal/features/routing.go "$SEARCH_DB/harness_$ID/src/features/"
}

apply_harness() {
    local ID=$1
    cp "$SEARCH_DB/harness_$ID/src/core/"*.go internal/core/
    cp "$SEARCH_DB/harness_$ID/src/providers/"*.go internal/providers/
    cp "$SEARCH_DB/harness_$ID/src/tools/"*.go internal/tools/
    cp "$SEARCH_DB/harness_$ID/src/features/"*.go internal/features/
}

report_scores() {
    python3 -c "
import sys, json
s = json.load(sys.stdin)
if 'error' in s:
    print(f'  ERROR: {s[\"error\"]}')
else:
    print(f'  composite={s[\"composite\"]:.3f} coding={s[\"coding_pass_rate\"]:.2f} efficiency={s[\"efficiency_score\"]:.2f} tools={s[\"tool_precision\"]:.2f} compression={s[\"compression_retention\"]:.2f}')
" < "$1"
}

# === Baseline ===
if [ ! -f "$SEARCH_DB/harness_000/scores.json" ]; then
    echo "=== Evaluating baseline ==="
    snapshot_harness 000
    go build -o "$BINARY" ./cmd/
    python3 evolve-harness/evaluate.py \
        --harness=000 --tasks="$TASKS" --binary="$BINARY" --search-db="$SEARCH_DB" $EVAL_FLAGS
    echo -n "Baseline: "
    report_scores "$SEARCH_DB/harness_000/scores.json"
fi

# Get baseline tokens_per_pass for efficiency scoring
BASELINE_TPP=$(python3 -c "import json; print(json.load(open('$SEARCH_DB/harness_000/scores.json'))['tokens_per_pass'])")

# === Search Loop ===
for i in $(seq 1 $MAX_ITER); do
    ID=$(printf "%03d" $i)
    echo ""
    echo "=== Iteration $ID ==="

    # 1. Run proposer
    echo "Running proposer..."
    claude -p --model claude-sonnet-4-6 "$(cat evolve-harness/proposer.md)

Current iteration: $ID
Search database: $SEARCH_DB/
Write new harness to: $SEARCH_DB/harness_$ID/src/

Read the search_db to understand what's been tried. Propose your modification." \
        --allowedTools Read,Write,Edit,Grep,Glob

    # 2. Apply + build
    if [ ! -d "$SEARCH_DB/harness_$ID/src" ]; then
        echo "Proposer didn't write files — skipping"
        continue
    fi

    apply_harness "$ID"
    if ! go build -o "$BINARY" ./cmd/ 2>"$SEARCH_DB/harness_$ID/build_error.txt"; then
        echo "Build failed — see $SEARCH_DB/harness_$ID/build_error.txt"
        echo '{"error": "build_failed", "composite": 0}' > "$SEARCH_DB/harness_$ID/scores.json"
        apply_harness "000"
        continue
    fi

    # 3. Evaluate
    echo "Evaluating..."
    python3 evolve-harness/evaluate.py \
        --harness="$ID" --tasks="$TASKS" --binary="$BINARY" \
        --search-db="$SEARCH_DB" --baseline-tpp="$BASELINE_TPP" $EVAL_FLAGS

    # 4. Revert to baseline source
    apply_harness "000"

    # 5. Report
    echo -n "Harness $ID: "
    report_scores "$SEARCH_DB/harness_$ID/scores.json"
done

# === Summary ===
echo ""
echo "=== Search Complete ==="
python3 -c "
import json, glob, os
scores = []
for f in sorted(glob.glob('$SEARCH_DB/*/scores.json')):
    s = json.load(open(f))
    if 'composite' in s and s['composite'] > 0:
        hid = os.path.basename(os.path.dirname(f))
        scores.append((s['composite'], hid, s))
scores.sort(reverse=True)
print('Top 5 harnesses:')
for composite, hid, s in scores[:5]:
    print(f'  {hid}: composite={composite:.3f}  coding={s[\"coding_pass_rate\"]:.2f}  efficiency={s[\"efficiency_score\"]:.2f}  tools={s[\"tool_precision\"]:.2f}  compression={s[\"compression_retention\"]:.2f}')
if scores:
    best_id = scores[0][1]
    print(f'\nBest: {best_id}')
    print(f'Apply with: ./evolve-harness/run.sh apply {best_id.split(\"_\")[1]}')
else:
    print('No valid harnesses found.')
"
