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

type TechSummary struct {
	Languages []string
	Domains   []string
	Systems   []string
	TopSkills []string
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
	summary := buildTechSummary(in.Repos, topLanguages, highlights)

	githubReadme := buildProfileREADME(in, summary, highlights)
	linkedinDoc := buildLinkedInGuide(in, summary, highlights)
	resumeDoc := buildResume(in, summary, highlights)

	must(writeFile(filepath.Join(*outDir, "repo_analysis.md"), buildRepoAnalysis(in, summary, topProjects)))
	must(writeFile(filepath.Join(*outDir, "github_profile_README.md"), githubReadme))
	must(writeFile(filepath.Join(*outDir, "linkedin_optimization.md"), linkedinDoc))
	must(writeFile(filepath.Join(*outDir, "resume_master.md"), resumeDoc))
	must(writeFile(filepath.Join(*outDir, "career_portfolio.md"), buildCareerPortfolio(in, summary, highlights, githubReadme, linkedinDoc, resumeDoc)))

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
		quality := 0.0
		if len(r.Topics) > 0 {
			quality += 4
		}
		if strings.TrimSpace(r.Description) != "" {
			quality += 3
		}
		if r.Private {
			quality += 1
		}
		if r.Fork {
			quality -= 12
		}
		if r.Archived {
			quality -= 10
		}

		out = append(out, ScoredRepo{Repo: r, Score: recency + impact + quality})
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

func buildTechSummary(repos []Repo, langs []string, projects []ScoredRepo) TechSummary {
	domainCount := map[string]int{}
	systemCount := map[string]int{}

	for _, r := range repos {
		text := strings.ToLower(strings.Join([]string{r.Name, r.Description, strings.Join(r.Topics, " ")}, " "))

		if hasAny(text, "api", "gateway", "grpc", "graphql", "rest") {
			systemCount["API and service integration"]++
		}
		if hasAny(text, "microservice", "service mesh", "kubernetes", "docker", "container") {
			systemCount["Distributed systems and container platforms"]++
		}
		if hasAny(text, "ci", "cd", "pipeline", "github action", "automation") {
			systemCount["CI/CD and release automation systems"]++
		}
		if hasAny(text, "security", "firmware", "forensics", "yara", "vulnerability") {
			systemCount["Security engineering and analysis workflows"]++
		}
		if hasAny(text, "web3", "blockchain", "ethereum", "smart contract") {
			systemCount["Web3 application and protocol integration"]++
		}
		if hasAny(text, "frontend", "react", "vue", "ui", "dashboard") {
			systemCount["Frontend product interfaces and dashboards"]++
		}

		if hasAny(text, "infra", "terraform", "iac", "k8s", "cloud") {
			domainCount["Cloud and Infrastructure"]++
		}
		if hasAny(text, "security", "firmware", "forensics", "vuln") {
			domainCount["Security Engineering"]++
		}
		if hasAny(text, "fintech", "trading", "payment", "crypto", "web3") {
			domainCount["FinTech and Web3"]++
		}
		if hasAny(text, "platform", "devtool", "tooling", "automation", "workflow") {
			domainCount["Developer Platform and Automation"]++
		}
		if hasAny(text, "ai", "ml", "llm", "model", "agent") {
			domainCount["AI/ML Application Engineering"]++
		}
	}

	return TechSummary{
		Languages: topLabels(langs, 8),
		Domains:   topMapKeys(domainCount, 5),
		Systems:   topMapKeys(systemCount, 5),
		TopSkills: deriveHRSkills(langs, projects),
	}
}

func buildRepoAnalysis(in Input, s TechSummary, projects []ScoredRepo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Repo Analysis for %s\n\n", displayName(in.User))
	fmt.Fprintf(&b, "- Snapshot repos: **%d**\n", len(in.Repos))
	fmt.Fprintf(&b, "- Dominant languages: %s\n", strings.Join(s.Languages, ", "))
	fmt.Fprintf(&b, "- Core domains: %s\n", strings.Join(s.Domains, ", "))
	fmt.Fprintf(&b, "- System patterns: %s\n\n", strings.Join(s.Systems, ", "))
	b.WriteString("## Top Projects to Feature\n\n")
	b.WriteString("| Project | Score | Language | Last Push | Inferred System Context |\n")
	b.WriteString("|---|---:|---|---|---|\n")
	for _, p := range projects {
		fmt.Fprintf(&b, "| [%s](%s) | %.1f | %s | %s | %s |\n",
			p.Repo.FullName, p.Repo.HTMLURL, p.Score, safe(p.Repo.Language), p.Repo.PushedAt.Format("2006-01-02"), inferSystemContext(p.Repo))
	}
	return b.String()
}

func buildProfileREADME(in Input, s TechSummary, projects []ScoredRepo) string {
	var b strings.Builder
	name := displayName(in.User)
	fmt.Fprintf(&b, "# %s | Engineering Portfolio\n\n", name)
	b.WriteString("## Technical Positioning\n\n")
	fmt.Fprintf(&b, "Engineer focused on %s. Primary implementation languages include %s.\n\n", strings.Join(s.Domains, ", "), strings.Join(s.Languages, ", "))

	b.WriteString("## System Architecture Focus\n\n")
	for _, sys := range s.Systems {
		fmt.Fprintf(&b, "- %s\n", sys)
	}

	b.WriteString("\n## Engineering Scope\n\n")
	b.WriteString("- API layer: REST/GraphQL/service interfaces and gateway integration\n")
	b.WriteString("- Application layer: business logic, workflow orchestration, and automation\n")
	b.WriteString("- Runtime layer: containerized deployments, CI/CD pipelines, reliability practices\n")
	b.WriteString("- Product layer: web interfaces and domain-specific tooling\n\n")

	b.WriteString("## Featured Repositories (Technical)\n\n")
	b.WriteString("| Repository | Role in Architecture | Tech Stack | Last Updated |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, p := range projects {
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s |\n",
			p.Repo.Name, p.Repo.HTMLURL, inferSystemContext(p.Repo), safe(p.Repo.Language), p.Repo.PushedAt.Format("2006-01-02"))
	}

	b.WriteString("\n## Contact\n\n")
	fmt.Fprintf(&b, "- GitHub: [@%s](https://github.com/%s)\n", in.User.Login, in.User.Login)
	b.WriteString("- LinkedIn: <add-linkedin-url>\n")
	return b.String()
}

