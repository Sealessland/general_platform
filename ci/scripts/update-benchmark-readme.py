#!/usr/bin/env python3
"""Parse Go benchmark output and update the performance table in README.md."""

import re
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
README = ROOT / "README.md"

BENCH_RE = re.compile(
    r"^(Benchmark\w+)-\d+\s+(\d+)\s+([\d.]+)\s+ns/op\s+([\d.]+)\s+B/op\s+([\d.]+)\s+allocs/op"
)


def format_qps(ns_per_op: float) -> str:
    qps = 1_000_000_000.0 / ns_per_op
    if qps >= 1_000_000:
        return f"{qps / 1_000_000:.2f}M"
    if qps >= 1_000:
        return f"{qps / 1_000:.2f}K"
    return f"{qps:.2f}"


def parse_benchmarks(text: str) -> list[dict]:
    results = []
    for line in text.splitlines():
        match = BENCH_RE.match(line)
        if not match:
            continue
        name, _, ns, b, allocs = match.groups()
        results.append(
            {
                "name": name,
                "qps": format_qps(float(ns)),
                "ns": ns,
                "b": b,
                "allocs": allocs,
            }
        )
    return results


def render_table(results: list[dict]) -> str:
    if not results:
        return "_No benchmark results available._"
    lines = [
        "## Performance",
        "",
        f"_Last updated: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')} via GitHub Actions._",
        "",
        "| Benchmark | QPS | ns/op | B/op | allocs/op |",
        "|---|---|---|---|---|",
    ]
    for r in results:
        lines.append(f"| `{r['name']}` | {r['qps']} | {r['ns']} | {r['b']} | {r['allocs']} |")
    return "\n".join(lines)


def update_readme(table_markdown: str) -> None:
    start_marker = "<!-- BENCHMARK_RESULTS_START -->\n"
    end_marker = "\n<!-- BENCHMARK_RESULTS_END -->"

    content = README.read_text(encoding="utf-8")
    if start_marker not in content:
        content = content.rstrip() + "\n\n" + start_marker + end_marker + "\n"

    before, rest = content.split(start_marker, 1)
    _, after = rest.split(end_marker, 1)
    new_content = before + start_marker + table_markdown + end_marker + after

    if new_content != content:
        README.write_text(new_content, encoding="utf-8")
        print(f"Updated {README}")
    else:
        print("README already up to date")


def main() -> int:
    raw = sys.stdin.read()
    results = parse_benchmarks(raw)
    if not results:
        print("No benchmark results found in input", file=sys.stderr)
        return 1
    update_readme(render_table(results))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
