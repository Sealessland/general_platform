#!/usr/bin/env python3
"""Project-scoped Codex hook for RedCart agent handoffs.

The hook intentionally stays narrow:
- PreToolUse checks shell commands follow the local `rtk` rule and blocks
  obviously destructive git/worktree operations.
- Stop runs a quick handoff gate so agents do not finish after structural
  test splits without preserving test entrypoints or running workspace checks.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TEST_NAME_RE = re.compile(r"^func\s+((?:Test|Benchmark)[A-Za-z0-9_]*)\s*\(", re.MULTILINE)


def emit(payload: dict[str, object]) -> None:
    print(json.dumps(payload, ensure_ascii=False))


def deny(reason: str) -> None:
    emit(
        {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": reason,
            }
        }
    )


def stop(reason: str, details: list[str]) -> None:
    message = reason
    if details:
        message += "\n\n" + "\n".join(f"- {line}" for line in details)
    emit({"decision": "block", "reason": message})


def command_text(data: dict[str, object]) -> str:
    tool_input = data.get("tool_input")
    if not isinstance(tool_input, dict):
        return ""
    for key in ("command", "cmd"):
        value = tool_input.get(key)
        if isinstance(value, str):
            return value
    return ""


def check_pre_tool_use(data: dict[str, object]) -> int:
    if data.get("tool_name") != "Bash":
        return 0

    command = command_text(data).strip()
    if not command:
        return 0

    destructive_patterns = [
        r"\bgit\s+reset\s+--hard\b",
        r"\bgit\s+checkout\s+--\b",
        r"\bgit\s+clean\s+-[^\s]*[fd]",
        r"\brm\s+-[^\s]*r[^\n]*(?:\s|/)(?:\.git|backend|frontend|ai-service|docs)(?:\s|/|$)",
        r"\bdocker\s+compose\s+down\b[^\n]*\s-v\b",
    ]
    for pattern in destructive_patterns:
        if re.search(pattern, command):
            deny(f"Blocked by RedCart project hook: high-risk destructive command: {command}")
            return 0

    first_word = ""
    try:
        first_word = shlex.split(command, posix=True)[0] if command else ""
    except ValueError:
        first_word = command.split(maxsplit=1)[0]

    allowed_prefixes = ("rtk", "/usr/bin/rtk")
    if first_word not in allowed_prefixes:
        deny("RedCart workspace commands must be prefixed with `rtk` per AGENTS.md.")
        return 0

    return 0


def run(args: list[str], *, cwd: Path = ROOT, env: dict[str, str] | None = None) -> tuple[bool, str]:
    merged_env = os.environ.copy()
    if env:
        merged_env.update(env)
    try:
        proc = subprocess.run(
            args,
            cwd=str(cwd),
            env=merged_env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=220,
        )
    except subprocess.TimeoutExpired:
        return False, f"{' '.join(args)} timed out"
    output = proc.stdout.strip()
    if proc.returncode != 0:
        return False, f"{' '.join(args)} failed\n{output}"
    return True, output


def rtk_args(args: list[str]) -> list[str]:
    if shutil.which("rtk"):
        return ["rtk", *args]
    return args


def changed_paths() -> list[str]:
    ok, output = run(rtk_args(["git", "status", "--porcelain"]))
    if not ok:
        return []
    paths: list[str] = []
    for line in output.splitlines():
        if not line:
            continue
        path = line[3:]
        if " -> " in path:
            path = path.split(" -> ", 1)[1]
        paths.append(path)
    return paths


def head_file(path: str) -> str:
    ok, output = run(rtk_args(["git", "show", f"HEAD:{path}"]))
    return output if ok else ""


def package_test_names(package_dir: Path) -> set[str]:
    names: set[str] = set()
    for path in package_dir.glob("*_test.go"):
        names.update(TEST_NAME_RE.findall(path.read_text()))
    return names


def check_test_entrypoints(paths: list[str]) -> list[str]:
    failures: list[str] = []
    package_cache: dict[Path, set[str]] = {}
    for path in paths:
        if not path.endswith("_test.go"):
            continue
        before = head_file(path)
        if not before:
            continue
        previous_names = set(TEST_NAME_RE.findall(before))
        if not previous_names:
            continue
        package_dir = (ROOT / path).parent
        current_names = package_cache.setdefault(package_dir, package_test_names(package_dir))
        missing = sorted(previous_names - current_names)
        if missing:
            failures.append(f"{path}: missing test/benchmark entrypoints after changes: {', '.join(missing)}")
    return failures


def require_docs_for_code_changes(paths: list[str]) -> list[str]:
    code_changed = any(
        path.endswith((".go", ".ts", ".mjs", ".py", ".yaml", ".yml", ".sh"))
        and not path.startswith(".codex/hooks/")
        for path in paths
    )
    if not code_changed:
        return []

    docs_changed = {"AI_WORKFLOW.md", "CHANGELOG.md"} & set(paths)
    if docs_changed:
        return []
    return ["code or workflow files changed without updating AI_WORKFLOW.md or CHANGELOG.md"]


def quick_commands(paths: list[str]) -> list[tuple[list[str], Path, dict[str, str] | None]]:
    commands: list[tuple[list[str], Path, dict[str, str] | None]] = [
        (rtk_args(["git", "diff", "--check"]), ROOT, None),
        (rtk_args(["bash", "scripts/validate-workspace.sh"]), ROOT, None),
        (rtk_args(["bash", "scripts/check-openapi.sh"]), ROOT, None),
    ]
    if any(path.startswith("backend/") for path in paths):
        commands.append(
            (
                rtk_args(["env", "GOCACHE=/tmp/go-build-cache", "go", "test", "./..."]),
                ROOT / "backend",
                None,
            )
        )
    if any(path.startswith("frontend/") for path in paths):
        commands.append((rtk_args(["bash", "ci/scripts/frontend-ci.sh"]), ROOT, None))
    if any(path.startswith("ai-service/") for path in paths):
        commands.append((rtk_args(["bash", "ci/scripts/ai-service-ci.sh"]), ROOT, None))
    return commands


def full_commands() -> list[tuple[list[str], Path, dict[str, str] | None]]:
    pg_env = {
        "POSTGRES_DSN": os.environ.get(
            "POSTGRES_DSN",
            "postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable",
        ),
        "RUN_POSTGRES_INTEGRATION": "1",
        "POSTGRES_BENCHTIME": os.environ.get("POSTGRES_BENCHTIME", "1s"),
        "GOCACHE": os.environ.get("GOCACHE", "/tmp/go-build-cache"),
    }
    return [
        (rtk_args(["bash", "ci/scripts/backend-ci.sh"]), ROOT, pg_env),
        (rtk_args(["bash", "ci/scripts/frontend-ci.sh"]), ROOT, None),
        (rtk_args(["bash", "ci/scripts/ai-service-ci.sh"]), ROOT, None),
        (rtk_args(["bash", "ci/scripts/security-ci.sh"]), ROOT, None),
        (rtk_args(["bash", "ci/scripts/check-openapi.sh"]), ROOT, None),
        (rtk_args(["bash", "ci/scripts/validate-workspace.sh"]), ROOT, None),
    ]


def check_stop(mode: str) -> int:
    paths = changed_paths()
    failures = check_test_entrypoints(paths)
    failures.extend(require_docs_for_code_changes(paths))

    commands = full_commands() if mode == "full" else quick_commands(paths)
    for args, cwd, env in commands:
        ok, output = run(args, cwd=cwd, env=env)
        if not ok:
            failures.append(output)

    if failures:
        stop("RedCart project hook found unfinished handoff work.", failures)
    return 0


def self_test() -> int:
    sample = {
        "hook_event_name": "PreToolUse",
        "tool_name": "Bash",
        "tool_input": {"command": "go test ./..."},
    }
    if command_text(sample) != "go test ./...":
        print("self-test failed: command_text", file=sys.stderr)
        return 1
    names = set(TEST_NAME_RE.findall("func TestExample(t *testing.T) {}\nfunc BenchmarkX(b *testing.B) {}\n"))
    if names != {"TestExample", "BenchmarkX"}:
        print("self-test failed: TEST_NAME_RE", file=sys.stderr)
        return 1
    print("redcart project hook self-test passed")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("quick", "full"), default="quick")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()

    if args.self_test:
        return self_test()

    raw = sys.stdin.read().strip()
    data: dict[str, object] = {}
    if raw:
        try:
            parsed = json.loads(raw)
            if isinstance(parsed, dict):
                data = parsed
        except json.JSONDecodeError as exc:
            stop("RedCart project hook received invalid JSON input.", [str(exc)])
            return 0

    event = data.get("hook_event_name")
    if event == "PreToolUse":
        return check_pre_tool_use(data)
    if event == "Stop" or not raw:
        return check_stop(args.mode)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
