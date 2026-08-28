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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/utility"
)

const (
	defaultBitbucketBatchSize = 32
	bitbucketRequestTimeout   = 60 * time.Second
	bitbucketPRPageSize       = 50
	bitbucketRepoPageSize     = 100
	bitbucketBaseURL          = "https://api.bitbucket.org/2.0"
)

var (
	bitbucketRetryTries     = 6
	bitbucketRetryBaseDelay = 1 * time.Second
	bitbucketRetryBackoff   = 2.0
	bitbucketRetryMaxDelay  = 30 * time.Second
	bitbucketSpaceRE        = regexp.MustCompile(`\s+`)
)

const bitbucketPRFields = "next,page,pagelen,values.author,values.close_source_branch,values.closed_by,values.comment_count,values.created_on,values.description,values.destination,values.draft,values.id,values.links,values.merge_commit,values.participants,values.reason,values.rendered,values.reviewers,values.source,values.state,values.summary,values.task_count,values.title,values.type,values.updated_on"

const bitbucketSlimPRFields = "next,page,pagelen,values.id"

const bitbucketRepoFields = "next,page,pagelen,values.slug,values.full_name,values.project.key"

// BitbucketConnector reads Bitbucket Cloud pull requests.
type BitbucketConnector struct {
	workspace       string
	repositorySlugs []string
	projects        []string
	email           string
	apiToken        string
	batchSize       int
	baseURL         string
	doJSON          func(ctx context.Context, apiURL string, out any) (http.Header, error)
}

// NewBitbucketConnector creates a Bitbucket connector from config.
func NewBitbucketConnector(config map[string]any) (*BitbucketConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	return &BitbucketConnector{
		workspace:       strings.TrimSpace(stringConfig(config["workspace"])),
		repositorySlugs: splitBitbucketList(stringConfig(config["repository_slugs"])),
		projects:        splitBitbucketList(stringConfig(config["projects"])),
		email:           strings.TrimSpace(stringConfig(credentials["bitbucket_account_email"])),
		apiToken:        strings.TrimSpace(stringConfig(credentials["bitbucket_api_token"])),
		batchSize:       configInt(config["batch_size"], defaultBitbucketBatchSize),
		baseURL:         bitbucketBaseURL,
	}, nil
}

// Validate validates Bitbucket connector settings and credentials.
func (c *BitbucketConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("bitbucket connector is nil")
	}
	if c.workspace == "" {
		return fmt.Errorf("Invalid connector settings: 'workspace' must be provided")
	}
	if c.email == "" {
		return fmt.Errorf("Missing bitbucket_account_email in credentials")
	}
	if c.apiToken == "" {
		return fmt.Errorf("Missing bitbucket_api_token in credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	repos, err := c.listRepos(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("found no repositories for Bitbucket workspace %s", c.workspace)
	}
	return nil
}

// ValidateConnectorSetting validates Bitbucket settings from an unsaved config.
func (c *BitbucketConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if c == nil {
		return fmt.Errorf("bitbucket connector is nil")
	}
	if c.workspace == "" {
		return fmt.Errorf("Invalid connector settings: 'workspace' must be provided")
	}
	if c.email == "" {
		return fmt.Errorf("Missing bitbucket_account_email in credentials")
	}
	if c.apiToken == "" {
		return fmt.Errorf("Missing bitbucket_api_token in credentials")
	}
	var page map[string]any
	_, err := c.getJSON(ctx, c.apiURL("/repositories/"+url.PathEscape(c.workspace), url.Values{
		"pagelen": {"1"},
		"fields":  {"pagelen"},
	}), &page)
	if err != nil {
		var httpErr *bitbucketHTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.Status {
			case http.StatusUnauthorized:
				return fmt.Errorf("Invalid or expired Bitbucket credentials (HTTP 401).")
			case http.StatusForbidden:
				return fmt.Errorf("Insufficient permissions to access Bitbucket workspace (HTTP 403).")
			}
			return err
		}
		return err
	}
	return nil
}