func buildLinkedInGuide(in Input, s TechSummary, projects []ScoredRepo) string {
	name := displayName(in.User)
	headline := fmt.Sprintf("Software Engineer | %s | Platform, Product, and Delivery", strings.Join(s.Domains, " | "))
	if len(headline) > 120 {
		headline = "Software Engineer | Platform Engineering, Automation, and Product Delivery"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# LinkedIn Optimization for %s\n\n", name)
	b.WriteString("## 1) Headline (HR-Friendly)\n\n")
	fmt.Fprintf(&b, "`%s`\n\n", headline)

	b.WriteString("## 2) About Section (Recruiter-Facing Draft)\n\n")
	fmt.Fprintf(&b, "Software engineer with a large-scale GitHub portfolio and hands-on delivery across %s. Proven execution from architecture to production, including API-enabled products, automation systems, and developer tooling.\n\n", strings.Join(s.Domains, ", "))
	fmt.Fprintf(&b, "Strengths include cross-functional execution, fast iteration, and building maintainable systems with %s. Comfortable operating in ambiguous environments and aligning technical decisions with product outcomes.\n\n", strings.Join(s.Languages, ", "))

	b.WriteString("## 3) Skills to Pin (Top 30, Recruiter Search Optimized)\n\n")
	for _, skill := range s.TopSkills {
		fmt.Fprintf(&b, "- %s\n", skill)
	}

	b.WriteString("\n## 4) Experience Bullets (HR Format: Action + Scope + Outcome)\n\n")
	for i, p := range projects {
		if i >= 8 {
			break
		}
		fmt.Fprintf(&b, "- Delivered **%s** (%s), owning design and implementation of %s to improve delivery speed and operational consistency.\n", p.Repo.Name, safe(p.Repo.Language), inferSystemContext(p.Repo))
	}

	b.WriteString("\n## 5) Featured Section Links\n\n")
	for i, p := range projects {
		if i >= 6 {
			break
		}
		fmt.Fprintf(&b, "- %s\n", p.Repo.HTMLURL)
	}

	b.WriteString("\n## 6) Profile Hygiene\n\n")
	b.WriteString("- Keep Experience titles outcome-oriented (not tool names).\n")
	b.WriteString("- Keep About section to 5-7 lines, with one quantified achievement per role when possible.\n")
	b.WriteString("- Align Top 3 skills with target role keywords.\n")
	return b.String()
}

func buildResume(in Input, s TechSummary, projects []ScoredRepo) string {
	name := displayName(in.User)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	fmt.Fprintf(&b, "GitHub: https://github.com/%s  \nLinkedIn: <your-linkedin-url>\n\n", in.User.Login)

	b.WriteString("## Professional Summary\n\n")
	fmt.Fprintf(&b, "Software engineer with portfolio scale across %d repositories, specializing in %s. Delivers production systems end-to-end with focus on reliability, maintainability, and speed of execution. Core technical stack includes %s.\n\n", len(in.Repos), strings.Join(s.Domains, ", "), strings.Join(s.Languages, ", "))

	b.WriteString("## Core Competencies (HR-Oriented)\n\n")
	fmt.Fprintf(&b, "%s\n\n", strings.Join(s.TopSkills, " | "))

	b.WriteString("## Selected Project Experience\n\n")
	for i, p := range projects {
		if i >= 8 {
			break
		}
		desc := cleanDesc(p.Repo.Description)
		if desc == "" {
			desc = "Implemented and maintained production-grade engineering workflows."
		}
		fmt.Fprintf(&b, "### %s\n", p.Repo.Name)
		fmt.Fprintf(&b, "- Scope: %s\n", inferSystemContext(p.Repo))
		fmt.Fprintf(&b, "- Stack: %s\n", safe(p.Repo.Language))
		fmt.Fprintf(&b, "- Outcome: %s\n", truncate(desc, 180))
		fmt.Fprintf(&b, "- Link: %s\n\n", p.Repo.HTMLURL)
	}

	b.WriteString("## Experience\n\n")
	b.WriteString("- Add official roles (Company, Title, Date) and map project evidence to each role.\n")
	b.WriteString("- Use measurable outcomes where possible: latency, reliability, throughput, cost, release frequency.\n\n")

	b.WriteString("## Education\n\n")
	b.WriteString("- Add degree, institution, and graduation date.\n")
	return b.String()
}

func buildCareerPortfolio(in Input, s TechSummary, projects []ScoredRepo, githubReadme, linkedinDoc, resumeDoc string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Career Portfolio | %s\n\n", displayName(in.User))
	b.WriteString("## Executive Overview\n\n")
	fmt.Fprintf(&b, "This portfolio consolidates technical positioning (GitHub profile), recruiter positioning (LinkedIn), and hiring-document positioning (resume) from a single repository dataset.\n\n")
	fmt.Fprintf(&b, "- Portfolio scale: **%d repositories**\n", len(in.Repos))
	fmt.Fprintf(&b, "- Technical domains: %s\n", strings.Join(s.Domains, ", "))
	fmt.Fprintf(&b, "- System capabilities: %s\n", strings.Join(s.Systems, ", "))
	fmt.Fprintf(&b, "- Core stack: %s\n\n", strings.Join(s.Languages, ", "))

	b.WriteString("## Flagship Repositories\n\n")
	b.WriteString("| Repo | System Context | Stack | Last Updated |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, p := range projects {
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s |\n", p.Repo.Name, p.Repo.HTMLURL, inferSystemContext(p.Repo), safe(p.Repo.Language), p.Repo.PushedAt.Format("2006-01-02"))
	}

	b.WriteString("\n## Content Strategy by Channel\n\n")
	b.WriteString("- GitHub README: architecture, system boundaries, and technical implementation scope.\n")
	b.WriteString("- LinkedIn: business-friendly positioning, transferable strengths, recruiter keywords.\n")
	b.WriteString("- Resume: concise evidence of outcomes, role fit, and execution history.\n\n")

	b.WriteString("## Next Actions\n\n")
	b.WriteString("1. Replace placeholder contact links with final profile URLs.\n")
	b.WriteString("2. Add quantified metrics to top 5 project bullets.\n")
	b.WriteString("3. Tailor one resume variant per target role.\n")
	b.WriteString("4. Refresh artifacts monthly using `./scripts/run_all.sh`.\n")

	_ = githubReadme
	_ = linkedinDoc
	_ = resumeDoc
	return b.String()
}

func inferSystemContext(r Repo) string {
	text := strings.ToLower(strings.Join([]string{r.Name, r.Description, strings.Join(r.Topics, " ")}, " "))
	switch {
	case hasAny(text, "api", "gateway", "grpc", "graphql", "rest"):
		return "API/service integration and interface orchestration"
	case hasAny(text, "ci", "cd", "workflow", "github action", "pipeline"):
		return "CI/CD pipeline automation and delivery workflow"
	case hasAny(text, "security", "firmware", "forensics", "yara", "vulnerability"):
		return "Security analysis and defensive engineering workflow"
	case hasAny(text, "web3", "ethereum", "blockchain", "smart contract"):
		return "Web3 product workflow and blockchain integration"
	case hasAny(text, "frontend", "react", "vue", "ui", "dashboard"):
		return "Frontend application and product interface delivery"
	case hasAny(text, "docker", "kubernetes", "infra", "terraform", "cloud"):
		return "Infrastructure/platform operations and runtime management"
	default:
		return "General software system implementation"
	}
}

func deriveHRSkills(langs []string, projects []ScoredRepo) []string {
	base := []string{
		"Software Engineering", "System Design", "Backend Development", "API Design (REST/GraphQL)",
		"CI/CD", "DevOps", "Automation", "Cloud Infrastructure", "Containerization (Docker)",
		"Testing and Quality Engineering", "Technical Documentation", "Cross-functional Collaboration",
	}
	for _, l := range langs {
		base = append(base, strings.Split(l, " (")[0])
	}
	for _, p := range projects {
		for _, t := range p.Repo.Topics {
			if len(base) >= 40 {
				break
			}
			mapped := topicToSkill(t)
			if mapped != "" {
				base = append(base, mapped)
			}
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, s := range base {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k := strings.ToLower(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
		if len(out) >= 30 {
			break
		}
	}
	return out
}

func topicToSkill(topic string) string {
	t := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(topic), "_", "-"))
	switch {
	case strings.Contains(t, "security"):
		return "Application Security"
	case strings.Contains(t, "fintech") || strings.Contains(t, "trading"):
		return "FinTech Systems"
	case strings.Contains(t, "blockchain") || strings.Contains(t, "web3") || strings.Contains(t, "ethereum"):
		return "Blockchain Integration"
	case strings.Contains(t, "kubernetes") || strings.Contains(t, "k8s"):
		return "Kubernetes"
	case strings.Contains(t, "terraform") || strings.Contains(t, "iac"):
		return "Infrastructure as Code"
	case strings.Contains(t, "github-actions") || strings.Contains(t, "ci"):
		return "GitHub Actions"
	case strings.Contains(t, "frontend") || strings.Contains(t, "react") || strings.Contains(t, "vue"):
		return "Frontend Engineering"
	case strings.Contains(t, "api"):
		return "API Integration"
	default:
		return ""
	}
}

func topLabels(langs []string, n int) []string {
	if len(langs) < n {
		n = len(langs)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, strings.Split(langs[i], " (")[0])
	}
	if len(out) == 0 {
		return []string{"Go", "TypeScript", "Python"}
	}
	return out
}

func topMapKeys(m map[string]int, n int) []string {
	type kv struct {
		K string
		V int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].V > pairs[j].V })
	if len(pairs) < n {
		n = len(pairs)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, pairs[i].K)
	}
	if len(out) == 0 {
		return []string{"Platform Engineering", "Backend Systems", "Automation"}
	}
	return out
}

func hasAny(text string, tokens ...string) bool {
	for _, t := range tokens {
		if strings.Contains(text, t) {
			return true
		}
	}
	return false
}

func displayName(u User) string {
	if strings.TrimSpace(u.Name) != "" {
		return u.Name
	}
	return u.Login
}

func cleanDesc(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
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
