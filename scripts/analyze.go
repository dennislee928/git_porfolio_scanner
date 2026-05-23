package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Repo struct {
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	Description     string    `json:"description"`
	Private         bool      `json:"private"`
	Fork            bool      `json:"fork"`
	Language        string    `json:"language"`
	Topics          []string  `json:"topics"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	HTMLURL         string    `json:"html_url"`
	Archived        bool      `json:"archived"`
	PushedAt        time.Time `json:"pushed_at"`
	Source          string    `json:"_source"`
}

type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Bio   string `json:"bio"`
}

type Input struct {
	User  User   `json:"user"`
	Repos []Repo `json:"repos"`
}

type ScoredRepo struct {
	Repo  Repo
	Score float64
}

func main() {
	inputPath := flag.String("input", "repos.json", "Input JSON from scraper")
	outDir := flag.String("output-dir", "artifacts", "Artifacts output directory")
	flag.Parse()

	b, err := os.ReadFile(*inputPath)
	must(err)

	var in Input
	must(json.Unmarshal(b, &in))

	if len(in.Repos) == 0 {
		must(fmt.Errorf("no repos found in %s", *inputPath))
	}

	must(os.MkdirAll(*outDir, 0o755))

	topLanguages := languageRank(in.Repos, 12)
	scored := scoreRepos(in.Repos)
	topProjects := topN(scored, 12)
	highlights := topN(nonForks(scored), 8)

	must(writeFile(filepath.Join(*outDir, "repo_analysis.md"), buildRepoAnalysis(in, topLanguages, topProjects)))
	must(writeFile(filepath.Join(*outDir, "github_profile_README.md"), buildProfileREADME(in, topLanguages, highlights)))
	must(writeFile(filepath.Join(*outDir, "linkedin_optimization.md"), buildLinkedInGuide(in, topLanguages, highlights)))
	must(writeFile(filepath.Join(*outDir, "resume_master.md"), buildResume(in, topLanguages, highlights)))

	fmt.Printf("Generated artifacts in %s\n", *outDir)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644)
}

func nonForks(items []ScoredRepo) []ScoredRepo {
	out := make([]ScoredRepo, 0, len(items))
	for _, it := range items {
		if !it.Repo.Fork && !it.Repo.Archived {
			out = append(out, it)
		}
	}
	return out
}

func topN(items []ScoredRepo, n int) []ScoredRepo {
	if len(items) < n {
		n = len(items)
	}
	return items[:n]
}

func scoreRepos(repos []Repo) []ScoredRepo {
	now := time.Now()
	out := make([]ScoredRepo, 0, len(repos))
	for _, r := range repos {
		days := now.Sub(r.PushedAt).Hours() / 24.0
		recency := 0.0
		switch {
		case days <= 30:
			recency = 40
		case days <= 180:
			recency = 25
		case days <= 365:
			recency = 12
		default:
			recency = 4
		}

		impact := float64(r.StargazersCount*5 + r.ForksCount*2)
		complexity := 0.0
		if len(r.Topics) > 0 {
			complexity += 4
		}
		if r.Description != "" {
			complexity += 3
		}
		if r.Private {
			complexity += 2
		}
		if r.Fork {
			complexity -= 12
		}
		if r.Archived {
			complexity -= 10
		}

		out = append(out, ScoredRepo{Repo: r, Score: recency + impact + complexity})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Repo.PushedAt.After(out[j].Repo.PushedAt)
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func languageRank(repos []Repo, limit int) []string {
	count := map[string]int{}
	for _, r := range repos {
		if strings.TrimSpace(r.Language) != "" {
			count[r.Language]++
		}
	}
	type kv struct {
		K string
		V int
	}
	pairs := make([]kv, 0, len(count))
	for k, v := range count {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].V > pairs[j].V })
	if len(pairs) > limit {
		pairs = pairs[:limit]
	}
	res := make([]string, 0, len(pairs))
	for _, p := range pairs {
		res = append(res, fmt.Sprintf("%s (%d repos)", p.K, p.V))
	}
	return res
}

func buildRepoAnalysis(in Input, langs []string, projects []ScoredRepo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Repo Analysis for %s\n\n", displayName(in.User))
	fmt.Fprintf(&b, "- Snapshot repos: **%d**\n", len(in.Repos))
	fmt.Fprintf(&b, "- Top languages: %s\n\n", strings.Join(langs, ", "))
	b.WriteString("## Top Projects to Feature\n\n")
	b.WriteString("| Project | Score | Language | Visibility | Last Push | Why it matters |\n")
	b.WriteString("|---|---:|---|---|---|---|\n")
	for _, p := range projects {
		vis := "Public"
		if p.Repo.Private {
			vis = "Private"
		}
		reason := summarizeReason(p.Repo)
		fmt.Fprintf(&b, "| [%s](%s) | %.1f | %s | %s | %s | %s |\n",
			p.Repo.FullName, p.Repo.HTMLURL, p.Score, safe(p.Repo.Language), vis, p.Repo.PushedAt.Format("2006-01-02"), reason)
	}
	return b.String()
}

func buildProfileREADME(in Input, langs []string, projects []ScoredRepo) string {
	var b strings.Builder
	name := displayName(in.User)
	fmt.Fprintf(&b, "# Hi, I'm %s\n\n", name)
	if strings.TrimSpace(in.User.Bio) != "" {
		fmt.Fprintf(&b, "%s\n\n", in.User.Bio)
	} else {
		b.WriteString("I build production software across platform, backend, and automation domains.\n\n")
	}
	b.WriteString("## Focus\n\n")
	b.WriteString("- Building reliable systems and developer tooling\n")
	b.WriteString("- Shipping end-to-end products across backend, web, and infra\n")
	b.WriteString("- Applying AI pragmatically to real workflows\n\n")
	b.WriteString("## Core Stack\n\n")
	for i, l := range langs {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&b, "- %s\n", l)
	}
	b.WriteString("\n## Featured Projects\n\n")
	b.WriteString("| Project | What it does | Stack | Updated |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, p := range projects {
		desc := p.Repo.Description
		if strings.TrimSpace(desc) == "" {
			desc = "Engineering project"
		}
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s |\n", p.Repo.Name, p.Repo.HTMLURL, truncate(desc, 90), safe(p.Repo.Language), p.Repo.PushedAt.Format("2006-01-02"))
	}
	b.WriteString("\n## Contact\n\n")
	fmt.Fprintf(&b, "- GitHub: [@%s](https://github.com/%s)\n", in.User.Login, in.User.Login)
	b.WriteString("- LinkedIn: add your profile URL\n")
	return b.String()
}

func buildLinkedInGuide(in Input, langs []string, projects []ScoredRepo) string {
	name := displayName(in.User)
	headline := "Software Engineer | Platform, Backend, Automation, AI-enabled Delivery"
	if len(langs) > 0 {
		headline = fmt.Sprintf("Software Engineer | %s | Platform & Automation", strings.Split(langs[0], " (")[0])
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# LinkedIn Optimization for %s\n\n", name)
	fmt.Fprintf(&b, "## 1) Headline\n\n`%s`\n\n", headline)
	b.WriteString("## 2) About Section (Draft)\n\n")
	fmt.Fprintf(&b, "I build and scale software products across backend services, platform tooling, and automation. I ship end-to-end solutions from architecture to production, with strong focus on reliability, developer velocity, and measurable business outcomes.\n\n")
	fmt.Fprintf(&b, "My recent work spans %s, plus cross-functional delivery in fast-moving environments. I enjoy turning complex systems into simple workflows teams can trust.\n\n", topLangText(langs, 5))
	b.WriteString("## 3) Skills to Pin (Top 30)\n\n")
	for _, skill := range deriveSkills(langs, projects) {
		fmt.Fprintf(&b, "- %s\n", skill)
	}
	b.WriteString("\n## 4) Experience Bullet Patterns\n\n")
	for i, p := range projects {
		if i >= 8 {
			break
		}
		fmt.Fprintf(&b, "- Built and maintained **%s** (%s), improving delivery reliability and reducing manual operations through automation.\n", p.Repo.Name, safe(p.Repo.Language))
	}
	b.WriteString("\n## 5) Featured Section\n\n")
	for i, p := range projects {
		if i >= 6 {
			break
		}
		fmt.Fprintf(&b, "- %s\n", p.Repo.HTMLURL)
	}
	return b.String()
}

func buildResume(in Input, langs []string, projects []ScoredRepo) string {
	name := displayName(in.User)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	fmt.Fprintf(&b, "GitHub: https://github.com/%s  \\nLinkedIn: <your-linkedin-url>\n\n", in.User.Login)
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "Software engineer with hands-on delivery across backend, platform, and automation. Built and operated large portfolios of repositories with consistent focus on maintainability, reliability, and execution speed. Core stack includes %s.\n\n", topLangText(langs, 6))
	b.WriteString("## Core Skills\n\n")
	fmt.Fprintf(&b, "%s\n\n", strings.Join(deriveSkills(langs, projects), " | "))
	b.WriteString("## Selected Projects\n\n")
	for i, p := range projects {
		if i >= 10 {
			break
		}
		desc := p.Repo.Description
		if strings.TrimSpace(desc) == "" {
			desc = "Shipped engineering project with production-oriented implementation."
		}
		fmt.Fprintf(&b, "### %s\n", p.Repo.Name)
		fmt.Fprintf(&b, "- URL: %s\n", p.Repo.HTMLURL)
		fmt.Fprintf(&b, "- Stack: %s\n", safe(p.Repo.Language))
		fmt.Fprintf(&b, "- Last Updated: %s\n", p.Repo.PushedAt.Format("2006-01-02"))
		fmt.Fprintf(&b, "- Impact: %s\n\n", truncate(desc, 140))
	}
	b.WriteString("## Experience\n\n")
	b.WriteString("- Add your official job history here (company, title, dates), then map selected projects to each role.\n")
	b.WriteString("- Keep bullets outcome-focused: action + measurable result + tech stack.\n\n")
	b.WriteString("## Education\n\n")
	b.WriteString("- Add your education details.\n")
	return b.String()
}

func displayName(u User) string {
	if strings.TrimSpace(u.Name) != "" {
		return u.Name
	}
	return u.Login
}

func topLangText(langs []string, n int) string {
	if len(langs) == 0 {
		return "Go, TypeScript, and cloud-native tooling"
	}
	if len(langs) < n {
		n = len(langs)
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, strings.Split(langs[i], " (")[0])
	}
	return strings.Join(parts, ", ")
}

func summarizeReason(r Repo) string {
	bits := []string{}
	if r.StargazersCount > 0 {
		bits = append(bits, fmt.Sprintf("%d stars", r.StargazersCount))
	}
	if !r.PushedAt.IsZero() {
		bits = append(bits, "recently maintained")
	}
	if r.Language != "" {
		bits = append(bits, r.Language+" stack")
	}
	if len(bits) == 0 {
		return "active engineering project"
	}
	return strings.Join(bits, ", ")
}

func deriveSkills(langs []string, projects []ScoredRepo) []string {
	base := []string{"Software Architecture", "Backend Development", "API Design", "CI/CD", "Docker", "GitHub Actions", "System Design", "Automation"}
	for _, l := range langs {
		name := strings.Split(l, " (")[0]
		base = append(base, name)
	}
	for _, p := range projects {
		for _, t := range p.Repo.Topics {
			if len(base) >= 30 {
				break
			}
			base = append(base, strings.ReplaceAll(t, "-", " "))
		}
		if len(base) >= 30 {
			break
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, s := range base {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
		if len(out) >= 30 {
			break
		}
	}
	return out
}

func safe(s string) string {
	if strings.TrimSpace(s) == "" {
		return "N/A"
	}
	return strings.ReplaceAll(s, "|", "/")
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return strings.ReplaceAll(s, "|", "/")
	}
	return strings.ReplaceAll(s[:limit-3], "|", "/") + "..."
}
