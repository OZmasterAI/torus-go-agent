#!/usr/bin/env python3
"""Verify tool-use patterns for 002_search_codebase.

Checks:
  - Required tools: grep (must use grep to search for TODO comments)
  - Banned tools: bash (should not shell out to grep/find/rg)
  - Score: 1.0 for using grep, +0.5 bonus for not using bash, -0.5 per banned tool
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
    required = ["grep"]
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
            # Check if bash was used for grep/find/rg (the banned pattern)
            bash_events = [
                e for e in trace
                if e.get("type") == "tool_start"
                and e.get("tool_name") == "bash"
            ]
            grep_in_bash = False
            for evt in bash_events:
                args = evt.get("tool_args", {})
                cmd = args.get("command", "")
                if any(kw in cmd for kw in ["grep", "find", "rg ", "ag "]):
                    grep_in_bash = True
                    break

            if grep_in_bash:
                score -= 0.5
                details.append(
                    f"PENALTY: used bash to run grep/find "
                    f"(should use the grep tool directly)"
                )
            else:
                details.append(
                    f"INFO: bash used but not for searching — no penalty"
                )
        else:
            score += 0.5
            details.append(f"PASS: correctly avoided '{tool}' tool")

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
