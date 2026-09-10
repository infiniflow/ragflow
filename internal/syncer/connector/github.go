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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
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
		return fmt.Errorf("Invalid connector settings: 'repository_owner' must be provided")
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

// ValidateConnectorSetting validates GitHub settings from an unsaved config.
func (c *GitHubConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if c == nil {
		return &ConnectorValidationError{Message: "github connector is nil"}
	}
	if c.token == "" {
		return &ConnectorMissingCredentialError{Message: "Missing github_access_token in credentials"}
	}
	if c.owner == "" {
		return &ConnectorValidationError{Message: "Invalid connector settings: 'repo_owner' must be provided."}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}

	return c.validateOwner(ctx)
}

// validateOwner validates the owner as an organization or a user with repositories.
func (c *GitHubConnector) validateOwner(ctx context.Context) error {
	err := c.validateOrgRepos(ctx)
	if err == nil {
		return nil
	}
	var apiErr *githubAPIError
	if errors.As(err, &apiErr) {
		if isGitHubMissingSSO(apiErr.Message) {
			const ssoGuideLink = "https://docs.github.com/en/enterprise-cloud@latest/authentication/authenticating-with-saml-single-sign-on/authorizing-a-personal-access-token-for-use-with-saml-single-sign-on"
			return &ConnectorValidationError{Message: fmt.Sprintf("Your GitHub token is missing authorization to access the `%s` organization. Please follow the guide to authorize your token: %s", c.owner, ssoGuideLink)}
		}
		if apiErr.Status != http.StatusNotFound {
			return classifyGitHubError(apiErr)
		}
		// The owner is not accessible as an organization; try as a user.
		userErr := c.validateUserRepos(ctx)
		if userErr == nil {
			return nil
		}
		var userAPIErr *githubAPIError
		if errors.As(userErr, &userAPIErr) && userAPIErr.Status == http.StatusNotFound {
			return &ConnectorValidationError{Message: fmt.Sprintf("GitHub user or organization not found: %s", c.owner)}
		}
		return classifyGitHubError(userErr)
	}
	return classifyGitHubError(err)
}

// validateOrgRepos validates the owner as a GitHub organization with repositories.
func (c *GitHubConnector) validateOrgRepos(ctx context.Context) error {
	var org map[string]any
	if _, err := c.getJSON(ctx, c.apiURL("/orgs/"+url.PathEscape(c.owner), nil), &org); err != nil {
		return err
	}
	hasRepos, err := c.ownerHasRepos(ctx, "/orgs/"+url.PathEscape(c.owner)+"/repos")
	if err != nil {
		return err
	}
	if !hasRepos {
		return &ConnectorValidationError{Message: fmt.Sprintf("Found no repos for organization: %s. Does the credential have the right scopes?", c.owner)}
	}
	return nil
}

// validateUserRepos validates the owner as a GitHub user with repositories.
func (c *GitHubConnector) validateUserRepos(ctx context.Context) error {
	var user map[string]any
	if _, err := c.getJSON(ctx, c.apiURL("/users/"+url.PathEscape(c.owner), nil), &user); err != nil {
		return err
	}
	hasRepos, err := c.ownerHasRepos(ctx, "/users/"+url.PathEscape(c.owner)+"/repos")
	if err != nil {
		return err
	}
	if !hasRepos {
		return &ConnectorValidationError{Message: fmt.Sprintf("Found no repos for user: %s. Does the credential have the right scopes?", c.owner)}
	}
	return nil
}

// ownerHasRepos reports whether an org or user owner has at least one visible repository.
func (c *GitHubConnector) ownerHasRepos(ctx context.Context, path string) (bool, error) {
	query := url.Values{"per_page": {"1"}}
	var batch []githubRepo
	if _, err := c.getJSON(ctx, c.apiURL(path, query), &batch); err != nil {
		return false, err
	}
	return len(batch) > 0, nil
}

