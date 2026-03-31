#!/usr/bin/env python3
"""Evaluate a harness against the evolve-harness task suite.

Runs the agent binary against coding, tool-use, and compression tasks,
then computes per-dimension scores and a weighted composite.

Usage:
    python3 evaluate.py --harness=000 --tasks=evolve-harness/tasks \
        --binary=./torus-agent --search-db=evolve-harness/search_db
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Evaluate harness against task suite")
    parser.add_argument("--harness", required=True, help="Harness ID (e.g. 000)")
    parser.add_argument("--tasks", required=True, help="Path to tasks directory")
    parser.add_argument("--binary", default="./torus-agent", help="Agent binary path")
    parser.add_argument("--search-db", default="evolve-harness/search_db",
                        help="Search database directory")
    parser.add_argument("--baseline-tpp", type=float, default=0,
                        help="Baseline tokens_per_pass for efficiency scoring")
    parser.add_argument("--timeout", type=int, default=300,
                        help="Per-task timeout in seconds")
    args = parser.parse_args()

    harness_dir = os.path.join(args.search_db, f"harness_{args.harness}")
    traces_dir = os.path.join(harness_dir, "traces")
    os.makedirs(traces_dir, exist_ok=True)

    print(f"=== Evaluating harness {args.harness} ===")

    coding_dir = os.path.join(args.tasks, "coding")
    tool_dir = os.path.join(args.tasks, "tool_use")
    compression_dir = os.path.join(args.tasks, "compression")

    coding = evaluate_coding_tasks(args.binary, coding_dir, traces_dir, args.timeout)
    tools = evaluate_tool_tasks(args.binary, tool_dir, traces_dir, args.timeout)
    compression = evaluate_compression_tasks(
        args.binary, compression_dir, traces_dir, args.timeout
    )

    scores = compute_scores(coding, tools, compression, args.baseline_tpp)
    write_results(harness_dir, scores, coding, tools, compression)

    print(f"  coding_pass_rate:       {scores['coding_pass_rate']:.3f}")
    print(f"  tokens_per_pass:        {scores['tokens_per_pass']:.0f}")
    print(f"  efficiency_score:       {scores['efficiency_score']:.3f}")
    print(f"  tool_precision:         {scores['tool_precision']:.3f}")
    print(f"  compression_retention:  {scores['compression_retention']:.3f}")
    print(f"  composite:              {scores['composite']:.3f}")


def run_agent(binary, prompt_file, output_dir, workdir=None, extra_flags=None,
              timeout=300):
    """Run the agent binary in batch mode. Returns result.json as dict or None."""
    output_dir = os.path.abspath(output_dir)
    os.makedirs(output_dir, exist_ok=True)

    cmd = [binary, "--no-setup", f"--batch={prompt_file}", f"--output={output_dir}"]
    if workdir:
        cmd.append(f"--workdir={workdir}")
    if extra_flags:
        cmd.extend(extra_flags)

    try:
        subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        print(f"    TIMEOUT after {timeout}s")
        return None
    except FileNotFoundError:
        print(f"    Binary not found: {binary}")
        return None

    result_path = os.path.join(output_dir, "result.json")
    if os.path.exists(result_path):
        with open(result_path) as f:
            return json.load(f)
    return None


def evaluate_coding_tasks(binary, tasks_dir, traces_dir, timeout):
    """Evaluate coding tasks. Returns list of per-task result dicts."""
    results = []
    if not os.path.isdir(tasks_dir):
        print(f"  No coding tasks directory: {tasks_dir}")
        return results

    for task_name in sorted(os.listdir(tasks_dir)):
        task_path = os.path.join(tasks_dir, task_name)
        if not os.path.isdir(task_path):
            continue

        prompt_file = os.path.join(task_path, "prompt.txt")
        test_script = os.path.join(task_path, "test.sh")
        workspace = os.path.join(task_path, "workspace")

        if not os.path.exists(prompt_file):
            continue

        print(f"  coding/{task_name}...")

        # Copy workspace to temp dir so agent doesn't modify originals
        tmpdir = tempfile.mkdtemp(prefix=f"evolve_{task_name}_")
        if os.path.isdir(workspace):
            shutil.copytree(workspace, tmpdir, dirs_exist_ok=True)

        # Copy test.sh into the temp workspace
        if os.path.exists(test_script):
            shutil.copy2(test_script, os.path.join(tmpdir, "test.sh"))

        trace_out = os.path.join(traces_dir, f"coding_{task_name}")
        result = run_agent(binary, prompt_file, trace_out, workdir=tmpdir,
                           timeout=timeout)

        # Run test.sh to verify pass/fail
        passed = False
        if os.path.exists(test_script):
            try:
                test_result = subprocess.run(
                    ["bash", test_script], capture_output=True, text=True,
                    timeout=30, cwd=tmpdir
                )
                passed = test_result.returncode == 0
            except (subprocess.TimeoutExpired, Exception):
                passed = False

        tokens_in = 0
        tokens_out = 0
        cost = 0.0
        duration_ms = 0
        if result:
            tokens_in = result.get("total_input_tokens", 0)
            tokens_out = result.get("total_output_tokens", 0)
            cost = result.get("total_cost", 0.0)
            duration_ms = result.get("duration_ms", 0)

        status = "PASS" if passed else "FAIL"
        print(f"    {status} (tokens: {tokens_in}in/{tokens_out}out)")

        results.append({
            "task": task_name,
            "passed": passed,
            "tokens_in": tokens_in,
            "tokens_out": tokens_out,
            "cost": cost,
            "duration_ms": duration_ms,
        })

        shutil.rmtree(tmpdir, ignore_errors=True)

    return results


def evaluate_tool_tasks(binary, tasks_dir, traces_dir, timeout):
    """Evaluate tool-use tasks. Returns list of per-task result dicts."""
    results = []
    if not os.path.isdir(tasks_dir):
        print(f"  No tool-use tasks directory: {tasks_dir}")
        return results

    for task_name in sorted(os.listdir(tasks_dir)):
        task_path = os.path.join(tasks_dir, task_name)
        if not os.path.isdir(task_path):
            continue

        prompt_file = os.path.join(task_path, "prompt.txt")
        verify_script = os.path.join(task_path, "verify.py")
        workspace = os.path.join(task_path, "workspace")

        if not os.path.exists(prompt_file):
            continue

        print(f"  tool_use/{task_name}...")

        tmpdir = tempfile.mkdtemp(prefix=f"evolve_{task_name}_")
        if os.path.isdir(workspace):
            shutil.copytree(workspace, tmpdir, dirs_exist_ok=True)

        trace_out = os.path.join(traces_dir, f"tool_{task_name}")
        result = run_agent(binary, prompt_file, trace_out, workdir=tmpdir,
                           timeout=timeout)

        # Run verify.py on result.json
        tool_score = 0.0
        max_score = 2.0
        passed = False
        details = []

        result_json_path = os.path.join(trace_out, "result.json")
        if os.path.exists(verify_script) and os.path.exists(result_json_path):
            try:
                verify_result = subprocess.run(
                    ["python3", verify_script],
                    capture_output=True, text=True, timeout=30,
                    cwd=trace_out
                )
                if verify_result.stdout.strip():
                    vr = json.loads(verify_result.stdout)
                    tool_score = vr.get("score", 0.0)
                    max_score = vr.get("max_score", 2.0)
                    passed = vr.get("passed", False)
                    details = vr.get("details", [])
            except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception) as e:
                details = [f"verify.py error: {e}"]

        status = "PASS" if passed else "FAIL"
        print(f"    {status} (score: {tool_score}/{max_score})")

        cost = result.get("total_cost", 0.0) if result else 0.0

        results.append({
            "task": task_name,
            "passed": passed,
            "tool_score": tool_score,
            "max_score": max_score,
            "details": details,
            "cost": cost,
        })

        shutil.rmtree(tmpdir, ignore_errors=True)

    return results


def evaluate_compression_tasks(binary, tasks_dir, traces_dir, timeout):
    """Evaluate compression retention. Returns list of per-task result dicts."""
    results = []
    if not os.path.isdir(tasks_dir):
        print(f"  No compression tasks directory: {tasks_dir}")
        return results

    for task_name in sorted(os.listdir(tasks_dir)):
        task_path = os.path.join(tasks_dir, task_name)
        if not os.path.isdir(task_path):
            continue

        convo_file = os.path.join(task_path, "conversation.json")
        queries_file = os.path.join(task_path, "queries.json")

        if not os.path.exists(convo_file) or not os.path.exists(queries_file):
            continue

        print(f"  compression/{task_name}...")

        trace_out = os.path.join(traces_dir, f"compression_{task_name}")

        # Feed conversation via multi-turn batch with small context window
        result = run_agent(
            binary, convo_file, trace_out,
            extra_flags=["--multi-turn", "--context-window=8000"],
            timeout=timeout
        )

        with open(queries_file) as f:
            queries = json.load(f)

        keywords_found = 0
        keywords_total = 0

        for qi, query in enumerate(queries):
            question = query["question"]
            expected_keywords = query["keywords"]
            keywords_total += len(expected_keywords)

            # Write query to temp file and run agent
            query_tmpfile = os.path.join(trace_out, f"query_{qi}.txt")
            os.makedirs(os.path.dirname(query_tmpfile), exist_ok=True)
            with open(query_tmpfile, "w") as f:
                f.write(question)

            query_trace = os.path.join(trace_out, f"query_{qi}")
            query_result = run_agent(binary, query_tmpfile, query_trace, timeout=60)

            response_text = query_result.get("response", "") if query_result else ""
            found = check_keywords(response_text, expected_keywords)
            keywords_found += found
            print(f"    query {qi}: {found}/{len(expected_keywords)} keywords")

        results.append({
            "task": task_name,
            "keywords_found": keywords_found,
            "keywords_total": keywords_total,
            "cost": result.get("total_cost", 0.0) if result else 0.0,
        })

    return results


def check_keywords(response_text, keywords):
    """Case-insensitive substring match. Returns count of keywords found."""
    found = 0
    lower = response_text.lower()
    for kw in keywords:
        if kw.lower() in lower:
            found += 1
    return found


def compute_scores(coding, tools, compression, baseline_tpp):
    """Compute all metrics and weighted composite score."""
    coding_total = max(len(coding), 1)
    coding_passed = sum(1 for c in coding if c["passed"])
    coding_pass_rate = coding_passed / coding_total

    tasks_passed = max(coding_passed, 1)
    total_input_tokens = sum(c["tokens_in"] for c in coding)
    total_output_tokens = sum(c.get("tokens_out", 0) for c in coding)
    tokens_per_pass = total_input_tokens / tasks_passed

    if baseline_tpp > 0:
        efficiency_score = min(baseline_tpp / max(tokens_per_pass, 1), 1.0)
    else:
        efficiency_score = 1.0  # first run IS the baseline

    tool_score_sum = sum(t["tool_score"] for t in tools)
    tool_max_sum = max(sum(t.get("max_score", 2.0) for t in tools), 1)
    tool_precision = tool_score_sum / tool_max_sum

    kw_found = sum(c["keywords_found"] for c in compression)
    kw_total = max(sum(c["keywords_total"] for c in compression), 1)
    compression_retention = kw_found / kw_total

    composite = (
        0.35 * coding_pass_rate
        + 0.20 * efficiency_score
        + 0.25 * tool_precision
        + 0.20 * compression_retention
    )

    total_cost = (
        sum(c.get("cost", 0) for c in coding)
        + sum(t.get("cost", 0) for t in tools)
        + sum(c.get("cost", 0) for c in compression)
    )

    return {
        "coding_pass_rate": coding_pass_rate,
        "coding_passed": coding_passed,
        "coding_total": coding_total,
        "tokens_per_pass": tokens_per_pass,
        "efficiency_score": efficiency_score,
        "tool_precision": tool_precision,
        "compression_retention": compression_retention,
        "composite": composite,
        "total_input_tokens": total_input_tokens,
        "total_output_tokens": total_output_tokens,
        "total_cost": total_cost,
    }


def write_results(harness_dir, scores, coding, tools, compression):
    """Write scores.json and detailed_results.json."""
    with open(os.path.join(harness_dir, "scores.json"), "w") as f:
        json.dump(scores, f, indent=2)

    detailed = {
        "coding": coding,
        "tool_use": tools,
        "compression": compression,
    }
    with open(os.path.join(harness_dir, "detailed_results.json"), "w") as f:
        json.dump(detailed, f, indent=2)


if __name__ == "__main__":
    main()