// OpenSync opens one Bitbucket sync session.
func (c *BitbucketConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	repos, err := c.listRepos(ctx)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("found no repositories for Bitbucket workspace %s", c.workspace)
	}
	session := &bitbucketSyncSession{
		connector:   c,
		repos:       repos,
		batchSize:   c.batchSize,
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Bitbucket prune snapshot session.
func (c *BitbucketConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	repos, err := c.listRepos(ctx)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("found no repositories for Bitbucket workspace %s", c.workspace)
	}
	return &bitbucketPruneSession{
		connector: c,
		repos:     repos,
		batchSize: c.batchSize,
	}, nil
}

// listRepos returns deterministically ordered target repository slugs.
func (c *BitbucketConnector) listRepos(ctx context.Context) ([]string, error) {
	var repos []string
	if len(c.repositorySlugs) > 0 {
		repos = append(repos, c.repositorySlugs...)
	} else if len(c.projects) > 0 {
		for _, project := range c.projects {
			projectRepos, err := c.listWorkspaceRepos(ctx, project)
			if err != nil {
				return nil, err
			}
			repos = append(repos, projectRepos...)
		}
	} else {
		var err error
		repos, err = c.listWorkspaceRepos(ctx, "")
		if err != nil {
			return nil, err
		}
	}
	return uniqueSortedStrings(repos), nil
}

// listWorkspaceRepos lists workspace repositories, optionally filtered by project key.
func (c *BitbucketConnector) listWorkspaceRepos(ctx context.Context, project string) ([]string, error) {
	query := bitbucketRepoListQuery()
	if project != "" {
		query.Set("q", fmt.Sprintf("project.key=%q", project))
	}
	var repos []string
	for pageURL := ""; ; {
		if pageURL == "" {
			pageURL = c.apiURL("/repositories/"+url.PathEscape(c.workspace), query)
		}
		var page bitbucketRepositoryPage
		if _, err := c.getJSON(ctx, pageURL, &page); err != nil {
			return nil, err
		}
		for _, repo := range page.Values {
			if repo.Slug != "" {
				repos = append(repos, repo.Slug)
			}
		}
		if page.Next == "" || len(page.Values) == 0 {
			break
		}
		if err := c.validateBitbucketHost(page.Next); err != nil {
			return nil, err
		}
		pageURL = page.Next
	}
	return repos, nil
}

// listPullRequestPage returns one Bitbucket pull request page.
func (c *BitbucketConnector) listPullRequestPage(ctx context.Context, repo, pageURL string, windowStart *time.Time, windowEnd time.Time) (bitbucketPullRequestPage, error) {
	if pageURL == "" {
		pageURL = c.apiURL("/repositories/"+url.PathEscape(c.workspace)+"/"+url.PathEscape(repo)+"/pullrequests", bitbucketPRListQuery(windowStart, windowEnd))
	}
	if err := c.validateBitbucketHost(pageURL); err != nil {
		return bitbucketPullRequestPage{}, err
	}
	var page bitbucketPullRequestPage
	if _, err := c.getJSON(ctx, pageURL, &page); err != nil {
		return bitbucketPullRequestPage{}, err
	}
	return page, nil
}

// listSlimPullRequestPage returns one Bitbucket pull request page for prune snapshots.
func (c *BitbucketConnector) listSlimPullRequestPage(ctx context.Context, repo, pageURL string) (bitbucketPullRequestPage, error) {
	if pageURL == "" {
		query := url.Values{
			"fields":  {bitbucketSlimPRFields},
			"pagelen": {strconv.Itoa(bitbucketPRPageSize)},
			"sort":    {"updated_on"},
			"q":       {bitbucketPullRequestStateQuery()},
		}
		pageURL = c.apiURL("/repositories/"+url.PathEscape(c.workspace)+"/"+url.PathEscape(repo)+"/pullrequests", query)
	}
	if err := c.validateBitbucketHost(pageURL); err != nil {
		return bitbucketPullRequestPage{}, err
	}
	var page bitbucketPullRequestPage
	if _, err := c.getJSON(ctx, pageURL, &page); err != nil {
		return bitbucketPullRequestPage{}, err
	}
	return page, nil
}

