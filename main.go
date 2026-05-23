// main.go
// Usage: GITHUB_PAT=<your_token> go run main.go
// Output: repos.json (or GITHUB_OUTPUT_FILE if set)

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const baseURL = "https://api.github.com"

type Repo struct {
	ID              int64          `json:"id"`
	Name            string         `json:"name"`
	FullName        string         `json:"full_name"`
	Description     string         `json:"description"`
	Private         bool           `json:"private"`
	Visibility      string         `json:"visibility"`
	Fork            bool           `json:"fork"`
	Language        string         `json:"language"`
	LanguagesURL    string         `json:"languages_url"`
	LanguageBytes   map[string]int `json:"language_bytes,omitempty"`
	Topics          []string       `json:"topics"`
	StargazersCount int            `json:"stargazers_count"`
	ForksCount      int            `json:"forks_count"`
	WatchersCount   int            `json:"watchers_count"`
	Size            int            `json:"size"`
	HTMLURL         string         `json:"html_url"`
	Homepage        string         `json:"homepage"`
	Archived        bool           `json:"archived"`
	PushedAt        time.Time      `json:"pushed_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	Owner           struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"owner"`
	Permissions struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
	Source     string   `json:"_source"`
	SourceType string   `json:"_source_type"`
	Sources    []string `json:"_sources,omitempty"`
}

type Org struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type User struct {
	Login           string    `json:"login"`
	Name            string    `json:"name"`
	Bio             string    `json:"bio"`
	Company         string    `json:"company"`
	Location        string    `json:"location"`
	Email           string    `json:"email"`
	Blog            string    `json:"blog"`
	TwitterUsername string    `json:"twitter_username"`
	PublicRepos     int       `json:"public_repos"`
	Followers       int       `json:"followers"`
	Following       int       `json:"following"`
	CreatedAt       time.Time `json:"created_at"`
}

type Stats struct {
	PublicRepos        int            `json:"public_repos"`
	PrivateRepos       int            `json:"private_repos"`
	OwnerRepos         int            `json:"owner_repos"`
	OrgRepos           int            `json:"org_repos"`
	CollaboratorRepos  int            `json:"collaborator_repos"`
	ArchivedRepos      int            `json:"archived_repos"`
	ForkRepos          int            `json:"fork_repos"`
	LanguageRepoCount  map[string]int `json:"language_repo_count"`
	LanguageBytes      map[string]int `json:"language_bytes"`
	TopicCount         map[string]int `json:"topic_count"`
	ReposBySource      map[string]int `json:"repos_by_source"`
	ReposBySourceType  map[string]int `json:"repos_by_source_type"`
	LanguageFetchError int            `json:"language_fetch_errors"`
}

type Output struct {
	User       User      `json:"user"`
	Repos      []Repo    `json:"repos"`
	TotalRepos int       `json:"total_repos"`
	Languages  map[string]int `json:"languages"`
	Topics     map[string]int `json:"topics"`
	OrgNames   []string  `json:"orgs"`
	Stats      Stats     `json:"stats"`
	ScrapedAt  time.Time `json:"scraped_at"`
}

var pat string
var client = &http.Client{Timeout: 45 * time.Second}

func apiGet(rawURL string, out interface{}) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+pat)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}

		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return fmt.Errorf("GitHub API rate limit reached; reset at unix timestamp %s", resp.Header.Get("X-RateLimit-Reset"))
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		return json.Unmarshal(body, out)
	}
	return lastErr
}

func pageURL(rawURL string, page int) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("per_page", "100")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func fetchAllPages[T any](rawURL string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		u, err := pageURL(rawURL, page)
		if err != nil {
			return nil, err
		}
		var items []T
		if err := apiGet(u, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		fmt.Fprintf(os.Stderr, "  fetched page %d (%d items so far)\n", page, len(all))
		if len(items) < 100 {
			break
		}
	}
	return all, nil
}

