#!/usr/bin/env python3
"""Generate a local branch/worktree status file for the primary workspace."""

from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
import textwrap
from datetime import datetime, timezone
from pathlib import Path

STATUS_FILE = "BRANCH_STATUS.local.md"
MAX_CONTEXT_CHARS = 16000


def run(args: list[str], cwd: Path) -> str:
    proc = subprocess.run(
        args,
        cwd=str(cwd),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"{' '.join(args)} failed\n{proc.stderr.strip()}")
    return proc.stdout


def maybe_run(
    args: list[str],
    cwd: Path,
    *,
    input_text: str | None = None,
    timeout: int = 120,
) -> tuple[bool, str]:
    proc = subprocess.run(
        args,
        cwd=str(cwd),
        text=True,
        input=input_text,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    output = (proc.stdout or proc.stderr).strip()
    return proc.returncode == 0, output


def tool_args(args: list[str]) -> list[str]:
    if shutil.which("rtk"):
        return ["rtk", *args]
    return args


def repo_root(cwd: Path) -> Path:
    return Path(run(["git", "rev-parse", "--show-toplevel"], cwd).strip())


def common_git_dir(cwd: Path) -> Path:
    raw = run(["git", "rev-parse", "--git-common-dir"], cwd).strip()
    path = Path(raw)
    if not path.is_absolute():
        path = (cwd / path).resolve()
    return path


def primary_root(cwd: Path) -> Path:
    return common_git_dir(cwd).parent


def ensure_local_exclude(cwd: Path, pattern: str) -> None:
    exclude_path = common_git_dir(cwd) / "info" / "exclude"
    existing = exclude_path.read_text(encoding="utf-8") if exclude_path.exists() else ""
    lines = [line.strip() for line in existing.splitlines()]
    if pattern in lines:
        return
    with exclude_path.open("a", encoding="utf-8") as handle:
        if existing and not existing.endswith("\n"):
            handle.write("\n")
        handle.write(pattern + "\n")


def worktrees(cwd: Path) -> list[dict[str, str]]:
    blocks = run(["git", "worktree", "list", "--porcelain"], cwd).strip().split("\n\n")
    out: list[dict[str, str]] = []
    for block in blocks:
        if not block.strip():
            continue
        entry: dict[str, str] = {}
        for line in block.splitlines():
            key, _, value = line.partition(" ")
            entry[key] = value
        out.append(entry)
    return out


def branch_rows(cwd: Path) -> list[dict[str, str]]:
    output = run(
        [
            "git",
            "for-each-ref",
            "--sort=-committerdate",
            "--format=%(refname:short)\t%(committerdate:short)\t%(objectname:short)\t%(subject)",
            "refs/heads",
        ],
        cwd,
    )
    rows: list[dict[str, str]] = []
    for line in output.splitlines():
        parts = line.split("\t", 3)
        if len(parts) != 4:
            continue
        rows.append(
            {
                "branch": parts[0],
                "date": parts[1],
                "sha": parts[2],
                "subject": parts[3],
            }
        )
    return rows


def worktree_status(path: Path) -> str:
    output = run(["git", "status", "--short"], path)
    lines = [line for line in output.splitlines() if line.strip()]
    if not lines:
        return "clean"
    return f"dirty ({len(lines)} paths)"


def current_branch(path: Path) -> str:
    return run(["git", "branch", "--show-current"], path).strip()


def changed_paths(path: Path) -> list[str]:
    output = run(["git", "status", "--short"], path)
    return [line[3:] for line in output.splitlines() if line.strip()]


def clip(text: str, limit: int = MAX_CONTEXT_CHARS) -> str:
    if len(text) <= limit:
        return text
    return text[:limit] + "\n...[truncated]..."


def git_block(path: Path, title: str, args: list[str]) -> str:
    ok, output = maybe_run(["git", *args], path)
    if not ok or not output:
        return ""
    return f"## {title}\n{clip(output.strip())}\n"


def build_context(path: Path, branch: str, status: str) -> str:
    lines = [
        f"# Branch: {branch}",
        f"# Path: {path}",
        f"# Status: {status}",
        "",
    ]
    if status == "clean":
        blocks = [
            git_block(path, "Latest Commit", ["show", "--stat", "--name-status", "--format=medium", "-1"]),
        ]
    else:
        blocks = [
            git_block(path, "Git Status", ["status", "--short"]),
            git_block(path, "Diff Stat", ["diff", "--stat"]),
            git_block(path, "Name Status", ["diff", "--name-status"]),
            git_block(path, "Staged Diff Stat", ["diff", "--cached", "--stat"]),
            git_block(path, "Staged Name Status", ["diff", "--cached", "--name-status"]),
            git_block(path, "Diff Patch", ["diff", "--no-color", "--unified=1"]),
            git_block(path, "Staged Diff Patch", ["diff", "--cached", "--no-color", "--unified=1"]),
        ]
    lines.extend(block for block in blocks if block)
    return "\n".join(lines).strip()


def fallback_outline(path: Path, branch: str, status: str) -> str:
    if status == "clean":
        latest = run(
            ["git", "log", "-1", "--pretty=format:%h %s (%cs)"],
            path,
        ).strip()
        return "\n".join(
            [
                f"- 分支：`{branch}`",
                "- 当前工作区干净",
                f"- 最近提交：`{latest}`",
            ]
        )

    paths = changed_paths(path)
    diff_stat = run(["git", "diff", "--stat"], path).strip()
    bullets = [
        f"- 分支：`{branch}`",
        f"- 当前工作区有未提交改动，共 `{len(paths)}` 个路径",
    ]
    if paths:
        bullets.append("- 变更路径：`" + "`, `".join(paths[:8]) + "`")
        if len(paths) > 8:
            bullets.append(f"- 其余未展开路径：`{len(paths) - 8}` 个")
    if diff_stat:
        bullets.append("- diff 统计：")
        bullets.extend(f"  {line}" for line in diff_stat.splitlines()[:8])
    return "\n".join(bullets)


def ai_outline(path: Path, branch: str, status: str) -> str:
    if shutil.which("codex") is None:
        return fallback_outline(path, branch, status)

    prompt = textwrap.dedent(
        f"""
        你在为本地仓库生成分支状态板中的“更改大纲”。

        只基于下面提供的 Git 上下文输出简洁中文 markdown，不要虚构未出现的信息。
        输出结构固定为：

        - 意图：
        - 关键文件：
        - 行为变化：
        - 验证状态：
        - 风险/阻塞：

        要求：
        - 每一项控制在一到三行
        - 如果当前工作区是 clean，就基于最近一次提交概括
        - 如果没有足够证据，就明确写“未从上下文确认”
        - 不要输出代码块，不要加额外标题

        <git_context>
        {build_context(path, branch, status)}
        </git_context>
        """
    ).strip()

    with tempfile.NamedTemporaryFile(prefix="branch-outline-", suffix=".md", delete=False) as tmp:
        output_path = Path(tmp.name)
    try:
        ok, output = maybe_run(
            tool_args(
                [
                    "codex",
                    "exec",
                    "--disable",
                    "hooks",
                    "--ephemeral",
                    "--sandbox",
                    "read-only",
                    "--cd",
                    str(path),
                    "-c",
                    'model_reasoning_effort="low"',
                    "--output-last-message",
                    str(output_path),
                    "-",
                ]
            ),
            path,
            input_text=prompt,
            timeout=12,
        )
        if not ok:
            return fallback_outline(path, branch, status) + f"\n- AI 摘要回退：`{output}`"
        summary = output_path.read_text(encoding="utf-8").strip()
        return summary or fallback_outline(path, branch, status)
    except Exception as exc:  # noqa: BLE001
        return fallback_outline(path, branch, status) + f"\n- AI 摘要回退：`{exc}`"
    finally:
        output_path.unlink(missing_ok=True)


def render(cwd: Path, *, mode: str) -> str:
    root = repo_root(cwd)
    main_root = primary_root(cwd)
    wt_entries = worktrees(cwd)
    branch_info = {row["branch"]: row for row in branch_rows(cwd)}

    lines = [
        "# Branch Status",
        "",
        "> Auto-generated local workspace status. Do not commit this file.",
        "",
        f"- Updated: `{datetime.now(timezone.utc).astimezone().isoformat(timespec='seconds')}`",
        f"- Primary workspace: `{main_root}`",
        f"- Source repo root: `{root}`",
        "",
        "## Active Worktrees",
        "",
        "| Branch | Path | Status | HEAD | Latest Commit |",
        "|---|---|---|---|---|",
    ]

    seen_branches: set[str] = set()
    outlines: list[tuple[str, str, Path]] = []
    for entry in wt_entries:
        path = Path(entry.get("worktree", ""))
        branch_ref = entry.get("branch", "")
        branch = branch_ref.removeprefix("refs/heads/") or current_branch(path)
        seen_branches.add(branch)
        sha = entry.get("HEAD", "")[:7]
        status = worktree_status(path)
        info = branch_info.get(branch, {})
        latest = info.get("subject", "")
        lines.append(f"| `{branch}` | `{path}` | {status} | `{sha}` | {latest} |")
        if path == root or status != "clean":
            outlines.append((branch, status, path))

    remaining = [row for row in branch_rows(cwd) if row["branch"] not in seen_branches]
    if remaining:
        lines.extend(
            [
                "",
                "## Local Branches Not Checked Out",
                "",
                "| Branch | Latest Commit Date | HEAD | Latest Commit |",
                "|---|---|---|---|",
            ]
        )
        for row in remaining:
            lines.append(
                f"| `{row['branch']}` | `{row['date']}` | `{row['sha']}` | {row['subject']} |"
            )

    stash_output = run(["git", "stash", "list"], cwd).strip()
    lines.extend(["", "## Stashes", ""])
    if stash_output:
        for line in stash_output.splitlines():
            lines.append(f"- `{line}`")
    else:
        lines.append("- none")
    lines.extend(["", "## Change Outlines", ""])
    if outlines:
        for branch, status, path in outlines:
            lines.append(f"### `{branch}`")
            lines.append("")
            if mode == "full":
                lines.append(ai_outline(path, branch, status))
            else:
                lines.append(fallback_outline(path, branch, status))
            lines.append("")
    else:
        lines.append("- no active branch outlines")
        lines.append("")
    return "\n".join(lines)


def main() -> int:
    cwd = Path.cwd()
    mode = "full"
    if len(sys.argv) > 1:
        mode = sys.argv[1]
    if mode not in {"fast", "full"}:
        raise SystemExit("usage: update-branch-status.py [fast|full]")
    primary = primary_root(cwd)
    target = primary / STATUS_FILE
    ensure_local_exclude(cwd, STATUS_FILE)
    target.write_text(render(cwd, mode=mode), encoding="utf-8")
    print(target)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