// validateBitbucketHost rejects a server-supplied pagination URL whose host does
// not match the configured Bitbucket host, so authenticated requests cannot be
// redirected at an attacker-controlled host.
func (c *BitbucketConnector) validateBitbucketHost(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("bitbucket: invalid pagination URL: %w", err)
	}
	base, err := url.Parse(strings.TrimSpace(c.baseURL))
	if err != nil {
		return fmt.Errorf("bitbucket: invalid configured base URL: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, base.Scheme) {
		return fmt.Errorf("bitbucket: pagination URL scheme %q does not match configured scheme %q", parsed.Scheme, base.Scheme)
	}
	if !strings.EqualFold(parsed.Hostname(), base.Hostname()) {
		return fmt.Errorf("bitbucket: pagination URL host %q does not match configured host %q", parsed.Hostname(), base.Hostname())
	}
	if effectiveBitbucketPort(parsed) != effectiveBitbucketPort(base) {
		return fmt.Errorf("bitbucket: pagination URL port %q does not match configured port %q", effectiveBitbucketPort(parsed), effectiveBitbucketPort(base))
	}
	return nil
}

// effectiveBitbucketPort returns the explicit port, falling back to the
// scheme's default port so an explicit default port still matches.
func effectiveBitbucketPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	}
	return ""
}

// apiURL builds a Bitbucket API URL.
func (c *BitbucketConnector) apiURL(path string, query url.Values) string {
	path = strings.TrimLeft(path, "/")
	u := strings.TrimRight(c.baseURL, "/") + "/" + path
	if len(query) == 0 {
		return u
	}
	return u + "?" + query.Encode()
}

// getJSON fetches a Bitbucket API JSON response into out.
func (c *BitbucketConnector) getJSON(ctx context.Context, apiURL string, out any) (http.Header, error) {
	if c.doJSON != nil {
		return c.doJSON(ctx, apiURL, out)
	}
	return c.getJSONWithRetry(ctx, apiURL, out)
}

func (c *BitbucketConnector) getJSONWithRetry(ctx context.Context, apiURL string, out any) (http.Header, error) {
	delay := bitbucketRetryBaseDelay
	var lastErr error
	for attempt := 0; attempt < bitbucketRetryTries; attempt++ {
		resp, err := c.get(ctx, apiURL)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			if attempt == bitbucketRetryTries-1 {
				break
			}
			if err := sleepFor(ctx, delay); err != nil {
				return nil, err
			}
			delay = bitbucketNextDelay(delay)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := bitbucketRetryAfter(resp)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = &bitbucketHTTPError{Status: resp.StatusCode}
			if attempt == bitbucketRetryTries-1 {
				break
			}
			wait := retryAfter
			if wait <= 0 {
				wait = delay
			}
			if err := sleepFor(ctx, wait); err != nil {
				return nil, err
			}
			delay = bitbucketNextDelay(delay)
			continue
		}

		if resp.StatusCode >= http.StatusInternalServerError {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = &bitbucketHTTPError{Status: resp.StatusCode, Body: string(body)}
			if attempt == bitbucketRetryTries-1 {
				break
			}
			if err := sleepFor(ctx, delay); err != nil {
				return nil, err
			}
			delay = bitbucketNextDelay(delay)
			continue
		}

		if resp.StatusCode >= http.StatusBadRequest {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, &bitbucketHTTPError{Status: resp.StatusCode, Body: string(body)}
		}

		headers := resp.Header.Clone()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		return headers, nil
	}
	return nil, lastErr
}

// get performs one SSRF-safe authenticated GET request.
func (c *BitbucketConnector) get(ctx context.Context, apiURL string) (*http.Response, error) {
	hostname, resolvedIP, err := utility.AssertURLSafe(apiURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.email, c.apiToken)
	client := utility.PinnedHTTPClient(hostname, resolvedIP, bitbucketRequestTimeout)
	return client.Do(req)
}