func main() {
	pat = os.Getenv("GITHUB_PAT")
	if pat == "" {
		fmt.Fprintln(os.Stderr, "Error: GITHUB_PAT environment variable not set")
		fmt.Fprintln(os.Stderr, "Usage: GITHUB_PAT=<token> go run main.go")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Fetching user info...")
	var user User
	if err := apiGet(baseURL+"/user", &user); err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching user: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Logged in as: %s (%s)\n", user.Login, user.Name)

	reposByKey := map[string]*Repo{}
	orgSet := map[string]bool{}

	fmt.Fprintln(os.Stderr, "\nFetching all accessible repos from /user/repos...")
	accessibleRepos, err := fetchAllPages[Repo](baseURL + "/user/repos?sort=pushed&direction=desc&affiliation=owner,collaborator,organization_member")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching accessible repos: %v\n", err)
		os.Exit(1)
	}
	for _, repo := range accessibleRepos {
		sourceType := classifySourceType(repo, user.Login)
		addRepo(reposByKey, repo, sourceType, repo.Owner.Login)
		if repo.Owner.Type == "Organization" {
			orgSet[repo.Owner.Login] = true
		}
	}
	fmt.Fprintf(os.Stderr, "Accessible repos: %d\n", len(accessibleRepos))

	fmt.Fprintln(os.Stderr, "\nFetching org memberships...")
	orgs, err := fetchAllPages[Org](baseURL + "/user/orgs")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching orgs: %v\n", err)
		os.Exit(1)
	}
	for _, org := range orgs {
		orgSet[org.Login] = true
	}

	orgNames := make([]string, 0, len(orgSet))
	for org := range orgSet {
		orgNames = append(orgNames, org)
	}
	sort.Strings(orgNames)
	fmt.Fprintf(os.Stderr, "Orgs discovered: %v\n", orgNames)

	for _, org := range orgNames {
		fmt.Fprintf(os.Stderr, "\nFetching repos for org: %s...\n", org)
		orgRepos, err := fetchAllPages[Repo](fmt.Sprintf("%s/orgs/%s/repos?type=all&sort=pushed&direction=desc", baseURL, org))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: error fetching org %s repos: %v\n", org, err)
			continue
		}
		for _, repo := range orgRepos {
			addRepo(reposByKey, repo, "org", org)
		}
		fmt.Fprintf(os.Stderr, "  Org %s repos visible to token: %d\n", org, len(orgRepos))
	}

	allRepos := make([]Repo, 0, len(reposByKey))
	for _, repo := range reposByKey {
		allRepos = append(allRepos, *repo)
	}
	sort.Slice(allRepos, func(i, j int) bool {
		return allRepos[i].PushedAt.After(allRepos[j].PushedAt)
	})

	fetchLanguages := strings.ToLower(os.Getenv("GITHUB_FETCH_LANGUAGES")) != "false"
	languageErrors := 0
	if fetchLanguages {
		fmt.Fprintln(os.Stderr, "\nFetching language breakdowns...")
		for i := range allRepos {
			if allRepos[i].LanguagesURL == "" {
				continue
			}
			var langBytes map[string]int
			if err := apiGet(allRepos[i].LanguagesURL, &langBytes); err != nil {
				languageErrors++
				fmt.Fprintf(os.Stderr, "  Warning: language fetch failed for %s: %v\n", allRepos[i].FullName, err)
				continue
			}
			allRepos[i].LanguageBytes = langBytes
			if (i+1)%25 == 0 || i == len(allRepos)-1 {
				fmt.Fprintf(os.Stderr, "  enriched %d/%d repos\n", i+1, len(allRepos))
			}
		}
	}

	stats := buildStats(allRepos, user.Login)
	stats.LanguageFetchError = languageErrors

	output := Output{
		User:       user,
		Repos:      allRepos,
		TotalRepos: len(allRepos),
		Languages:  stats.LanguageRepoCount,
		Topics:     stats.TopicCount,
		OrgNames:   orgNames,
		Stats:      stats,
		ScrapedAt:  time.Now(),
	}

	outFile := os.Getenv("GITHUB_OUTPUT_FILE")
	if outFile == "" {
		outFile = "repos.json"
	}
	if dir := filepath.Dir(outFile); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
			os.Exit(1)
		}
	}
	f, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nDone. Scraped %d total repos (%d public, %d private, %d org-owned)\n",
		len(allRepos), stats.PublicRepos, stats.PrivateRepos, stats.OrgRepos)
	fmt.Fprintf(os.Stderr, "Output written to: %s\n", outFile)

	summary := map[string]interface{}{
		"user":          user.Login,
		"name":          user.Name,
		"total_repos":   len(allRepos),
		"public_repos":  stats.PublicRepos,
		"private_repos": stats.PrivateRepos,
		"org_repos":     stats.OrgRepos,
		"orgs":          orgNames,
		"languages":     stats.LanguageRepoCount,
	}
	summaryEnc := json.NewEncoder(os.Stdout)
	summaryEnc.SetIndent("", "  ")
	summaryEnc.Encode(summary)
}

func addRepo(repos map[string]*Repo, repo Repo, sourceType string, source string) {
	if repo.Visibility == "" {
		if repo.Private {
			repo.Visibility = "private"
		} else {
			repo.Visibility = "public"
		}
	}
	repo.SourceType = sourceType
	repo.Source = source
	repo.Sources = []string{sourceType + ":" + source}

	key := repoKey(repo)
	existing, ok := repos[key]
	if !ok {
		cp := repo
		repos[key] = &cp
		return
	}
	if repo.PushedAt.After(existing.PushedAt) {
		existing.PushedAt = repo.PushedAt
	}
	if existing.SourceType == "" || existing.SourceType == "collaborator" {
		existing.SourceType = sourceType
		existing.Source = source
	}
	label := sourceType + ":" + source
	for _, existingLabel := range existing.Sources {
		if existingLabel == label {
			return
		}
	}
	existing.Sources = append(existing.Sources, label)
}

func repoKey(repo Repo) string {
	if repo.ID != 0 {
		return fmt.Sprintf("%d", repo.ID)
	}
	return strings.ToLower(repo.FullName)
}

func classifySourceType(repo Repo, userLogin string) string {
	if strings.EqualFold(repo.Owner.Login, userLogin) {
		return "owner"
	}
	if repo.Owner.Type == "Organization" {
		return "org"
	}
	return "collaborator"
}

func buildStats(repos []Repo, userLogin string) Stats {
	stats := Stats{
		LanguageRepoCount: map[string]int{},
		LanguageBytes:     map[string]int{},
		TopicCount:        map[string]int{},
		ReposBySource:     map[string]int{},
		ReposBySourceType: map[string]int{},
	}
	for _, repo := range repos {
		if repo.Private {
			stats.PrivateRepos++
		} else {
			stats.PublicRepos++
		}
		if repo.Archived {
			stats.ArchivedRepos++
		}
		if repo.Fork {
			stats.ForkRepos++
		}
		switch repo.SourceType {
		case "owner":
			stats.OwnerRepos++
		case "org":
			stats.OrgRepos++
		case "collaborator":
			stats.CollaboratorRepos++
		}
		if strings.EqualFold(repo.Owner.Login, userLogin) {
			stats.OwnerRepos++
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
		if repo.SourceType != "" {
			stats.ReposBySourceType[repo.SourceType]++
		}
	}
	return stats
}
