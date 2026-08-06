//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/utility"
)

const (
	defaultGitHubBatchSize = 32
	githubItemsPerPage     = 100
	githubRequestTimeout   = 60 * time.Second
)

// GitHubConnector reads GitHub issues and pull requests.
type GitHubConnector struct {
	owner         string
	repos         []string
	includePRs    bool
	includeIssues bool
	token         string
	batchSize     int
	baseURL       string
	doJSON        func(ctx context.Context, apiURL string, out any) (http.Header, error)
}

// NewGitHubConnector creates a GitHub connector from Python-compatible config.
func NewGitHubConnector(config map[string]any) (*GitHubConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	token, _ := credentials["github_access_token"].(string)
	baseURL := strings.TrimRight(os.Getenv("GITHUB_CONNECTOR_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &GitHubConnector{
		owner:         strings.TrimSpace(stringConfig(config["repository_owner"])),
		repos:         splitGitHubRepos(stringConfig(config["repository_name"])),
		includePRs:    configBoolDefault(config["include_pull_requests"], true),
		includeIssues: configBoolDefault(config["include_issues"], true),
		token:         strings.TrimSpace(token),
		batchSize:     configInt(config["batch_size"], defaultGitHubBatchSize),
		baseURL:       baseURL,
	}, nil
}

// Validate validates GitHub connector settings and credentials.
func (c *GitHubConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("github connector is nil")
	}
	if c.owner == "" {
		return fmt.Errorf("Invalid connector settings: 'repo_owner' must be provided")
	}
	if c.token == "" {
		return fmt.Errorf("Missing github_access_token in credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	repos, err := c.listRepos(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("found no repos for GitHub owner %s", c.owner)
	}
	return nil
}

// OpenSync opens one GitHub sync session.
func (c *GitHubConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	documents, err := c.loadDocuments(ctx, request.WindowStart, request.WindowEnd)
	if err != nil {
		return nil, err
	}
	return &githubSyncSession{documents: documents, batchSize: c.batchSize}, nil
}

// OpenPrune opens one complete GitHub prune snapshot session.
func (c *GitHubConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	documents, err := c.loadDocuments(ctx, nil, time.Time{})
	if err != nil {
		return nil, err
	}
	slim := make([]SlimDocument, 0, len(documents))
	for _, doc := range documents {
		slim = append(slim, SlimDocument{SourceID: doc.SourceID})
	}
	return &githubPruneSession{documents: slim, batchSize: c.batchSize}, nil
}

// loadDocuments reads GitHub PRs and issues for the configured repos.
func (c *GitHubConnector) loadDocuments(ctx context.Context, windowStart *time.Time, windowEnd time.Time) ([]SourceDocument, error) {
	repos, err := c.listRepos(ctx)
	if err != nil {
		return nil, err
	}
	var documents []SourceDocument
	for _, repo := range repos {
		if c.includePRs {
			prs, err := c.listPullRequests(ctx, repo.FullName, windowStart, windowEnd)
			if err != nil {
				return nil, err
			}
			documents = append(documents, prs...)
		}
		if c.includeIssues {
			issues, err := c.listIssues(ctx, repo.FullName, windowStart, windowEnd)
			if err != nil {
				return nil, err
			}
			documents = append(documents, issues...)
		}
	}
	return documents, nil
}

// listRepos returns configured repositories or all owner repositories.
func (c *GitHubConnector) listRepos(ctx context.Context) ([]githubRepo, error) {
	if len(c.repos) > 0 {
		repos := make([]githubRepo, 0, len(c.repos))
		for _, repoName := range c.repos {
			var repo githubRepo
			if _, err := c.getJSON(ctx, c.apiURL("/repos/"+url.PathEscape(c.owner)+"/"+url.PathEscape(repoName), nil), &repo); err != nil {
				return nil, err
			}
			repos = append(repos, repo)
		}
		return repos, nil
	}

	repos, err := c.listRepoEndpoint(ctx, "/orgs/"+url.PathEscape(c.owner)+"/repos")
	if err == nil {
		return repos, nil
	}
	return c.listRepoEndpoint(ctx, "/users/"+url.PathEscape(c.owner)+"/repos")
}

// listRepoEndpoint returns all repos from an org or user endpoint.
func (c *GitHubConnector) listRepoEndpoint(ctx context.Context, path string) ([]githubRepo, error) {
	repos := []githubRepo{}
	for page := 1; ; page++ {
		query := url.Values{"per_page": {strconv.Itoa(githubItemsPerPage)}, "page": {strconv.Itoa(page)}}
		var batch []githubRepo
		headers, err := c.getJSON(ctx, c.apiURL(path, query), &batch)
		if err != nil {
			return nil, err
		}
		repos = append(repos, batch...)
		if !hasNextPage(headers) || len(batch) == 0 {
			break
		}
	}
	return repos, nil
}

// listPullRequests returns GitHub pull requests as source documents.
func (c *GitHubConnector) listPullRequests(ctx context.Context, fullName string, windowStart *time.Time, windowEnd time.Time) ([]SourceDocument, error) {
	documents := []SourceDocument{}
	for page := 1; ; page++ {
		query := url.Values{
			"state":     {"all"},
			"sort":      {"updated"},
			"direction": {"desc"},
			"per_page":  {strconv.Itoa(githubItemsPerPage)},
			"page":      {strconv.Itoa(page)},
		}
		var batch []githubPullRequest
		headers, err := c.getJSON(ctx, c.apiURL("/repos/"+fullName+"/pulls", query), &batch)
		if err != nil {
			return nil, err
		}
		stop := false
		for _, pr := range batch {
			if beforeOrAtWindowStart(pr.UpdatedAt, windowStart) {
				stop = true
				break
			}
			if afterWindowEnd(pr.UpdatedAt, windowEnd) {
				continue
			}
			documents = append(documents, pr.toSourceDocument(fullName))
		}
		if stop || !hasNextPage(headers) || len(batch) == 0 {
			break
		}
	}
	return documents, nil
}

// listIssues returns GitHub issues as source documents.
func (c *GitHubConnector) listIssues(ctx context.Context, fullName string, windowStart *time.Time, windowEnd time.Time) ([]SourceDocument, error) {
	documents := []SourceDocument{}
	for page := 1; ; page++ {
		query := url.Values{
			"state":     {"all"},
			"sort":      {"updated"},
			"direction": {"desc"},
			"per_page":  {strconv.Itoa(githubItemsPerPage)},
			"page":      {strconv.Itoa(page)},
		}
		var batch []githubIssue
		headers, err := c.getJSON(ctx, c.apiURL("/repos/"+fullName+"/issues", query), &batch)
		if err != nil {
			return nil, err
		}
		stop := false
		for _, issue := range batch {
			if issue.PullRequest != nil {
				continue
			}
			if beforeOrAtWindowStart(issue.UpdatedAt, windowStart) {
				stop = true
				break
			}
			if afterWindowEnd(issue.UpdatedAt, windowEnd) {
				continue
			}
			documents = append(documents, issue.toSourceDocument(fullName))
		}
		if stop || !hasNextPage(headers) || len(batch) == 0 {
			break
		}
	}
	return documents, nil
}

// getJSON fetches a GitHub API response into out.
func (c *GitHubConnector) getJSON(ctx context.Context, apiURL string, out any) (http.Header, error) {
	if c.doJSON != nil {
		return c.doJSON(ctx, apiURL, out)
	}
	hostname, resolvedIP, err := utility.AssertURLSafe(apiURL)
	if err != nil {
		return nil, err
	}
	client := utility.PinnedHTTPClient(hostname, resolvedIP, githubRequestTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GitHub API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err = json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, err
	}
	return resp.Header.Clone(), nil
}

// apiURL builds a GitHub API URL.
func (c *GitHubConnector) apiURL(path string, query url.Values) string {
	path = strings.TrimLeft(path, "/")
	u := strings.TrimRight(c.baseURL, "/") + "/" + path
	if len(query) == 0 {
		return u
	}
	return u + "?" + query.Encode()
}

type githubSyncSession struct {
	documents []SourceDocument
	batchSize int
	index     int
}

// NextBatch returns the next GitHub document batch.
func (s *githubSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.index >= len(s.documents) {
		return SyncBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batch := SyncBatch{Documents: s.documents[s.index:end]}
	s.index = end
	return batch, nil
}

// Close closes the GitHub sync session.
func (s *githubSyncSession) Close() error {
	return nil
}

type githubPruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

// NextBatch returns the next GitHub prune snapshot batch.
func (s *githubPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.index >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batch := PruneBatch{Documents: s.documents[s.index:end]}
	s.index = end
	return batch, nil
}

// Close closes the GitHub prune session.
func (s *githubPruneSession) Close() error {
	return nil
}

type githubRepo struct {
	FullName string `json:"full_name"`
}

type githubPullRequest struct {
	HTMLURL   string        `json:"html_url"`
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Body      string        `json:"body"`
	State     string        `json:"state"`
	MergedAt  *time.Time    `json:"merged_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	CreatedAt *time.Time    `json:"created_at"`
	ClosedAt  *time.Time    `json:"closed_at"`
	User      *githubUser   `json:"user"`
	Assignees []githubUser  `json:"assignees"`
	Labels    []githubLabel `json:"labels"`
}

// toSourceDocument converts a pull request into the syncer model.
func (p githubPullRequest) toSourceDocument(repo string) SourceDocument {
	body := []byte(p.Body)
	return SourceDocument{
		SourceID:           p.HTMLURL,
		SemanticIdentifier: fmt.Sprintf("%d:%s", p.Number, sanitizeGitHubName(p.Title, "md")),
		Extension:          ".md",
		Blob:               body,
		UpdatedAt:          p.UpdatedAt.UTC(),
		SizeBytes:          int64(len(body)),
		Metadata: map[string]any{
			"object_type": "PullRequest",
			"id":          strconv.Itoa(p.Number),
			"state":       p.State,
			"repo":        repo,
			"merged":      strconv.FormatBool(p.MergedAt != nil),
			"labels":      githubLabelNames(p.Labels),
			"user":        p.User.metadata(),
			"assignees":   githubUsersMetadata(p.Assignees),
		},
	}
}

type githubIssue struct {
	HTMLURL     string        `json:"html_url"`
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	Body        string        `json:"body"`
	State       string        `json:"state"`
	UpdatedAt   time.Time     `json:"updated_at"`
	CreatedAt   *time.Time    `json:"created_at"`
	ClosedAt    *time.Time    `json:"closed_at"`
	User        *githubUser   `json:"user"`
	Assignees   []githubUser  `json:"assignees"`
	Labels      []githubLabel `json:"labels"`
	PullRequest *struct{}     `json:"pull_request"`
}

// toSourceDocument converts an issue into the syncer model.
func (i githubIssue) toSourceDocument(repo string) SourceDocument {
	body := []byte(i.Body)
	return SourceDocument{
		SourceID:           i.HTMLURL,
		SemanticIdentifier: fmt.Sprintf("%d:%s", i.Number, sanitizeGitHubName(i.Title, "md")),
		Extension:          ".md",
		Blob:               body,
		UpdatedAt:          i.UpdatedAt.UTC(),
		SizeBytes:          int64(len(body)),
		Metadata: map[string]any{
			"object_type": "Issue",
			"id":          strconv.Itoa(i.Number),
			"state":       i.State,
			"repo":        repo,
			"labels":      githubLabelNames(i.Labels),
			"user":        i.User.metadata(),
			"assignees":   githubUsersMetadata(i.Assignees),
		},
	}
}

type githubUser struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// metadata returns Python-like user metadata.
func (u *githubUser) metadata() map[string]string {
	if u == nil {
		return nil
	}
	out := map[string]string{}
	if u.Login != "" {
		out["login"] = u.Login
	}
	if u.Name != "" {
		out["name"] = u.Name
	}
	if u.Email != "" {
		out["email"] = u.Email
	}
	return out
}

type githubLabel struct {
	Name string `json:"name"`
}

// githubLabelNames returns label names.
func githubLabelNames(labels []githubLabel) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		if label.Name != "" {
			out = append(out, label.Name)
		}
	}
	return out
}

// githubUsersMetadata returns metadata for users.
func githubUsersMetadata(users []githubUser) []map[string]string {
	out := make([]map[string]string, 0, len(users))
	for i := range users {
		out = append(out, (&users[i]).metadata())
	}
	return out
}

// hasNextPage reports whether a GitHub Link header has rel next.
func hasNextPage(headers http.Header) bool {
	return strings.Contains(headers.Get("Link"), `rel="next"`)
}

// beforeOrAtWindowStart reports whether an updated time is outside the lower bound.
func beforeOrAtWindowStart(updatedAt time.Time, windowStart *time.Time) bool {
	return windowStart != nil && !updatedAt.After(*windowStart)
}

// afterWindowEnd reports whether an updated time is outside the upper bound.
func afterWindowEnd(updatedAt time.Time, windowEnd time.Time) bool {
	return !windowEnd.IsZero() && updatedAt.After(windowEnd)
}

// splitGitHubRepos parses Python's comma-separated repository_name config.
func splitGitHubRepos(value string) []string {
	parts := strings.Split(value, ",")
	repos := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			repos = append(repos, part)
		}
	}
	return repos
}

// stringConfig reads a string config value.
func stringConfig(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// configBoolDefault reads a Python JSON bool/string flag with a default.
func configBoolDefault(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

// sanitizeGitHubName mirrors Python's sanitized markdown filename intent.
func sanitizeGitHubName(name, extension string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "github"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", `"`, "_", "<", "_", ">", "_", "|", "_")
	name = replacer.Replace(name)
	if !strings.HasSuffix(strings.ToLower(name), "."+extension) {
		name += "." + extension
	}
	return name
}