func sleepFor(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

func bitbucketNextDelay(delay time.Duration) time.Duration {
	next := time.Duration(float64(delay) * bitbucketRetryBackoff)
	if next > bitbucketRetryMaxDelay {
		return bitbucketRetryMaxDelay
	}
	return next
}

func bitbucketRetryAfter(resp *http.Response) time.Duration {
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds < 0 {
		return 0
	}
	wait := time.Duration(seconds * float64(time.Second))
	if wait > bitbucketRetryMaxDelay {
		return bitbucketRetryMaxDelay
	}
	return wait
}

func bitbucketPullRequestStateQuery() string {
	return `(state = "OPEN" OR state = "MERGED" OR state = "DECLINED")`
}

func bitbucketPRListQuery(windowStart *time.Time, windowEnd time.Time) url.Values {
	query := url.Values{
		"fields":  {bitbucketPRFields},
		"pagelen": {strconv.Itoa(bitbucketPRPageSize)},
		"sort":    {"updated_on"},
	}
	stateQuery := bitbucketPullRequestStateQuery()
	if windowStart != nil && !windowEnd.IsZero() {
		query.Set("q", fmt.Sprintf("%s AND (updated_on > %q AND updated_on <= %q)", stateQuery, bitbucketISO(*windowStart), bitbucketISO(windowEnd)))
	} else {
		query.Set("q", stateQuery)
	}
	return query
}

func bitbucketRepoListQuery() url.Values {
	return url.Values{
		"fields":  {bitbucketRepoFields},
		"pagelen": {strconv.Itoa(bitbucketRepoPageSize)},
		"sort":    {"full_name"},
	}
}

func bitbucketISO(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

type bitbucketHTTPError struct {
	Status int
	Body   string
}

func (e *bitbucketHTTPError) Error() string {
	if strings.TrimSpace(e.Body) != "" {
		return fmt.Sprintf("Bitbucket API returned HTTP %d: %s", e.Status, strings.TrimSpace(e.Body))
	}
	return fmt.Sprintf("Bitbucket API returned HTTP %d", e.Status)
}

type bitbucketSyncSession struct {
	connector     *BitbucketConnector
	repos         []string
	repoIndex     int
	pageURL       string
	batchSize     int
	windowStart   *time.Time
	windowEnd     time.Time
	buffer        []bitbucketBufferedDocument
	resumeRepo    string
	resumePageURL string
	resumeOffset  int
	resumeSource  string
}

// NextBatch returns the next Bitbucket document batch.
func (s *bitbucketSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
		if len(batch) == 0 {
			if s.repoIndex >= len(s.repos) && len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			continue
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

// Close closes the Bitbucket sync session.
func (s *bitbucketSyncSession) Close() error {
	return nil
}

func (s *bitbucketSyncSession) nextDocumentPage(ctx context.Context) ([]bitbucketBufferedDocument, error) {
	if s.repoIndex >= len(s.repos) {
		return nil, nil
	}
	repo := s.repos[s.repoIndex]
	pageURL := s.pageURL
	page, err := s.connector.listPullRequestPage(ctx, repo, pageURL, s.windowStart, s.windowEnd)
	if err != nil {
		return nil, err
	}
	documents := make([]bitbucketBufferedDocument, 0, len(page.Values))
	pageOffset := 0
	for _, pr := range page.Values {
		updatedAt := parseBitbucketTime(pr.UpdatedOn)
		if beforeOrAtWindowStart(updatedAt, s.windowStart) {
			continue
		}
		if afterWindowEnd(updatedAt, s.windowEnd) {
			continue
		}
		doc := pr.toSourceDocument(s.connector.workspace, repo)
		pageOffset++
		documents = append(documents, bitbucketBufferedDocument{
			document:   doc,
			checkpoint: bitbucketSyncCheckpoint(repo, pageURL, pageOffset, doc),
			offset:     pageOffset,
			sourceID:   doc.SourceID,
		})
	}
	documents, err = s.filterResumedDocuments(repo, pageURL, documents)
	if err != nil {
		return nil, err
	}
	if page.Next == "" || len(page.Values) == 0 {
		s.advanceRepo()
	} else {
		s.pageURL = page.Next
	}
	return documents, nil
}

// advanceRepo moves a Bitbucket sync session to the next repository.
func (s *bitbucketSyncSession) advanceRepo() {
	s.repoIndex++
	s.pageURL = ""
	s.clearResume()
}

// applyResume advances a sync session to the last committed Bitbucket position.
func (s *bitbucketSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("bitbucket sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor bitbucketSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("bitbucket sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	if cursor.RepoSlug == "" {
		return fmt.Errorf("bitbucket sync cursor has no resume anchor: %w", ErrSyncResumeInvalid)
	}
	for index, repo := range s.repos {
		if repo != cursor.RepoSlug {
			continue
		}
		s.repoIndex = index
		s.pageURL = cursor.PageURL
		s.resumeRepo = repo
		s.resumePageURL = cursor.PageURL
		s.resumeOffset = cursor.PageOffset
		s.resumeSource = firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
		if s.resumeSource == "" {
			return fmt.Errorf("bitbucket sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
		}
		return nil
	}
	return fmt.Errorf("bitbucket resume repo %q was not found in the current listing: %w", cursor.RepoSlug, ErrSyncResumeInvalid)
}

// filterResumedDocuments drops documents through the committed checkpoint.
func (s *bitbucketSyncSession) filterResumedDocuments(repo, pageURL string, candidates []bitbucketBufferedDocument) ([]bitbucketBufferedDocument, error) {
	if s.resumeRepo == "" {
		return candidates, nil
	}
	if repo != s.resumeRepo || pageURL != s.resumePageURL {
		return nil, fmt.Errorf("bitbucket resume page no longer matches checkpoint page: %w", ErrSyncResumeInvalid)
	}
	if s.resumeSource != "" {
		for index, candidate := range candidates {
			if candidate.sourceID == s.resumeSource {
				s.clearResume()
				return candidates[index+1:], nil
			}
		}
		return nil, fmt.Errorf("bitbucket resume anchor %q was not found on %s: %w", s.resumeSource, pageURL, ErrSyncResumeInvalid)
	}
	return nil, fmt.Errorf("bitbucket sync cursor has no source anchor: %w", ErrSyncResumeInvalid)
}

func (s *bitbucketSyncSession) clearResume() {
	s.resumeRepo = ""
	s.resumePageURL = ""
	s.resumeOffset = 0
	s.resumeSource = ""
}

type bitbucketPruneSession struct {
	connector *BitbucketConnector
	repos     []string
	repoIndex int
	pageURL   string
	batchSize int
	buffer    []SlimDocument
}

// NextBatch returns the next Bitbucket prune snapshot batch.
func (s *bitbucketPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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
		if len(batch) == 0 {
			if s.repoIndex >= len(s.repos) && len(documents) == 0 {
				return PruneBatch{}, io.EOF
			}
			continue
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

// Close closes the Bitbucket prune session.
func (s *bitbucketPruneSession) Close() error {
	return nil
}

func (s *bitbucketPruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	if s.repoIndex >= len(s.repos) {
		return nil, nil
	}
	repo := s.repos[s.repoIndex]
	pageURL := s.pageURL
	page, err := s.connector.listSlimPullRequestPage(ctx, repo, pageURL)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(page.Values))
	for _, pr := range page.Values {
		documents = append(documents, SlimDocument{SourceID: bitbucketSourceID(s.connector.workspace, repo, pr.ID)})
	}
	if page.Next == "" || len(page.Values) == 0 {
		s.repoIndex++
		s.pageURL = ""
	} else {
		s.pageURL = page.Next
	}
	return documents, nil
}

type bitbucketRepositoryPage struct {
	Values []bitbucketRepository `json:"values"`
	Next   string                `json:"next"`
}

type bitbucketRepository struct {
	Slug string `json:"slug"`
}

type bitbucketPullRequestPage struct {
	Values []bitbucketPullRequest `json:"values"`
	Next   string                 `json:"next"`
}

type bitbucketPullRequest struct {
	ID                int                    `json:"id"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	State             string                 `json:"state"`
	Reason            string                 `json:"reason"`
	Draft             bool                   `json:"draft"`
	Author            *bitbucketUser         `json:"author"`
	Reviewers         []bitbucketUser        `json:"reviewers"`
	Participants      []bitbucketParticipant `json:"participants"`
	CommentCount      int                    `json:"comment_count"`
	TaskCount         int                    `json:"task_count"`
	CreatedOn         string                 `json:"created_on"`
	UpdatedOn         string                 `json:"updated_on"`
	Source            bitbucketBranchRef     `json:"source"`
	Destination       bitbucketBranchRef     `json:"destination"`
	Links             bitbucketPRLinks       `json:"links"`
	ClosedBy          *bitbucketUser         `json:"closed_by"`
	CloseSourceBranch bool                   `json:"close_source_branch"`
}

type bitbucketUser struct {
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
}

type bitbucketParticipant struct {
	User     bitbucketUser `json:"user"`
	Approved bool          `json:"approved"`
}

type bitbucketBranchRef struct {
	Branch struct {
		Name string `json:"name"`
	} `json:"branch"`
}

type bitbucketPRLinks struct {
	HTML struct {
		Href string `json:"href"`
	} `json:"html"`
}

type bitbucketSyncCursor struct {
	RepoSlug   string `json:"repo_slug"`
	PageURL    string `json:"page_url,omitempty"`
	PageOffset int    `json:"page_offset,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
}

type bitbucketBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
	sourceID   string
}

func bitbucketSyncCheckpoint(repo, pageURL string, offset int, doc SourceDocument) *SyncCheckpoint {
	cursor, err := json.Marshal(bitbucketSyncCursor{
		RepoSlug:   repo,
		PageURL:    pageURL,
		PageOffset: offset,
		SourceID:   doc.SourceID,
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

func (p bitbucketPullRequest) toSourceDocument(workspace, repo string) SourceDocument {
	id := p.ID
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = fmt.Sprintf("PR %d", id)
	}
	body := bitbucketPRBody(p, title)
	updatedAt := parseBitbucketTime(p.UpdatedOn)
	reviewers := bitbucketUserNames(p.Reviewers)
	approvedBy := bitbucketApprovedBy(p.Participants)
	author := bitbucketUserName(p.Author)
	closedBy := bitbucketUserName(p.ClosedBy)
	sourceBranch := p.Source.Branch.Name
	destinationBranch := p.Destination.Branch.Name
	link := p.Links.HTML.Href
	if link == "" {
		link = fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/%d", workspace, repo, id)
	}
	return SourceDocument{
		SourceID:           bitbucketSourceID(workspace, repo, id),
		SemanticIdentifier: bitbucketSemanticIdentifier(id, title),
		Extension:          ".md",
		Blob:               []byte(body),
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(body)),
		Metadata: map[string]any{
			"object_type":         "PullRequest",
			"workspace":           workspace,
			"repository":          repo,
			"pr_key":              fmt.Sprintf("%s/%s#%d", workspace, repo, id),
			"id":                  strconv.Itoa(id),
			"title":               title,
			"state":               p.State,
			"draft":               strconv.FormatBool(p.Draft),
			"link":                link,
			"author":              author,
			"reviewers":           reviewers,
			"approved_by":         approvedBy,
			"comment_count":       strconv.Itoa(p.CommentCount),
			"task_count":          strconv.Itoa(p.TaskCount),
			"created_on":          p.CreatedOn,
			"updated_on":          p.UpdatedOn,
			"source_branch":       sourceBranch,
			"destination_branch":  destinationBranch,
			"closed_by":           closedBy,
			"close_source_branch": strconv.FormatBool(p.CloseSourceBranch),
		},
		Fingerprint: stableFingerprint(map[string]any{
			"object_type":         "PullRequest",
			"workspace":           workspace,
			"repository":          repo,
			"pr_id":               id,
			"title":               title,
			"description":         p.Description,
			"state":               p.State,
			"reason":              p.Reason,
			"draft":               p.Draft,
			"updated_on":          p.UpdatedOn,
			"created_on":          p.CreatedOn,
			"source_branch":       sourceBranch,
			"destination_branch":  destinationBranch,
			"author":              author,
			"reviewers":           reviewers,
			"approved_by":         approvedBy,
			"comment_count":       p.CommentCount,
			"task_count":          p.TaskCount,
			"closed_by":           closedBy,
			"close_source_branch": p.CloseSourceBranch,
		}),
	}
}

func bitbucketSourceID(workspace, repo string, id int) string {
	return fmt.Sprintf("bitbucket:%s:%s:pr:%d", workspace, repo, id)
}

func bitbucketSemanticIdentifier(id int, title string) string {
	return fmt.Sprintf("#%d: %s", id, sanitizeBitbucketName(title))
}

func bitbucketPRBody(p bitbucketPullRequest, title string) string {
	var body strings.Builder
	fmt.Fprintf(&body, "Pull Request Information:\n- Pull Request ID: %d\n- Title: %s\n- State: %s", p.ID, title, p.State)
	if p.Draft {
		body.WriteString(" (Draft)")
	}
	body.WriteString("\n")
	if strings.EqualFold(p.State, "DECLINED") {
		reason := strings.TrimSpace(p.Reason)
		if reason == "" {
			reason = "N/A"
		}
		fmt.Fprintf(&body, "- Reason: %s\n", reason)
	}
	fmt.Fprintf(&body, "- Author: %s\n", bitbucketBodyAuthor(p.Author))
	reviewers := bitbucketUserNames(p.Reviewers)
	if len(reviewers) == 0 {
		body.WriteString("- Reviewers: N/A\n")
	} else {
		fmt.Fprintf(&body, "- Reviewers: %s\n", strings.Join(reviewers, ", "))
	}
	fmt.Fprintf(&body, "- Branch: %s -> %s\n", p.Source.Branch.Name, p.Destination.Branch.Name)
	fmt.Fprintf(&body, "- Created: %s\n", bitbucketDateOrNA(p.CreatedOn))
	fmt.Fprintf(&body, "- Updated: %s", bitbucketDateOrNA(p.UpdatedOn))
	if description := strings.TrimSpace(p.Description); description != "" {
		body.WriteString("\n\nDescription:\n")
		body.WriteString(description)
	}
	return body.String()
}

func bitbucketBodyAuthor(user *bitbucketUser) string {
	if user == nil {
		return "N/A"
	}
	return bitbucketUserName(user)
}

func bitbucketUserName(user *bitbucketUser) string {
	if user == nil {
		return ""
	}
	if user.DisplayName != "" {
		return user.DisplayName
	}
	if user.Nickname != "" {
		return user.Nickname
	}
	return "unknown"
}

func bitbucketUserNames(users []bitbucketUser) []string {
	out := make([]string, 0, len(users))
	for i := range users {
		out = append(out, bitbucketUserName(&users[i]))
	}
	sort.Strings(out)
	return out
}

func bitbucketApprovedBy(participants []bitbucketParticipant) []string {
	out := make([]string, 0, len(participants))
	for _, participant := range participants {
		if participant.Approved {
			out = append(out, bitbucketUserName(&participant.User))
		}
	}
	sort.Strings(out)
	return out
}

func bitbucketDateOrNA(value string) string {
	if value == "" {
		return "N/A"
	}
	return strings.Split(value, "T")[0]
}

func parseBitbucketTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

// sanitizeBitbucketName mirrors the Bitbucket PR filename cleanup semantics.
func sanitizeBitbucketName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "PR"
	}
	replacer := strings.NewReplacer("\\", " ", "?", " ", "#", " ", "%", " ", "*", " ", ":", " ", "|", " ", "<", " ", ">", " ", `"`, " ")
	name = replacer.Replace(name)
	name = strings.ReplaceAll(name, "/", " ")
	name = bitbucketSpaceRE.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)
	const maxNameRunes = 200
	if runes := []rune(name); len(runes) > maxNameRunes {
		name = strings.TrimSpace(string(runes[:maxNameRunes]))
	}
	if name == "" {
		name = "PR"
	}
	return name + ".md"
}

func splitBitbucketList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	values = append([]string(nil), values...)
	sort.Strings(values)
	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}
