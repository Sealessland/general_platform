#!/usr/bin/env python3
"""Generate a local branch/worktree status file for the primary workspace.

生成主工作区（主仓库，而非 worktree）的本地分支状态文件 BRANCH_STATUS.local.md。
"""

from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
import textwrap
from datetime import datetime, timezone
from pathlib import Path

# 状态文件固定写入主工作区根目录；MAX_CONTEXT_CHARS 截断喂给 AI 的 git 上下文
STATUS_FILE = "BRANCH_STATUS.local.md"
MAX_CONTEXT_CHARS = 16000


def run(args: list[str], cwd: Path) -> str:
    """执行命令并返回 stdout；返回码非零时抛错。"""
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
    """尽力执行命令（带超时与可选 stdin）；返回 (是否成功, 合并后的输出)。"""
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
    """本仓库约定经 rtk 包装执行命令；环境无 rtk 时直接执行。"""
    if shutil.which("rtk"):
        return ["rtk", *args]
    return args


def repo_root(cwd: Path) -> Path:
    """返回仓库顶层目录（git rev-parse --show-toplevel）。"""
    return Path(run(["git", "rev-parse", "--show-toplevel"], cwd).strip())


def common_git_dir(cwd: Path) -> Path:
    """返回共享 git 目录（--git-common-dir），在 worktree 中也能定位主仓库元数据。"""
    raw = run(["git", "rev-parse", "--git-common-dir"], cwd).strip()
    path = Path(raw)
    if not path.is_absolute():
        path = (cwd / path).resolve()
    return path


def primary_root(cwd: Path) -> Path:
    """主工作区根目录 = 共享 git 目录的父目录（主仓库，而非当前 worktree）。"""
    return common_git_dir(cwd).parent


def ensure_local_exclude(cwd: Path, pattern: str) -> None:
    """把 pattern 追加到 .git/info/exclude，使生成的状态文件不被 git 追踪。"""
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
    """解析 git worktree list --porcelain，返回每个 worktree 的键值对字典。"""
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
    """列出本地分支（按最近提交时间倒序），返回分支/日期/sha/主题。"""
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
    """返回 worktree 的简洁状态：clean 或 dirty (路径数)。"""
    output = run(["git", "status", "--short"], path)
    lines = [line for line in output.splitlines() if line.strip()]
    if not lines:
        return "clean"
    return f"dirty ({len(lines)} paths)"


def current_branch(path: Path) -> str:
    """返回当前所在分支名。"""
    return run(["git", "branch", "--show-current"], path).strip()


def changed_paths(path: Path) -> list[str]:
    """返回未提交变更的文件路径列表（去掉状态标记列）。"""
    output = run(["git", "status", "--short"], path)
    return [line[3:] for line in output.splitlines() if line.strip()]


def clip(text: str, limit: int = MAX_CONTEXT_CHARS) -> str:
    """超出 limit 的文本截断到前 limit 字符并加标记，避免上下文过大。"""
    if len(text) <= limit:
        return text
    return text[:limit] + "\n...[truncated]..."


def git_block(path: Path, title: str, args: list[str]) -> str:
    """生成一个 git 命令输出的 markdown 小节；失败或空输出时返回空串。"""
    ok, output = maybe_run(["git", *args], path)
    if not ok or not output:
        return ""
    return f"## {title}\n{clip(output.strip())}\n"


def build_context(path: Path, branch: str, status: str) -> str:
    """汇总 worktree 的 git 上下文（提交或未提交变更的各类 diff），供 AI 摘要使用。"""
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
    """无 codex 或调用失败时的确定性大纲：只用本地 git 信息生成中文 markdown。"""
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
    """用 codex exec 基于 git 上下文生成中文变更大纲；不可用时回退到 fallback_outline。"""
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

    # codex 把最终消息写入临时文件再读取，避免与进度日志混在 stdout 中
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
    """渲染完整状态板 markdown：worktree 表、未检出分支、stash 与变更大纲。"""
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
        # porcelain 中 detached 状态的 worktree 没有 branch 字段，此时回退读取当前分支名
        branch = branch_ref.removeprefix("refs/heads/") or current_branch(path)
        seen_branches.add(branch)
        sha = entry.get("HEAD", "")[:7]
        status = worktree_status(path)
        info = branch_info.get(branch, {})
        latest = info.get("subject", "")
        lines.append(f"| `{branch}` | `{path}` | {status} | `{sha}` | {latest} |")
        # 只对主工作区或脏 worktree 生成大纲，避免对大量干净 worktree 白跑 AI 摘要
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
    """入口：解析 fast/full 模式并写状态文件到主工作区根目录。"""
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
