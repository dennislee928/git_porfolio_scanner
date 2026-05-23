// github_scraper.go
// Usage: GITHUB_PAT=<your_token> go run github_scraper.go
// Output: repos.json in the current directory

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"
)

const baseURL = "https://api.github.com"

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
	WatchersCount   int       `json:"watchers_count"`
	Size            int       `json:"size"`
	HTMLURL         string    `json:"html_url"`
	Homepage        string    `json:"homepage"`
	Archived        bool      `json:"archived"`
	PushedAt        time.Time `json:"pushed_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Owner           struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"owner"`
	Source string `json:"_source"` // "personal" or org name
}

type Org struct {
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

type Output struct {
	User       User           `json:"user"`
	Repos      []Repo         `json:"repos"`
	TotalRepos int            `json:"total_repos"`
	Languages  map[string]int `json:"languages"` // language -> repo count
	Topics     map[string]int `json:"topics"`    // topic -> repo count
	OrgNames   []string       `json:"orgs"`
	ScrapedAt  time.Time      `json:"scraped_at"`
}

var pat string
var client = &http.Client{Timeout: 30 * time.Second}

func apiGet(url string, out interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func fetchAllPages[T any](urlTemplate string) ([]T, error) {
	var all []T
	page := 1
	for {
		url := fmt.Sprintf("%s&page=%d&per_page=100", urlTemplate, page)
		var items []T
		if err := apiGet(url, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		fmt.Fprintf(os.Stderr, "  fetched page %d (%d items so far)\n", page, len(all))
		if len(items) < 100 {
			break
		}
		page++
	}
	return all, nil
}

func main() {
	pat = os.Getenv("GITHUB_PAT")
	if pat == "" {
		fmt.Fprintln(os.Stderr, "Error: GITHUB_PAT environment variable not set")
		fmt.Fprintln(os.Stderr, "Usage: GITHUB_PAT=<token> go run github_scraper.go")
		os.Exit(1)
	}

	// 1. Get authenticated user
	fmt.Fprintln(os.Stderr, "Fetching user info...")
	var user User
	if err := apiGet(baseURL+"/user", &user); err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching user: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Logged in as: %s (%s)\n", user.Login, user.Name)

	// 2. Fetch all personal repos
	fmt.Fprintln(os.Stderr, "\nFetching personal repos (public + private)...")
	// GitHub rejects combining `type` with `affiliation` on /user/repos.
	// `affiliation=owner` returns both public and private repos you own.
	personalRepos, err := fetchAllPages[Repo](baseURL + "/user/repos?sort=updated&affiliation=owner")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching personal repos: %v\n", err)
		os.Exit(1)
	}
	for i := range personalRepos {
		personalRepos[i].Source = "personal"
	}
	fmt.Fprintf(os.Stderr, "Personal repos: %d\n", len(personalRepos))

	// 3. Fetch all orgs
	fmt.Fprintln(os.Stderr, "\nFetching orgs...")
	orgs, err := fetchAllPages[Org](baseURL + "/user/orgs?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching orgs: %v\n", err)
		os.Exit(1)
	}
	orgNames := make([]string, len(orgs))
	for i, o := range orgs {
		orgNames[i] = o.Login
	}
	fmt.Fprintf(os.Stderr, "Orgs: %v\n", orgNames)

	// 4. Fetch all org repos
	allRepos := make([]Repo, 0, len(personalRepos))
	allRepos = append(allRepos, personalRepos...)

	seen := make(map[string]bool)
	for _, r := range personalRepos {
		seen[r.FullName] = true
	}

	for _, org := range orgs {
		fmt.Fprintf(os.Stderr, "\nFetching repos for org: %s...\n", org.Login)
		orgRepos, err := fetchAllPages[Repo](fmt.Sprintf("%s/orgs/%s/repos?type=all&sort=updated", baseURL, org.Login))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: error fetching org %s repos: %v\n", org.Login, err)
			continue
		}
		added := 0
		for i := range orgRepos {
			orgRepos[i].Source = org.Login
			if !seen[orgRepos[i].FullName] {
				seen[orgRepos[i].FullName] = true
				allRepos = append(allRepos, orgRepos[i])
				added++
			}
		}
		fmt.Fprintf(os.Stderr, "  Org %s: %d repos (%d new)\n", org.Login, len(orgRepos), added)
	}

	// 5. Analyze languages & topics
	fmt.Fprintln(os.Stderr, "\nAnalyzing data...")
	languages := make(map[string]int)
	topics := make(map[string]int)
	for _, r := range allRepos {
		if r.Language != "" {
			languages[r.Language]++
		}
		for _, t := range r.Topics {
			topics[t]++
		}
	}

	// Sort repos by pushed_at desc
	sort.Slice(allRepos, func(i, j int) bool {
		return allRepos[i].PushedAt.After(allRepos[j].PushedAt)
	})

	output := Output{
		User:       user,
		Repos:      allRepos,
		TotalRepos: len(allRepos),
		Languages:  languages,
		Topics:     topics,
		OrgNames:   orgNames,
		ScrapedAt:  time.Now(),
	}

	// 6. Write JSON output
	outFile := os.Getenv("GITHUB_OUTPUT_FILE")
	if outFile == "" {
		outFile = "repos.json"
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

	fmt.Fprintf(os.Stderr, "\n✅ Done! Scraped %d total repos\n", len(allRepos))
	fmt.Fprintf(os.Stderr, "Languages found: ")
	for lang, count := range languages {
		fmt.Fprintf(os.Stderr, "%s(%d) ", lang, count)
	}
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Output written to: %s\n", outFile)

	// Also print a summary to stdout for quick review
	summary := map[string]interface{}{
		"user":        user.Login,
		"name":        user.Name,
		"total_repos": len(allRepos),
		"languages":   languages,
		"topics":      topics,
		"orgs":        orgNames,
	}
	summaryEnc := json.NewEncoder(os.Stdout)
	summaryEnc.SetIndent("", "  ")
	summaryEnc.Encode(summary)
}
