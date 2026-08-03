#!/usr/bin/env python3
"""Parse Go benchmark output and update the performance table in README.md."""

import re
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
README = ROOT / "README.md"

# 匹配 go test -bench 的输出行，例如:
# BenchmarkHTTPPostgresOrderPreview-12  100  2000000 ns/op  123 B/op  4 allocs/op
BENCH_RE = re.compile(
    r"^(Benchmark\w+)-\d+\s+(\d+)\s+([\d.]+)\s+ns/op\s+([\d.]+)\s+B/op\s+([\d.]+)\s+allocs/op"
)

# 白名单：只允许真实运行时/中间件依赖（PostgreSQL、RabbitMQ、live HTTP）的基准，
# 内存仓储、空 publisher 或模拟延迟 benchmark 不允许进入 README 性能表
ALLOWED_BENCHMARKS = {
    "BenchmarkHTTPPostgresOrderPreview",
    "BenchmarkHTTPPostgresCreateOrder",
    "BenchmarkHTTPPostgresCreateOrderWithOutbox",
    "BenchmarkPostgresRabbitMQOutboxRelay",
    "BenchmarkRabbitMQPublish",
    "BenchmarkLiveHTTPHealthz",
    "BenchmarkLiveHTTPOrderPreview",
    "BenchmarkLiveHTTPCreateOrder",
}


def format_qps(ns_per_op: float) -> str:
    # 将 ns/op 换算为 QPS，并按数量级格式化为 M/K 单位便于阅读
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
        # 白名单之外的一律拒绝，防止本地调试基准污染性能表
        if name not in ALLOWED_BENCHMARKS:
            raise ValueError(f"refusing non-runtime benchmark result: {name}")
        results.append(
            {
                "name": name,
                "qps": format_qps(float(ns)),
                "qps_raw": 1_000_000_000.0 / float(ns),
                "ns": ns,
                "b": b,
                "allocs": allocs,
            }
        )
    return results


def parse_existing_table(content: str) -> dict[str, float]:
    """Extract QPS numbers from the existing README performance table."""
    # 解析当前 README 中的性能表（| Benchmark | QPS | ... 与 | `xxx` | ... 行），
    # 用于计算与上次结果的环比变化
    old = {}
    in_table = False
    for line in content.splitlines():
        if line.startswith("| Benchmark"):
            in_table = True
            continue
        if in_table and line.startswith("| `"):
            parts = [p.strip() for p in line.split("|")]
            if len(parts) >= 3:
                name = parts[1].strip("` ")
                qps_str = parts[2].strip()
                old[name] = parse_qps(qps_str)
        elif in_table and not line.startswith("|"):
            break
    return old


def parse_qps(qps_str: str) -> float:
    qps_str = qps_str.replace(",", "")
    if qps_str.endswith("M"):
        return float(qps_str[:-1]) * 1_000_000
    if qps_str.endswith("K"):
        return float(qps_str[:-1]) * 1_000
    return float(qps_str)


def format_change(current: float, previous: float) -> str:
    if previous == 0:
        return "-"
    delta = (current - previous) / previous * 100
    if delta > 0:
        return f"+{delta:.1f}%"
    if delta < 0:
        return f"{delta:.1f}%"
    return "0.0%"


def render_table(results: list[dict]) -> str:
    if not results:
        return "_No benchmark results available._"

    old = parse_existing_table(README.read_text(encoding="utf-8"))

    lines = [
        "## Performance",
        "",
        f"_Last updated: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')} via GitHub Actions._",
        "",
        "| Benchmark | QPS | Change vs previous | ns/op | B/op | allocs/op |",
        "|---|---|---|---|---|---|",
    ]
    for r in results:
        change = format_change(r["qps_raw"], old.get(r["name"], 0.0))
        lines.append(
            f"| `{r['name']}` | {r['qps']} | {change} | {r['ns']} | {r['b']} | {r['allocs']} |"
        )

    lines.append("")
    lines.append(
        "_Only PostgreSQL/Redis/RabbitMQ-backed component benchmarks and live HTTP "
        "benchmarks from a running backend process are accepted._"
    )

    return "\n".join(lines)


def update_readme(table_markdown: str) -> None:
    # 用一对标记（BENCHMARK_RESULTS_START/END）定位并替换 README 中的性能表区块；
    # 若标记不存在则先在文件末尾初始化
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
    try:
        results = parse_benchmarks(raw)
        if not results:
            print("No benchmark results found in input", file=sys.stderr)
            return 1
        update_readme(render_table(results))
        return 0
    except ValueError as exc:
        print(str(exc), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