// classifyGitHubError maps a GitHub API error to a typed connector error.
func classifyGitHubError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *githubAPIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Status {
	case http.StatusUnauthorized:
		return &ConnectorMissingCredentialError{Message: "GitHub credential appears to be invalid or expired (HTTP 401)."}
	case http.StatusForbidden:
		return &ConnectorValidationError{Message: "Your GitHub token does not have sufficient permissions for this repository (HTTP 403)."}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("Unexpected GitHub error (status=%d): %s", apiErr.Status, apiErr.Message)}
	}
}

// isGitHubMissingSSO reports whether a GitHub API error body requires organization SSO authorization.
func isGitHubMissingSSO(body string) bool {
	return strings.Contains(strings.ToLower(body), "you must grant your personal access token access to this organization")
}

// OpenSync opens one GitHub sync session.
func (c *GitHubConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	repos, err := c.listRepos(ctx)
	if err != nil {
		return nil, err
	}
	session := &githubSyncSession{connector: c, repos: repos, batchSize: c.batchSize, stage: githubStagePRs, page: 1, windowStart: request.WindowStart, windowEnd: request.WindowEnd}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete GitHub prune snapshot session.
func (c *GitHubConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	repos, err := c.listRepos(ctx)
	if err != nil {
		return nil, err
	}
	return &githubPruneSession{connector: c, repos: repos, batchSize: c.batchSize, stage: githubStagePRs, page: 1}, nil
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

// listPullRequestPage returns one page of GitHub pull requests.
func (c *GitHubConnector) listPullRequestPage(ctx context.Context, fullName string, page, pageSize int, windowStart *time.Time, windowEnd time.Time) ([]githubBufferedDocument, bool, error) {
	var batch []githubPullRequest
	headers, err := c.getJSON(ctx, c.apiURL("/repos/"+fullName+"/pulls", githubListQuery(page, pageSize)), &batch)
	if err != nil {
		return nil, false, err
	}
	documents := make([]githubBufferedDocument, 0, len(batch))
	doneByWindow := false
	pageOffset := 0
	for _, pr := range batch {
		if beforeOrAtWindowStart(pr.UpdatedAt, windowStart) {
			doneByWindow = true
			break
		}
		if afterWindowEnd(pr.UpdatedAt, windowEnd) {
			continue
		}
		doc := pr.toSourceDocument(fullName)
		pageOffset++
		documents = append(documents, githubBufferedDocument{
			document:   doc,
			checkpoint: githubSyncCheckpoint(fullName, githubStagePRs, page, pageOffset, doc),
			offset:     pageOffset,
			sourceID:   doc.SourceID,
		})
	}
	done := doneByWindow || !hasNextPage(headers) || len(batch) == 0
	return documents, done, nil
}

// listIssuePage returns one page of GitHub issues.
func (c *GitHubConnector) listIssuePage(ctx context.Context, fullName string, page, pageSize int, windowStart *time.Time, windowEnd time.Time) ([]githubBufferedDocument, bool, error) {
	var batch []githubIssue
	headers, err := c.getJSON(ctx, c.apiURL("/repos/"+fullName+"/issues", githubListQuery(page, pageSize)), &batch)
	if err != nil {
		return nil, false, err
	}
	documents := make([]githubBufferedDocument, 0, len(batch))
	doneByWindow := false
	pageOffset := 0
	for _, issue := range batch {
		if issue.PullRequest != nil {
			continue
		}
		if beforeOrAtWindowStart(issue.UpdatedAt, windowStart) {
			doneByWindow = true
			break
		}
		if afterWindowEnd(issue.UpdatedAt, windowEnd) {
			continue
		}
		doc := issue.toSourceDocument(fullName)
		pageOffset++
		documents = append(documents, githubBufferedDocument{
			document:   doc,
			checkpoint: githubSyncCheckpoint(fullName, githubStageIssues, page, pageOffset, doc),
			offset:     pageOffset,
			sourceID:   doc.SourceID,
		})
	}
	done := doneByWindow || !hasNextPage(headers) || len(batch) == 0
	return documents, done, nil
}

// listPullRequestSlimPage returns one page of GitHub pull request IDs.
func (c *GitHubConnector) listPullRequestSlimPage(ctx context.Context, fullName string, page, pageSize int) ([]SlimDocument, bool, error) {
	var batch []githubPullRequest
	headers, err := c.getJSON(ctx, c.apiURL("/repos/"+fullName+"/pulls", githubListQuery(page, pageSize)), &batch)
	if err != nil {
		return nil, false, err
	}
	documents := make([]SlimDocument, 0, len(batch))
	for _, pr := range batch {
		documents = append(documents, SlimDocument{SourceID: pr.HTMLURL})
	}
	return documents, !hasNextPage(headers) || len(batch) == 0, nil
}

// listIssueSlimPage returns one page of GitHub issue IDs.
func (c *GitHubConnector) listIssueSlimPage(ctx context.Context, fullName string, page, pageSize int) ([]SlimDocument, bool, error) {
	var batch []githubIssue
	headers, err := c.getJSON(ctx, c.apiURL("/repos/"+fullName+"/issues", githubListQuery(page, pageSize)), &batch)
	if err != nil {
		return nil, false, err
	}
	documents := make([]SlimDocument, 0, len(batch))
	for _, issue := range batch {
		if issue.PullRequest != nil {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: issue.HTMLURL})
	}
	return documents, !hasNextPage(headers) || len(batch) == 0, nil
}

// githubListQuery builds standard GitHub list query parameters.
func githubListQuery(page, pageSize int) url.Values {
	if pageSize <= 0 {
		pageSize = defaultGitHubBatchSize
	}
	return url.Values{
		"state":     {"all"},
		"sort":      {"updated"},
		"direction": {"desc"},
		"per_page":  {strconv.Itoa(pageSize)},
		"page":      {strconv.Itoa(page)},
	}
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
		return nil, &githubAPIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
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
	connector      *GitHubConnector
	repos          []githubRepo
	repoIndex      int
	stage          string
	page           int
	batchSize      int
	windowStart    *time.Time
	windowEnd      time.Time
	buffer         []githubBufferedDocument
	resumeRepo     string
	resumeStage    string
	resumePage     int
	resumeOffset   int
	resumeSourceID string
}

// NextBatch returns the next GitHub document batch.
func (s *githubSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	var checkpoint *SyncCheckpoint
	if len(s.buffer) > 0 {
		n := s.batchSize
		if n > len(s.buffer) {
			n = len(s.buffer)
		}
		for _, buffered := range s.buffer[:n] {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
		s.buffer = s.buffer[n:]
	}

	for len(documents) < s.batchSize {
		if s.repoIndex >= len(s.repos) {
			if len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			break
		}
		batch, err := s.nextDocumentPage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		remaining := s.batchSize - len(documents)
		if len(batch) > remaining {
			for _, buffered := range batch[:remaining] {
				documents = append(documents, buffered.document)
				checkpoint = buffered.checkpoint
			}
			s.buffer = append(s.buffer, batch[remaining:]...)
			break
		}
		for _, buffered := range batch {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close closes the GitHub sync session.
func (s *githubSyncSession) Close() error {
	return nil
}

type githubPruneSession struct {
	connector *GitHubConnector
	repos     []githubRepo
	repoIndex int
	stage     string
	page      int
	batchSize int
	buffer    []SlimDocument
}

// NextBatch returns the next GitHub prune snapshot batch.
func (s *githubPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := s.batchSize
		if n > len(s.buffer) {
			n = len(s.buffer)
		}
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.repoIndex >= len(s.repos) {
			if len(documents) == 0 {
				return PruneBatch{}, io.EOF
			}
			break
		}
		batch, err := s.nextSlimPage(ctx)
		if err != nil {
			return PruneBatch{}, err
		}
		remaining := s.batchSize - len(documents)
		if len(batch) > remaining {
			documents = append(documents, batch[:remaining]...)
			s.buffer = append(s.buffer, batch[remaining:]...)
			break
		}
		documents = append(documents, batch...)
	}
	return PruneBatch{Documents: documents}, nil
}

// Close closes the GitHub prune session.
func (s *githubPruneSession) Close() error {
	return nil
}

const (
	githubStagePRs    = "prs"
	githubStageIssues = "issues"
)

// nextDocumentPage fetches one GitHub API page for sync.
func (s *githubSyncSession) nextDocumentPage(ctx context.Context) ([]githubBufferedDocument, error) {
	repo := s.repos[s.repoIndex]
	switch s.stage {
	case githubStagePRs:
		if !s.connector.includePRs {
			s.advanceStage()
			return nil, nil
		}
		docs, done, err := s.connector.listPullRequestPage(ctx, repo.FullName, s.page, s.batchSize, s.windowStart, s.windowEnd)
		if err != nil {
			return nil, err
		}
		docs, err = s.filterResumedDocuments(repo.FullName, githubStagePRs, s.page, docs)
		if err != nil {
			return nil, err
		}
		if done {
			s.advanceStage()
		} else {
			s.page++
		}
		return docs, nil
	case githubStageIssues:
		if !s.connector.includeIssues {
			s.advanceRepo()
			return nil, nil
		}
		docs, done, err := s.connector.listIssuePage(ctx, repo.FullName, s.page, s.batchSize, s.windowStart, s.windowEnd)
		if err != nil {
			return nil, err
		}
		docs, err = s.filterResumedDocuments(repo.FullName, githubStageIssues, s.page, docs)
		if err != nil {
			return nil, err
		}
		if done {
			s.advanceRepo()
		} else {
			s.page++
		}
		return docs, nil
	default:
		s.advanceRepo()
		return nil, nil
	}
}

// nextSlimPage fetches one GitHub API page for prune.
func (s *githubPruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	repo := s.repos[s.repoIndex]
	switch s.stage {
	case githubStagePRs:
		if !s.connector.includePRs {
			s.advanceStage()
			return nil, nil
		}
		docs, done, err := s.connector.listPullRequestSlimPage(ctx, repo.FullName, s.page, s.batchSize)
		if err != nil {
			return nil, err
		}
		if done {
			s.advanceStage()
		} else {
			s.page++
		}
		return docs, nil
	case githubStageIssues:
		if !s.connector.includeIssues {
			s.advanceRepo()
			return nil, nil
		}
		docs, done, err := s.connector.listIssueSlimPage(ctx, repo.FullName, s.page, s.batchSize)
		if err != nil {
			return nil, err
		}
		if done {
			s.advanceRepo()
		} else {
			s.page++
		}
		return docs, nil
	default:
		s.advanceRepo()
		return nil, nil
	}
}

// advanceStage moves a GitHub session from PRs to issues.
func (s *githubSyncSession) advanceStage() {
	s.stage = githubStageIssues
	s.page = 1
	s.clearResume()
}

// advanceRepo moves a GitHub session to the next repository.
func (s *githubSyncSession) advanceRepo() {
	s.repoIndex++
	s.stage = githubStagePRs
	s.page = 1
	s.clearResume()
}

// applyResume advances a sync session to the last committed GitHub page.
func (s *githubSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}

	if checkpoint.Cursor == "" {
		return fmt.Errorf("github sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor githubSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("github sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	if cursor.Repo == "" || cursor.Stage == "" || cursor.Page <= 0 {
		return fmt.Errorf("github sync cursor has no resume anchor: %w", ErrSyncResumeInvalid)
	}
	for index, repo := range s.repos {
		if repo.FullName != cursor.Repo {
			continue
		}
		s.repoIndex = index
		s.stage = cursor.Stage
		s.page = cursor.Page
		s.resumeRepo = cursor.Repo
		s.resumeStage = cursor.Stage
		s.resumePage = cursor.Page
		s.resumeOffset = cursor.Offset
		s.resumeSourceID = firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
		if s.resumeSourceID == "" {
			return fmt.Errorf("github sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
		}
		return nil
	}
	return fmt.Errorf("github resume repo %q was not found in the current listing: %w", cursor.Repo, ErrSyncResumeInvalid)
}

// filterResumedDocuments drops documents through the committed checkpoint.
func (s *githubSyncSession) filterResumedDocuments(repo, stage string, page int, candidates []githubBufferedDocument) ([]githubBufferedDocument, error) {
	if s.resumeRepo == "" || repo != s.resumeRepo || stage != s.resumeStage || page != s.resumePage {
		return candidates, nil
	}
	if s.resumeSourceID != "" {
		for index, candidate := range candidates {
			if candidate.sourceID == s.resumeSourceID {
				s.clearResume()
				return candidates[index+1:], nil
			}
		}
		return nil, fmt.Errorf("github resume anchor %q was not found on page %d: %w", s.resumeSourceID, page, ErrSyncResumeInvalid)
	}
	return nil, fmt.Errorf("github sync cursor has no source anchor: %w", ErrSyncResumeInvalid)
}

func (s *githubSyncSession) clearResume() {
	s.resumeRepo = ""
	s.resumeStage = ""
	s.resumePage = 0
	s.resumeOffset = 0
	s.resumeSourceID = ""
}

// advanceStage moves a GitHub prune session from PRs to issues.
func (s *githubPruneSession) advanceStage() {
	s.stage = githubStageIssues
	s.page = 1
}

// advanceRepo moves a GitHub prune session to the next repository.
func (s *githubPruneSession) advanceRepo() {
	s.repoIndex++
	s.stage = githubStagePRs
	s.page = 1
}

type githubRepo struct {
	FullName string `json:"full_name"`
}

// githubAPIError is a GitHub API error with its HTTP status.
type githubAPIError struct {
	Status  int
	Message string
}

func (e *githubAPIError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("GitHub API returned HTTP %d: %s", e.Status, e.Message)
}

type githubSyncCursor struct {
	Repo     string `json:"repo"`
	Stage    string `json:"stage"`
	Page     int    `json:"page"`
	Offset   int    `json:"offset"`
	SourceID string `json:"source_id"`
}

type githubBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
	sourceID   string
}

func githubSyncCheckpoint(repo, stage string, page, offset int, doc SourceDocument) *SyncCheckpoint {
	cursor, err := json.Marshal(githubSyncCursor{
		Repo:     repo,
		Stage:    stage,
		Page:     page,
		Offset:   offset,
		SourceID: doc.SourceID,
	})
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    string(cursor),
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
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
	labels := githubLabelNames(p.Labels)
	user := p.User.metadata()
	assignees := githubUsersMetadata(p.Assignees)
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
			"labels":      labels,
			"user":        user,
			"assignees":   assignees,
		},
		Fingerprint: stableFingerprint(map[string]any{
			"type":       "PullRequest",
			"url":        p.HTMLURL,
			"title":      p.Title,
			"body":       p.Body,
			"state":      p.State,
			"updated_at": p.UpdatedAt.UTC(),
			"merged_at":  p.MergedAt,
			"labels":     labels,
			"user":       user,
			"assignees":  assignees,
		}),
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
	labels := githubLabelNames(i.Labels)
	user := i.User.metadata()
	assignees := githubUsersMetadata(i.Assignees)
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
			"labels":      labels,
			"user":        user,
			"assignees":   assignees,
		},
		Fingerprint: stableFingerprint(map[string]any{
			"type":       "Issue",
			"url":        i.HTMLURL,
			"title":      i.Title,
			"body":       i.Body,
			"state":      i.State,
			"updated_at": i.UpdatedAt.UTC(),
			"closed_at":  i.ClosedAt,
			"labels":     labels,
			"user":       user,
			"assignees":  assignees,
		}),
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
	sort.Strings(out)
	return out
}

// githubUsersMetadata returns metadata for users.
func githubUsersMetadata(users []githubUser) []map[string]string {
	users = append([]githubUser(nil), users...)
	sort.Slice(users, func(i, j int) bool {
		return githubUserSortKey(users[i]) < githubUserSortKey(users[j])
	})
	out := make([]map[string]string, 0, len(users))
	for i := range users {
		out = append(out, (&users[i]).metadata())
	}
	return out
}

func githubUserSortKey(user githubUser) string {
	if user.Login != "" {
		return user.Login
	}
	if user.Email != "" {
		return user.Email
	}
	return user.Name
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

// sanitizeGitHubName mirrors Python's sanitized Markdown filename intent.
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
