# Git Portfolio Pipeline

This repo is a repeatable pipeline to:
- scrape all GitHub repos you can access (public + private + org repos)
- rank and cluster projects from a large repo set
- generate draft content for:
  - GitHub profile introduction `README.md`
  - LinkedIn optimization notes (headline/about/skills/experience bullets)
  - resume in Markdown (easy to export to PDF)

## Requirements

- Go installed and working locally
- GitHub PAT in `.env`

`.env`:

```env
GITHUB_PAT=your_token_here
```

Use scopes that can read your private and org repos.

## Quick Start

```bash
./scripts/run_all.sh
```

This runs:
1. scrape stage (`main.go`)
2. analyze/report stage (`scripts/analyze.go`)

## Outputs

- Raw snapshots:
  - `data/repos_YYYYMMDD_HHMMSS.json`
  - `data/latest_repos.json` (symlink to latest snapshot)
- Generated artifacts:
  - `artifacts/repo_analysis.md`
  - `artifacts/github_profile_README.md`
  - `artifacts/linkedin_optimization.md`
  - `artifacts/resume_master.md`
  - `artifacts/career_portfolio.md`

## Recommended Workflow (for tremendous repo volume)

1. Keep one canonical snapshot per run in `data/`
2. Feature only top 6-10 projects in public profile
3. Keep 20-30 relevant LinkedIn skills, not all skills
4. Maintain one master resume + role-specific variants
5. Re-run monthly to keep profile/resume fresh

## File Map

- `main.go`: GitHub scraping (user repos + org repos)
- `scripts/scrap_andreport.sh`: end-to-end execution
- `scripts/analyze.go`: ranking + markdown artifact generation
- `scripts/run_all.sh`: convenience wrapper

## Notes

- This pipeline intentionally generates drafts.
- Final edits should tune narrative for target roles.
- Do not commit `.env`.
