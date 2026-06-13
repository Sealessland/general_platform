---
name: git-worktree
description: Use when working in a Git repository that needs multiple branches checked out simultaneously, feature isolation, or branch-per-task workflows.
---

# Git Worktree Workflow

Use `git worktree` when a task requires switching between branches without stashing, running tests on multiple branches, or keeping long-lived feature branches in isolated directories.

## When to Use

- A feature branch needs independent build/test state from `main`.
- Two or more branches must be validated or edited in parallel.
- A rewrite, ADR, or refactor should live in its own checkout until reviewed.
- The workspace uses a branch-status board or per-worktree validation scripts.

## Core Commands

```bash
# List current worktrees
git worktree list

# Add a worktree for an existing branch
git worktree add /tmp/<repo>-<branch> <branch>

# Add a worktree and create a new branch at the same time
git worktree add -b <new-branch> /tmp/<repo>-<new-branch> <base-branch>

# Remove a worktree (does not delete the branch)
git worktree remove /tmp/<repo>-<branch>

# Remove a worktree even if it has uncommitted changes
git worktree remove --force /tmp/<repo>-<branch>

# Prune stale or manually-deleted worktree metadata
git worktree prune

# Lock a worktree so it is not pruned automatically
git worktree lock /tmp/<repo>-<branch>

# Unlock when it is safe to prune
git worktree unlock /tmp/<repo>-<branch>
```

## Safety Rules

- Never remove a worktree that contains uncommitted work unless the user explicitly confirms it is disposable.
- Keep worktree paths deterministic, e.g. `/tmp/<repo>-<branch>`.
- A branch can only be checked out in one worktree at a time.
- `git worktree remove` removes the directory; the branch remains.
- `git branch -d <branch>` removes the branch; associated worktrees must be removed first.
- Run `git worktree prune` after manual directory deletion to avoid stale metadata.

## Project-Specific Workflow

In this workspace, use the helper script instead of raw `git worktree add` when possible:

```bash
rtk bash scripts/git-worktree.sh create <branch-name>
```

After creating, removing, or switching worktrees, regenerate the local status board:

```bash
rtk python3 scripts/update-branch-status.py fast
```

Validate the branch inside its worktree:

```bash
rtk bash -c "cd /tmp/<repo>-<branch> && bash scripts/validate-workspace.sh"
```

## Cleanup Checklist

Before claiming a worktree task is done:

- [ ] The intended branch is checked out in exactly one worktree.
- [ ] Unneeded experimental worktrees are removed or explicitly kept.
- [ ] `git worktree list` matches the active branch plan.
- [ ] `BRANCH_STATUS.local.md` is regenerated and reflects current worktrees.
- [ ] Each active feature worktree passes `scripts/validate-workspace.sh`.

## Common Patterns

**Split a commit out into its own branch + worktree:**

```bash
# From the branch that still contains the mixed commit
git log --oneline --graph
git rebase --onto <new-base> <last-shared-commit> <current-branch>
git checkout -b <new-branch>
git worktree add /tmp/<repo>-<new-branch> <new-branch>
```

**Move a commit from branch A to a new branch B:**

```bash
git checkout branch-a
git branch branch-b <commit-to-move>
git rebase --onto branch-a~1 <commit-to-move> branch-a   # drop it from A
git worktree add /tmp/<repo>-branch-b branch-b
```

**Clean up a stale feature worktree:**

```bash
git worktree remove /tmp/<repo>-<branch>
git branch -D <branch>   # only if the branch is truly discarded
git worktree prune
rtk python3 scripts/update-branch-status.py fast
```
