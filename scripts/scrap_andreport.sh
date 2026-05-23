#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load .env from repo root if present.
if [[ -f "$REPO_ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$REPO_ROOT/.env"
  set +a
fi

if [[ -z "${GITHUB_PAT:-}" ]]; then
  echo "Error: GITHUB_PAT is not set. Add it to $REPO_ROOT/.env or export it in your shell." >&2
  exit 1
fi

cd "$REPO_ROOT"
go run 1.go
