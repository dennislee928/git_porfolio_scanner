#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   ./scripts/auto_commit.sh           # one-shot commit (+ optional push if remote exists)
#   ./scripts/auto_commit.sh --watch   # repeat every 0.5s

watch_mode=false
if [[ "${1:-}" == "--watch" ]]; then
  watch_mode=true
fi

run_once() {
  local branch remote
  branch="$(git branch --show-current)"
  if [[ -z "$branch" ]]; then
    echo "Error: unable to detect current branch." >&2
    return 1
  fi

  if git remote get-url origin >/dev/null 2>&1; then
    remote="origin"
  else
    remote=""
  fi

  if [[ -n "$(git status --porcelain)" ]]; then
    echo "Changes detected, attempting to commit on branch '$branch'..."
    git add -A

    if git commit -m "Auto-commit: Project implementation in progress" --no-verify; then
      echo "Commit successful."
    else
      echo "Nothing to commit after staging."
    fi
  else
    echo "No changes detected."
  fi

  if [[ -n "$remote" ]]; then
    # Only attempt pull/push when upstream is configured.
    if git rev-parse --abbrev-ref "${branch}@{upstream}" >/dev/null 2>&1; then
      echo "Syncing with $remote/$branch..."
      git pull --rebase "$remote" "$branch"
      git push "$remote" "$branch"
      echo "Push successful."
    else
      echo "Remote '$remote' exists, but branch '$branch' has no upstream."
      echo "Set it with: git push -u $remote $branch"
    fi
  else
    echo "No remote named 'origin' configured. Skipping pull/push."
  fi
}

if [[ "$watch_mode" == true ]]; then
  while true; do
    run_once || true
    echo "Sleeping for 0.5 seconds..."
    sleep 0.5
  done
else
  run_once
fi
