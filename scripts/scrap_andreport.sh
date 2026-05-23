#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

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
mkdir -p data

out="data/repos_$(date +%Y%m%d_%H%M%S).json"
echo "[1/2] Scraping repos into $out"
GITHUB_OUTPUT_FILE="$out" go run main.go

ln -sf "$(basename "$out")" data/latest_repos.json

echo "[2/2] Generating profile/report artifacts"
go run scripts/analyze.go --input "$out" --output-dir artifacts

echo "Done. See artifacts/ and data/latest_repos.json"
