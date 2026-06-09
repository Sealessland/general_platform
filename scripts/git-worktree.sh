#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_NAME="$(basename "$ROOT_DIR")"
WORKTREE_BASE="${WORKTREE_BASE:-/tmp}"
STATUS_SCRIPT="$ROOT_DIR/scripts/update-branch-status.py"

usage() {
  cat <<'EOF'
Usage:
  bash scripts/git-worktree.sh create <branch> [start-point]
  bash scripts/git-worktree.sh list
  bash scripts/git-worktree.sh path <branch>
  bash scripts/git-worktree.sh remove <path>
  bash scripts/git-worktree.sh prune
EOF
}

slugify_branch() {
  printf '%s' "$1" | tr '/:' '--'
}

worktree_path_for_branch() {
  local branch="$1"
  printf '%s/%s-%s\n' "$WORKTREE_BASE" "$REPO_NAME" "$(slugify_branch "$branch")"
}

cmd_create() {
  local branch="${1:-}"
  local start_point="${2:-HEAD}"
  local path
  if [[ -z "$branch" ]]; then
    usage
    exit 1
  fi

  path="$(worktree_path_for_branch "$branch")"
  if git -C "$ROOT_DIR" show-ref --verify --quiet "refs/heads/$branch"; then
    git -C "$ROOT_DIR" worktree add "$path" "$branch"
  else
    git -C "$ROOT_DIR" worktree add -b "$branch" "$path" "$start_point"
  fi
  python3 "$STATUS_SCRIPT" fast >/dev/null
  printf '%s\n' "$path"
}

cmd_list() {
  git -C "$ROOT_DIR" worktree list
}

cmd_path() {
  local branch="${1:-}"
  if [[ -z "$branch" ]]; then
    usage
    exit 1
  fi
  worktree_path_for_branch "$branch"
}

cmd_remove() {
  local path="${1:-}"
  if [[ -z "$path" ]]; then
    usage
    exit 1
  fi
  git -C "$ROOT_DIR" worktree remove "$path"
  python3 "$STATUS_SCRIPT" fast >/dev/null
}

cmd_prune() {
  git -C "$ROOT_DIR" worktree prune
  python3 "$STATUS_SCRIPT" fast >/dev/null
}

main() {
  local cmd="${1:-}"
  shift || true

  case "$cmd" in
    create) cmd_create "$@" ;;
    list) cmd_list ;;
    path) cmd_path "$@" ;;
    remove) cmd_remove "$@" ;;
    prune) cmd_prune ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"
