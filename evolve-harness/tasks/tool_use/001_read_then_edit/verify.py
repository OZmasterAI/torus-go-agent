#!/usr/bin/env python3
"""Verify tool-use patterns for 001_read_then_edit.

Checks:
  - Required tools: read, edit (both must appear in trace)
  - Banned tools: write (must NOT appear — edit is the correct tool)
  - Tool order: read must appear before edit
  - Score: 2.0 max (1.0 per required tool), -0.5 per banned tool, +0.5 for correct order
"""
import json
import sys


def main():
    try:
        result = json.load(open("result.json"))
    except (FileNotFoundError, json.JSONDecodeError) as exc:
        print(json.dumps({
            "score": 0,
            "max_score": 2.5,
            "passed": False,
            "tools_used": [],
            "details": [f"ERROR: could not load result.json: {exc}"],
        }, indent=2))
        sys.exit(1)

    trace = result.get("trace", [])

    # Collect ordered list of tool names from trace
    tools_used = []
    for event in trace:
        if event.get("type") == "tool_start":
            name = event.get("tool_name", "")
            if name:
                tools_used.append(name)

    score = 0.0
    max_score = 2.5
    details = []

    # --- Required tools (1.0 each) ---
    required = ["read", "edit"]
    for tool in required:
        if tool in tools_used:
            score += 1.0
            details.append(f"PASS: used required tool '{tool}'")
        else:
            details.append(f"FAIL: missing required tool '{tool}'")

    # --- Banned tools (-0.5 each) ---
    banned = ["write"]
    for tool in banned:
        if tool in tools_used:
            score -= 0.5
            details.append(f"PENALTY: used banned tool '{tool}' (should use edit, not write)")
        else:
            details.append(f"PASS: correctly avoided banned tool '{tool}'")

    # --- Tool order: read before edit (+0.5) ---
    read_indices = [i for i, t in enumerate(tools_used) if t == "read"]
    edit_indices = [i for i, t in enumerate(tools_used) if t == "edit"]
    if read_indices and edit_indices:
        if min(read_indices) < min(edit_indices):
            score += 0.5
            details.append("PASS: read appeared before edit (correct order)")
        else:
            details.append("FAIL: edit appeared before read (should read first)")
    else:
        details.append("SKIP: order check — missing read or edit")

    result_out = {
        "score": max(score, 0),
        "max_score": max_score,
        "passed": score >= max_score * 0.5,
        "tools_used": tools_used,
        "details": details,
    }

    json.dump(result_out, sys.stdout, indent=2)
    print()  # trailing newline
    sys.exit(0 if result_out["passed"] else 1)


if __name__ == "__main__":
    main()
