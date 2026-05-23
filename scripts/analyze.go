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
	"unicode"
)

type Repo struct {
	ID              int64          `json:"id"`
	Name            string         `json:"name"`
	FullName        string         `json:"full_name"`
	Description     string         `json:"description"`
	Private         bool           `json:"private"`
	Visibility      string         `json:"visibility"`
	Fork            bool           `json:"fork"`
	Language        string         `json:"language"`
	LanguageBytes   map[string]int `json:"language_bytes"`
	Topics          []string       `json:"topics"`
	StargazersCount int            `json:"stargazers_count"`
	ForksCount      int            `json:"forks_count"`
	HTMLURL         string         `json:"html_url"`
	Archived        bool           `json:"archived"`
	PushedAt        time.Time      `json:"pushed_at"`
	Owner           struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"owner"`
	Source     string   `json:"_source"`
	SourceType string   `json:"_source_type"`
	Sources    []string `json:"_sources"`
}

type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Bio   string `json:"bio"`
}

type Stats struct {
	PublicRepos       int            `json:"public_repos"`
	PrivateRepos      int            `json:"private_repos"`
	OwnerRepos        int            `json:"owner_repos"`
	OrgRepos          int            `json:"org_repos"`
	CollaboratorRepos int            `json:"collaborator_repos"`
	ArchivedRepos     int            `json:"archived_repos"`
	ForkRepos         int            `json:"fork_repos"`
	LanguageRepoCount map[string]int `json:"language_repo_count"`
	LanguageBytes     map[string]int `json:"language_bytes"`
	TopicCount        map[string]int `json:"topic_count"`
	ReposBySource     map[string]int `json:"repos_by_source"`
}

type Input struct {
	User       User      `json:"user"`
	Repos      []Repo    `json:"repos"`
	TotalRepos int       `json:"total_repos"`
	OrgNames   []string  `json:"orgs"`
	Stats      Stats     `json:"stats"`
	ScrapedAt  time.Time `json:"scraped_at"`
}

type ScoredRepo struct {
	Repo    Repo
	Score   float64
	Domain  string
	System  string
	Signals []string
}

type DomainCluster struct {
	Name       string
	Count      int
	Public     int
	Private    int
	Languages  map[string]int
	TopRepos   []ScoredRepo
	Systems    map[string]int
	SignalText string
}

type Portfolio struct {
	Input          Input
	Scored         []ScoredRepo
	PublicFeatured []ScoredRepo
	CareerFeatured []ScoredRepo
	PrivateFeatured []ScoredRepo
	Clusters       []DomainCluster
	Languages      []string
	SkillGroups    map[string][]string
}

func main() {
	inputPath := flag.String("input", "repos.json", "Input JSON from scraper")
	outDir := flag.String("output-dir", "artifacts", "Artifacts output directory")
	flag.Parse()

	raw, err := os.ReadFile(*inputPath)
	must(err)

	var in Input
	must(json.Unmarshal(raw, &in))
	if len(in.Repos) == 0 {
		must(fmt.Errorf("no repos found in %s", *inputPath))
	}

	p := buildPortfolio(in)
	must(os.MkdirAll(*outDir, 0o755))
	must(writeFile(filepath.Join(*outDir, "repo_analysis.md"), buildRepoAnalysis(p)))
	must(writeFile(filepath.Join(*outDir, "github_profile_README.md"), buildProfileREADME(p)))
	must(writeFile(filepath.Join(*outDir, "linkedin_optimization.md"), buildLinkedInGuide(p)))
	must(writeFile(filepath.Join(*outDir, "resume_master.md"), buildResume(p)))
	must(writeFile(filepath.Join(*outDir, "career_portfolio.md"), buildCareerPortfolio(p)))

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

func buildPortfolio(in Input) Portfolio {
	stats := in.Stats
	if stats.PublicRepos == 0 && stats.PrivateRepos == 0 {
		stats = inferStats(in.Repos)
		in.Stats = stats
	}

	scored := make([]ScoredRepo, 0, len(in.Repos))
	for _, repo := range in.Repos {
		domain := inferDomain(repo)
		system := inferSystem(repo)
		signals := inferSignals(repo)
		scored = append(scored, ScoredRepo{
			Repo:    repo,
			Score:   repoScore(repo, domain, signals),
			Domain:  domain,
			System:  system,
			Signals: signals,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Repo.PushedAt.After(scored[j].Repo.PushedAt)
		}
		return scored[i].Score > scored[j].Score
	})

	clusters := clusterRepos(scored)
	return Portfolio{
		Input:          in,
		Scored:         scored,
		PublicFeatured: firstN(filterRepos(scored, true), 8),
		CareerFeatured: firstN(filterRepos(scored, false), 12),
		PrivateFeatured: firstN(filterPrivateRepos(scored), 8),
		Clusters:       clusters,
		Languages:      rankedLanguages(in, 12),
		SkillGroups:    skillGroups(in, clusters),
	}
}

func repoScore(repo Repo, domain string, signals []string) float64 {
	now := time.Now()
	days := now.Sub(repo.PushedAt).Hours() / 24
	recency := 0.0
	switch {
	case days <= 30:
		recency = 24
	case days <= 180:
		recency = 16
	case days <= 365:
		recency = 9
	default:
		recency = 3
	}

	score := recency
	score += float64(repo.StargazersCount*4 + repo.ForksCount*2)
	score += float64(len(repo.Topics)) * 1.5
	score += float64(len(signals)) * 2
	if strings.TrimSpace(repo.Description) != "" {
		score += 4
	}
	if domain != "General Software" {
		score += 5
	}
	if repo.Private {
		score += 3
	}
	if repo.SourceType == "org" {
		score += 4
	}
	if repo.Fork {
		score -= 14
	}
	if repo.Archived {
		score -= 10
	}
	return score
}

func filterRepos(repos []ScoredRepo, publicOnly bool) []ScoredRepo {
	out := make([]ScoredRepo, 0, len(repos))
	for _, item := range repos {
		if item.Repo.Archived || item.Repo.Fork {
			continue
		}
		if publicOnly && item.Repo.Private {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterPrivateRepos(repos []ScoredRepo) []ScoredRepo {
	out := make([]ScoredRepo, 0, len(repos))
	for _, item := range repos {
		if item.Repo.Archived || item.Repo.Fork || !item.Repo.Private {
			continue
		}
		out = append(out, item)
	}
	return out
}

func firstN[T any](items []T, n int) []T {
	if len(items) < n {
		n = len(items)
	}
	return items[:n]
}

func clusterRepos(scored []ScoredRepo) []DomainCluster {
	byDomain := map[string]*DomainCluster{}
	for _, item := range scored {
		c := byDomain[item.Domain]
		if c == nil {
			c = &DomainCluster{
				Name:      item.Domain,
				Languages: map[string]int{},
				Systems:   map[string]int{},
			}
			byDomain[item.Domain] = c
		}
		c.Count++
		if item.Repo.Private {
			c.Private++
		} else {
			c.Public++
		}
		if item.Repo.Language != "" {
			c.Languages[item.Repo.Language]++
		}
		c.Systems[item.System]++
		if !item.Repo.Archived && !item.Repo.Fork && len(c.TopRepos) < 5 {
			c.TopRepos = append(c.TopRepos, item)
		}
	}

	clusters := make([]DomainCluster, 0, len(byDomain))
	for _, c := range byDomain {
		c.SignalText = clusterSignal(*c)
		clusters = append(clusters, *c)
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Count == clusters[j].Count {
			return clusters[i].Name < clusters[j].Name
		}
		return clusters[i].Count > clusters[j].Count
	})
	return clusters
}

func buildRepoAnalysis(p Portfolio) string {
	in := p.Input
	var b strings.Builder
	fmt.Fprintf(&b, "# Repository Portfolio Analysis for %s\n\n", displayName(in.User))
	fmt.Fprintf(&b, "Snapshot: %s\n\n", dateOrUnknown(in.ScrapedAt))
	b.WriteString("## Coverage\n\n")
	fmt.Fprintf(&b, "- Total accessible repositories: **%d**\n", len(in.Repos))
	fmt.Fprintf(&b, "- Public/private: **%d public**, **%d private**\n", in.Stats.PublicRepos, in.Stats.PrivateRepos)
	fmt.Fprintf(&b, "- Ownership mix: **%d personal**, **%d org-owned**, **%d collaborator**\n", in.Stats.OwnerRepos, in.Stats.OrgRepos, in.Stats.CollaboratorRepos)
	fmt.Fprintf(&b, "- Organizations discovered: **%d** (%s)\n", len(in.OrgNames), strings.Join(in.OrgNames, ", "))
	fmt.Fprintf(&b, "- Primary stack: %s\n\n", strings.Join(p.Languages, ", "))

	b.WriteString("## Domain Clusters\n\n")
	b.WriteString("| Domain | Repos | Public | Private | Main Languages | System Signal |\n")
	b.WriteString("|---|---:|---:|---:|---|---|\n")
	for _, c := range p.Clusters {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %s | %s |\n", c.Name, c.Count, c.Public, c.Private, topMap(c.Languages, 4), c.SignalText)
	}

	b.WriteString("\n## Top Career Evidence\n\n")
	b.WriteString("| Repo | Visibility | Domain | System | Stack | Evidence |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, item := range p.CareerFeatured {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", repoLabel(item.Repo, true), visibility(item.Repo), item.Domain, item.System, stackLabel(item.Repo), evidenceLine(item))
	}
	return b.String()
}

func buildProfileREADME(p Portfolio) string {
	in := p.Input
	var b strings.Builder
	fmt.Fprintf(&b, "# %s | Technical Portfolio\n\n", displayName(in.User))
	b.WriteString("I build software systems across backend services, security workflows, automation, cloud infrastructure, and product interfaces. This profile highlights public repositories; private and organization repositories are included in aggregate stack analysis.\n\n")

	b.WriteString("## Architecture Map\n\n")
	for _, c := range firstN(p.Clusters, 6) {
		fmt.Fprintf(&b, "- **%s**: %d repos, focused on %s\n", c.Name, c.Count, c.SignalText)
	}

	b.WriteString("\n## Stack Signal\n\n")
	for _, lang := range firstN(p.Languages, 10) {
		fmt.Fprintf(&b, "- %s\n", lang)
	}

	b.WriteString("\n## Public Repositories to Inspect\n\n")
	b.WriteString("| Repository | Architectural Role | Stack | Last Updated |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, item := range p.PublicFeatured {
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s |\n", item.Repo.Name, item.Repo.HTMLURL, item.System, stackLabel(item.Repo), dateOrUnknown(item.Repo.PushedAt))
	}

	b.WriteString("\n## Engineering Narrative\n\n")
	b.WriteString("- Backend and integration work: service boundaries, data flow, authentication-aware APIs, and operational endpoints.\n")
	b.WriteString("- Delivery systems: CI/CD pipelines, repeatable automation, repository hygiene, and release support.\n")
	b.WriteString("- Security and infrastructure: security analysis workflows, containerized environments, and cloud/IaC patterns.\n")
	b.WriteString("- Product work: frontend dashboards, user workflows, and domain-specific applications.\n\n")

	b.WriteString("## Contact\n\n")
	fmt.Fprintf(&b, "- GitHub: [@%s](https://github.com/%s)\n", in.User.Login, in.User.Login)
	b.WriteString("- LinkedIn: <add-linkedin-url>\n")
	return b.String()
}

func buildLinkedInGuide(p Portfolio) string {
	in := p.Input
	var b strings.Builder
	fmt.Fprintf(&b, "# LinkedIn Optimization for %s\n\n", displayName(in.User))
	b.WriteString("## Positioning\n\n")
	fmt.Fprintf(&b, "Use the portfolio scale as proof: %d accessible repositories across %d organizations, including private and organization work. LinkedIn should emphasize breadth, execution, and role fit rather than listing every repository.\n\n", len(in.Repos), len(in.OrgNames))

	b.WriteString("## Headline Options\n\n")
	b.WriteString("- Software Engineer | Backend, Platform Automation, Security Workflows\n")
	b.WriteString("- Full-Stack / Platform Engineer | Cloud, CI/CD, API Systems, Security\n")
	b.WriteString("- Software Engineer | Product Engineering, DevOps Automation, Cybersecurity\n\n")

	b.WriteString("## About Section Draft\n\n")
	fmt.Fprintf(&b, "Software engineer with a broad GitHub portfolio spanning %s. I build backend services, product interfaces, automation workflows, and security-oriented systems, with experience across personal, private, and organization repositories.\n\n", joinClusterNames(p.Clusters, 5))
	fmt.Fprintf(&b, "My strongest stack signals are %s. I work best where engineering needs to connect product delivery, system reliability, developer velocity, and practical security thinking.\n\n", strings.Join(firstN(p.Languages, 6), ", "))

	b.WriteString("## Skills to Pin\n\n")
	for _, group := range orderedSkillGroups() {
		skills := p.SkillGroups[group]
		if len(skills) == 0 {
			continue
		}
		fmt.Fprintf(&b, "**%s:** %s\n\n", group, strings.Join(skills, ", "))
	}

	b.WriteString("## Experience Bullet Bank\n\n")
	for _, c := range firstN(p.Clusters, 6) {
		fmt.Fprintf(&b, "- Delivered %s work across %d repositories, including %d private/internal repositories, with focus on %s.\n", strings.ToLower(c.Name), c.Count, c.Private, c.SignalText)
	}
	for _, item := range firstN(p.CareerFeatured, 6) {
		fmt.Fprintf(&b, "- Built **%s** using %s to support %s.\n", publicSafeName(item.Repo), stackLabel(item.Repo), strings.ToLower(item.System))
	}

	b.WriteString("\n## Featured Section Strategy\n\n")
	b.WriteString("- Feature 3-5 polished public repos only.\n")
	b.WriteString("- Use the portfolio/career document for private and organization experience.\n")
	b.WriteString("- Put the GitHub profile README first, then strongest public projects, then resume PDF.\n")
	return b.String()
}

func buildResume(p Portfolio) string {
	in := p.Input
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", displayName(in.User))
	fmt.Fprintf(&b, "GitHub: https://github.com/%s  \nLinkedIn: <your-linkedin-url>  \nEmail: <your-email>\n\n", in.User.Login)

	b.WriteString("## Professional Summary\n\n")
	fmt.Fprintf(&b, "Software engineer with a portfolio of %d accessible repositories across personal, private, and organization work. Strongest evidence areas include %s. Experienced in translating broad technical requirements into working software, automation, and maintainable engineering workflows.\n\n", len(in.Repos), joinClusterNames(p.Clusters, 5))

	b.WriteString("## Core Skills\n\n")
	for _, group := range orderedSkillGroups() {
		skills := p.SkillGroups[group]
		if len(skills) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- %s: %s\n", group, strings.Join(skills, ", "))
	}

	b.WriteString("\n## Selected Portfolio Experience\n\n")
	for _, c := range firstN(p.Clusters, 5) {
		fmt.Fprintf(&b, "### %s\n", c.Name)
		fmt.Fprintf(&b, "- Scope: %d repositories (%d public, %d private) across %s.\n", c.Count, c.Public, c.Private, topMap(c.Languages, 4))
		fmt.Fprintf(&b, "- Systems: %s.\n", c.SignalText)
		if len(c.TopRepos) > 0 {
			fmt.Fprintf(&b, "- Evidence: %s.\n", repoList(c.TopRepos, 3))
		}
		b.WriteString("- Resume rewrite target: add one quantified result here after choosing the target job.\n\n")
	}

	b.WriteString("## Representative Projects\n\n")
	for _, item := range firstN(p.CareerFeatured, 8) {
		fmt.Fprintf(&b, "### %s\n", publicSafeName(item.Repo))
		fmt.Fprintf(&b, "- Domain: %s\n", item.Domain)
		fmt.Fprintf(&b, "- System: %s\n", item.System)
		fmt.Fprintf(&b, "- Stack: %s\n", stackLabel(item.Repo))
		fmt.Fprintf(&b, "- Evidence: %s\n", evidenceLine(item))
		if !item.Repo.Private && item.Repo.HTMLURL != "" {
			fmt.Fprintf(&b, "- Link: %s\n", item.Repo.HTMLURL)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Experience\n\n")
	b.WriteString("- Add official company/title/date history here, then map the portfolio bullets above to the correct role.\n\n")
	b.WriteString("## Education\n\n")
	b.WriteString("- Add school, degree, and graduation date.\n")
	return b.String()
}

func buildCareerPortfolio(p Portfolio) string {
	in := p.Input
	var b strings.Builder
	fmt.Fprintf(&b, "# Individual Career Portfolio | %s\n\n", displayName(in.User))
	b.WriteString("## Executive Narrative\n\n")
	fmt.Fprintf(&b, "This portfolio is built from a full GitHub inventory, not only public profile repositories. It considers %d accessible repositories, %d organizations, and %d private repositories to infer technical strengths and career positioning.\n\n", len(in.Repos), len(in.OrgNames), in.Stats.PrivateRepos)

	b.WriteString("## Portfolio Thesis\n\n")
	fmt.Fprintf(&b, "The strongest career signal is a hybrid engineering profile: %s. The profile should be marketed as a builder who can move from system design to implementation, automation, security awareness, and product delivery.\n\n", joinClusterNames(p.Clusters, 5))

	b.WriteString("## Combined Channel Strategy\n\n")
	b.WriteString("| Channel | Main Audience | Content Logic | What to Show |\n")
	b.WriteString("|---|---|---|---|\n")
	b.WriteString("| GitHub README | Engineers and technical hiring managers | Architecture-first, public proof, stack clarity | Public repos, system map, engineering scope |\n")
	b.WriteString("| LinkedIn | Recruiters and managers | HR keywords, scope, transferable strengths | Headline, About, skills, featured public links |\n")
	b.WriteString("| Resume | ATS and interview loops | Concise role fit and evidence | Domain clusters, representative projects, measurable outcomes |\n\n")

	b.WriteString("## Domain Evidence Matrix\n\n")
	b.WriteString("| Domain | Evidence Scale | Public/Private Mix | Career Message |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, c := range firstN(p.Clusters, 7) {
		fmt.Fprintf(&b, "| %s | %d repos | %d public / %d private | %s |\n", c.Name, c.Count, c.Public, c.Private, c.SignalText)
	}

	b.WriteString("\n## Best Public Proof for GitHub\n\n")
	for _, item := range p.PublicFeatured {
		fmt.Fprintf(&b, "- [%s](%s): %s, %s\n", item.Repo.Name, item.Repo.HTMLURL, item.Domain, item.System)
	}

	b.WriteString("\n## Private/Internal Proof to Use in Resume and Interviews\n\n")
	for _, item := range firstN(privateRepos(p.CareerFeatured), 8) {
		fmt.Fprintf(&b, "- %s: %s with %s\n", publicSafeName(item.Repo), item.System, stackLabel(item.Repo))
	}
	if len(privateRepos(p.CareerFeatured)) == 0 {
		b.WriteString("- No private repositories ranked in the top career evidence set. Re-run after the improved scraper collects the full private/org inventory.\n")
	}

	b.WriteString("\n## Next Revision Targets\n\n")
	b.WriteString("1. Add human-written outcomes for the top 5 repositories.\n")
	b.WriteString("2. Mark which private projects can be described publicly without exposing confidential details.\n")
	b.WriteString("3. Create three resume variants: backend/platform, security/cloud, and full-stack/product.\n")
	b.WriteString("4. Move the final public GitHub README into the `dennislee928/dennislee928` profile repository.\n")
	return b.String()
}

func inferDomain(repo Repo) string {
	text := repoText(repo)
	switch {
	case hasAny(text, "security", "cyber", "firmware", "forensics", "yara", "vulnerability", "malware", "ids", "ips"):
		return "Security Engineering"
	case hasAny(text, "terraform", "iac", "cloud", "kubernetes", "k8s", "docker", "infra", "gateway", "devops"):
		return "Cloud and Platform Engineering"
	case hasAny(text, "ci", "cd", "pipeline", "github action", "automation", "workflow", "tooling"):
		return "Developer Automation and CI/CD"
	case hasAny(text, "web3", "ethereum", "blockchain", "crypto", "smart contract", "trading", "carbon"):
		return "FinTech, Web3, and Climate Systems"
	case hasAny(text, "ai", "ml", "llm", "agent", "model", "rag", "vector"):
		return "AI and Data Applications"
	case hasAny(text, "frontend", "react", "vue", "ui", "dashboard", "next", "web"):
		return "Full-Stack Product Engineering"
	case hasAny(text, "api", "backend", "grpc", "graphql", "service", "microservice"):
		return "Backend and API Systems"
	default:
		return "General Software"
	}
}

func inferSystem(repo Repo) string {
	text := repoText(repo)
	switch {
	case hasAny(text, "gateway", "grpc", "graphql", "rest", "api"):
		return "API boundary, service integration, and request orchestration"
	case hasAny(text, "ci", "cd", "pipeline", "github action", "workflow"):
		return "delivery pipeline, automation, and release workflow"
	case hasAny(text, "firmware", "forensics", "yara", "vulnerability", "security"):
		return "security analysis workflow and defensive engineering"
	case hasAny(text, "terraform", "kubernetes", "k8s", "docker", "cloud", "infra"):
		return "cloud runtime, container, and infrastructure management"
	case hasAny(text, "web3", "ethereum", "blockchain", "smart contract"):
		return "blockchain integration and decentralized app workflow"
	case hasAny(text, "react", "vue", "frontend", "dashboard", "ui"):
		return "frontend product workflow and user-facing interface"
	case hasAny(text, "ai", "ml", "llm", "agent", "model"):
		return "AI-enabled application workflow and data processing"
	default:
		return "application implementation and repository-level delivery"
	}
}

func inferSignals(repo Repo) []string {
	signals := []string{}
	text := repoText(repo)
	candidates := map[string][]string{
		"API":             {"api", "gateway", "graphql", "grpc", "rest"},
		"CI/CD":           {"ci", "cd", "pipeline", "workflow", "github action"},
		"Security":        {"security", "firmware", "forensics", "yara", "vulnerability"},
		"Cloud":           {"cloud", "terraform", "kubernetes", "docker", "infra"},
		"Product UI":      {"react", "vue", "frontend", "dashboard", "ui"},
		"Web3/FinTech":    {"web3", "ethereum", "blockchain", "crypto", "trading"},
		"AI/Data":         {"ai", "ml", "llm", "agent", "model"},
		"Documentation":   {"docs", "readme", "guide"},
		"Testing/Quality": {"test", "qa", "validation"},
	}
	for label, keys := range candidates {
		if hasAny(text, keys...) {
			signals = append(signals, label)
		}
	}
	return signals
}

func rankedLanguages(in Input, limit int) []string {
	score := map[string]int{}
	for lang, count := range in.Stats.LanguageRepoCount {
		score[lang] += count * 1000
	}
	for lang, bytes := range in.Stats.LanguageBytes {
		score[lang] += bytes / 1000
	}
	if len(score) == 0 {
		for _, repo := range in.Repos {
			if repo.Language != "" {
				score[repo.Language] += 1000
			}
		}
	}
	type kv struct {
		K string
		V int
	}
	pairs := make([]kv, 0, len(score))
	for k, v := range score {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].V > pairs[j].V })
	out := []string{}
	for i, p := range pairs {
		if i >= limit {
			break
		}
		out = append(out, p.K)
	}
	return out
}

func skillGroups(in Input, clusters []DomainCluster) map[string][]string {
	groups := map[string][]string{
		"Engineering": {"Software Engineering", "System Design", "Backend Development", "API Design", "Full-Stack Development"},
		"Delivery":    {"CI/CD", "DevOps", "GitHub Actions", "Automation", "Technical Documentation"},
		"Security":    {},
		"Cloud":       {},
		"Languages":   firstN(rankedLanguages(in, 10), 10),
		"Domains":     {},
	}
	for _, c := range clusters {
		switch c.Name {
		case "Security Engineering":
			groups["Security"] = appendUnique(groups["Security"], "Application Security", "Security Engineering", "Firmware Analysis", "Vulnerability Analysis")
		case "Cloud and Platform Engineering":
			groups["Cloud"] = appendUnique(groups["Cloud"], "Cloud Infrastructure", "Docker", "Kubernetes", "Infrastructure as Code")
		case "Developer Automation and CI/CD":
			groups["Delivery"] = appendUnique(groups["Delivery"], "Release Automation", "Build Pipelines", "Workflow Automation")
		default:
			groups["Domains"] = appendUnique(groups["Domains"], c.Name)
		}
	}
	return groups
}

func inferStats(repos []Repo) Stats {
	stats := Stats{
		LanguageRepoCount: map[string]int{},
		LanguageBytes:     map[string]int{},
		TopicCount:        map[string]int{},
		ReposBySource:     map[string]int{},
	}
	for _, repo := range repos {
		if repo.Private {
			stats.PrivateRepos++
		} else {
			stats.PublicRepos++
		}
		if repo.SourceType == "org" || repo.Owner.Type == "Organization" {
			stats.OrgRepos++
		} else if repo.SourceType == "collaborator" {
			stats.CollaboratorRepos++
		} else {
			stats.OwnerRepos++
		}
		if repo.Archived {
			stats.ArchivedRepos++
		}
		if repo.Fork {
			stats.ForkRepos++
		}
		if repo.Language != "" {
			stats.LanguageRepoCount[repo.Language]++
		}
		for lang, bytes := range repo.LanguageBytes {
			stats.LanguageBytes[lang] += bytes
		}
		for _, topic := range repo.Topics {
			stats.TopicCount[topic]++
		}
		if repo.Source != "" {
			stats.ReposBySource[repo.Source]++
		}
	}
	return stats
}

func repoText(repo Repo) string {
	return strings.ToLower(strings.Join([]string{
		repo.Name,
		repo.FullName,
		repo.Description,
		repo.Language,
		strings.Join(repo.Topics, " "),
	}, " "))
}

func hasAny(text string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func clusterSignal(c DomainCluster) string {
	if len(c.Systems) == 0 {
		return "application delivery"
	}
	return topMap(c.Systems, 2)
}

func topMap(m map[string]int, limit int) string {
	type kv struct {
		K string
		V int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].V == pairs[j].V {
			return pairs[i].K < pairs[j].K
		}
		return pairs[i].V > pairs[j].V
	})
	labels := []string{}
	for i, p := range pairs {
		if i >= limit {
			break
		}
		labels = append(labels, p.K)
	}
	if len(labels) == 0 {
		return "N/A"
	}
	return strings.Join(labels, ", ")
}

func repoList(items []ScoredRepo, limit int) string {
	names := []string{}
	for i, item := range items {
		if i >= limit {
			break
		}
		names = append(names, publicSafeName(item.Repo))
	}
	return strings.Join(names, ", ")
}

func privateRepos(items []ScoredRepo) []ScoredRepo {
	out := []ScoredRepo{}
	for _, item := range items {
		if item.Repo.Private {
			out = append(out, item)
		}
	}
	return out
}

func joinClusterNames(clusters []DomainCluster, limit int) string {
	names := []string{}
	for i, c := range clusters {
		if i >= limit {
			break
		}
		names = append(names, c.Name)
	}
	return strings.Join(names, ", ")
}

func orderedSkillGroups() []string {
	return []string{"Engineering", "Delivery", "Security", "Cloud", "Languages", "Domains"}
}

func appendUnique(items []string, values ...string) []string {
	seen := map[string]bool{}
	for _, item := range items {
		seen[strings.ToLower(item)] = true
	}
	for _, value := range values {
		key := strings.ToLower(value)
		if !seen[key] {
			items = append(items, value)
			seen[key] = true
		}
	}
	return items
}

func publicSafeName(repo Repo) string {
	if repo.Private {
		if repo.Source != "" {
			return "Private " + repo.Source + " repository"
		}
		return "Private repository"
	}
	return repo.Name
}

func repoLabel(repo Repo, includeLink bool) string {
	if repo.Private || !includeLink || repo.HTMLURL == "" {
		return publicSafeName(repo)
	}
	return fmt.Sprintf("[%s](%s)", repo.FullName, repo.HTMLURL)
}

func visibility(repo Repo) string {
	if repo.Visibility != "" {
		return repo.Visibility
	}
	if repo.Private {
		return "private"
	}
	return "public"
}

func stackLabel(repo Repo) string {
	if len(repo.LanguageBytes) > 0 {
		return topMap(repo.LanguageBytes, 3)
	}
	if repo.Language != "" {
		return repo.Language
	}
	return "N/A"
}

func evidenceLine(item ScoredRepo) string {
	parts := []string{}
	if item.Repo.Description != "" {
		parts = append(parts, truncate(cleanDesc(item.Repo.Description), 120))
	}
	if len(item.Signals) > 0 {
		parts = append(parts, "signals: "+strings.Join(firstN(item.Signals, 3), ", "))
	}
	if item.Repo.SourceType == "org" {
		parts = append(parts, "org repository")
	}
	if item.Repo.Private {
		parts = append(parts, "private/internal evidence")
	}
	if len(parts) == 0 {
		return "repository-level delivery evidence"
	}
	return strings.Join(parts, "; ")
}

func displayName(u User) string {
	if strings.TrimSpace(u.Name) != "" {
		return u.Name
	}
	return u.Login
}

func dateOrUnknown(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Format("2006-01-02")
}

func cleanDesc(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return strings.ReplaceAll(s, "|", "/")
	}
	return strings.ReplaceAll(s[:limit-3], "|", "/") + "..."
}
