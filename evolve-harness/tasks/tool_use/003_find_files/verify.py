#!/usr/bin/env python3
"""Verify tool-use patterns for 003_find_files.

Checks:
  - Required tools: glob (must use glob to find files by pattern)
  - Banned tools: bash (should not shell out to find/ls/fd)
  - Score: 1.0 for using glob, +0.5 bonus for not using bash, -0.5 per banned tool
"""
import json
import sys


def main():
    try:
        result = json.load(open("result.json"))
    except (FileNotFoundError, json.JSONDecodeError) as exc:
        print(json.dumps({
            "score": 0,
            "max_score": 1.5,
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
    max_score = 1.5
    details = []

    # --- Required tools (1.0 each) ---
    required = ["glob"]
    for tool in required:
        if tool in tools_used:
            score += 1.0
            details.append(f"PASS: used required tool '{tool}'")
        else:
            details.append(f"FAIL: missing required tool '{tool}'")

    # --- Banned tools (-0.5 each) ---
    banned = ["bash"]
    for tool in banned:
        if tool in tools_used:
            # Check if bash was used for find/ls (the banned pattern)
            bash_events = [
                e for e in trace
                if e.get("type") == "tool_start"
                and e.get("tool_name") == "bash"
            ]
            find_in_bash = False
            for evt in bash_events:
                args = evt.get("tool_args", {})
                cmd = args.get("command", "")
                if any(kw in cmd for kw in ["find ", "ls ", "fd ", "locate "]):
                    find_in_bash = True
                    break

            if find_in_bash:
                score -= 0.5
                details.append(
                    f"PENALTY: used bash to run find/ls "
                    f"(should use the glob tool directly)"
                )
            else:
                details.append(
                    f"INFO: bash used but not for file-finding — no penalty"
                )
        else:
            score += 0.5
            details.append(f"PASS: correctly avoided '{tool}' tool")

    # --- Bonus: check if the glob pattern targeted test files ---
    glob_events = [
        e for e in trace
        if e.get("type") == "tool_start"
        and e.get("tool_name") == "glob"
    ]
    for evt in glob_events:
        args = evt.get("tool_args", {})
        pattern = args.get("pattern", "")
        if "_test.go" in pattern or "*_test*" in pattern or "test" in pattern.lower():
            details.append(f"INFO: glob pattern targeted test files: '{pattern}'")
            break

    result_out = {
        "score": max(score, 0),
        "max_score": max_score,
        "passed": score >= max_score * 0.5,
        "tools_used": tools_used,
        "details": details,
    }

    json.dump(result_out, sys.stdout, indent=2)
    print()
    sys.exit(0 if result_out["passed"] else 1)


if __name__ == "__main__":
    main()
