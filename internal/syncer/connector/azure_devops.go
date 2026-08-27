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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	azureDevOpsAPIVersion    = "7.1"
	azureDevOpsHostedBaseURL = "https://dev.azure.com"
	azureDevOpsPRPageSize    = 100
	// The pull request list endpoint truncates descriptions at 400 characters;
	// only the single pull request endpoint returns the full text.
	azureDevOpsPRDescriptionLimit = 400
	azureDevOpsMaxFileBytes       = 1_000_000
	defaultAzureDevOpsBatchSize   = 50

	azureDevOpsIndexModeOrganization = "organization"
	azureDevOpsIndexModeProjects     = "projects"
	azureDevOpsIndexModeRepositories = "repositories"

	azureDevOpsContentCode         = "code"
	azureDevOpsContentPullRequests = "pull_requests"
	azureDevOpsContentBoth         = "both"

	azureDevOpsStageCode         = "code"
	azureDevOpsStagePullRequests = "pull_requests"
)

var (
	azureDevOpsRetryTries     = 6
	azureDevOpsRetryBaseDelay = 1 * time.Second
	azureDevOpsRetryBackoff   = 2.0
	azureDevOpsRetryMaxDelay  = 30 * time.Second

	// Build output and vendored dependencies carry no retrievable signal.
	azureDevOpsExcludedSegments = []string{
		"/node_modules/", "/bin/", "/obj/", "/dist/", "/build/", "/target/",
		"/vendor/", "/packages/", "/.git/", "/__pycache__/", "/.venv/",
	}

	azureDevOpsBinaryExtensions = map[string]struct{}{
		".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".bmp": {}, ".ico": {},
		".svg": {}, ".webp": {}, ".pdf": {}, ".zip": {}, ".gz": {}, ".tar": {},
		".7z": {}, ".rar": {}, ".jar": {}, ".war": {}, ".dll": {}, ".exe": {},
		".so": {}, ".dylib": {}, ".pdb": {}, ".class": {}, ".pyc": {},
		".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {}, ".otf": {},
		".mp3": {}, ".mp4": {}, ".avi": {}, ".mov": {}, ".psd": {},
		".xlsx": {}, ".docx": {},
	}

	// Version-control metadata exists in every repository and adds only noise.
	azureDevOpsSkippedFilenames = map[string]struct{}{
		".gitattributes": {}, ".gitignore": {}, ".gitkeep": {},
		".gitmodules": {}, ".dockerignore": {}, ".editorconfig": {},
	}
)

// azureDevOpsHTTPError carries the status of a failed Azure DevOps call.
type azureDevOpsHTTPError struct {
	Status int
	Body   string
}

func (e *azureDevOpsHTTPError) Error() string {
	return fmt.Sprintf("azure devops request failed with status %d: %s", e.Status, e.Body)
}

// AzureDevOpsConnector reads Azure Repos source files and pull requests.
type AzureDevOpsConnector struct {
	organization string
	indexMode    string
	projects     []string
	repositories []string
	contentTypes string
	pat          string
	batchSize    int
	baseURL      string
	httpClient   *http.Client
}

// azureDevOpsRepository is one repository selected for indexing.
type azureDevOpsRepository struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Branch  string `json:"branch"`
}

// Key identifies the repository inside a resume cursor.
func (r azureDevOpsRepository) Key() string {
	return r.Project + "/" + r.Name
}

type azureDevOpsChange struct {
	CommitID  string `json:"commitId"`
	Committer struct {
		Name string    `json:"name"`
		Date time.Time `json:"date"`
	} `json:"committer"`
	Author struct {
		Name string    `json:"name"`
		Date time.Time `json:"date"`
	} `json:"author"`
}

type azureDevOpsItem struct {
	Path                  string             `json:"path"`
	GitObjectType         string             `json:"gitObjectType"`
	IsFolder              bool               `json:"isFolder"`
	LatestProcessedChange *azureDevOpsChange `json:"latestProcessedChange"`
}

type azureDevOpsPullRequest struct {
	PullRequestID int        `json:"pullRequestId"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Status        string     `json:"status"`
	SourceRefName string     `json:"sourceRefName"`
	TargetRefName string     `json:"targetRefName"`
	CreationDate  *time.Time `json:"creationDate"`
	ClosedDate    *time.Time `json:"closedDate"`
	CreatedBy     struct {
		DisplayName string `json:"displayName"`
	} `json:"createdBy"`
	Reviewers []struct {
		DisplayName string `json:"displayName"`
	} `json:"reviewers"`
}

// NewAzureDevOpsConnector creates an Azure DevOps connector from config.
func NewAzureDevOpsConnector(config map[string]any) (*AzureDevOpsConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	organization := strings.TrimSpace(stringConfig(config["organization"]))

	connector := &AzureDevOpsConnector{
		organization: organization,
		indexMode:    firstNonEmpty(strings.TrimSpace(stringConfig(config["index_mode"])), azureDevOpsIndexModeOrganization),
		projects:     splitAzureDevOpsList(stringConfig(config["projects"])),
		repositories: splitAzureDevOpsList(stringConfig(config["repositories"])),
		contentTypes: firstNonEmpty(strings.TrimSpace(stringConfig(config["content_types"])), azureDevOpsContentBoth),
		pat:          strings.TrimSpace(stringConfig(credentials["azure_devops_pat"])),
		batchSize:    configInt(config["batch_size"], defaultAzureDevOpsBatchSize),
		baseURL:      azureDevOpsOrganizationURL(organization),
		httpClient:   &http.Client{Timeout: 60 * time.Second},
	}
	return connector, nil
}

// azureDevOpsOrganizationURL resolves the API root of a hosted organization or
// a self-hosted Azure DevOps Server collection.
func azureDevOpsOrganizationURL(organization string) string {
	if organization == "" {
		return ""
	}
	if strings.HasPrefix(organization, "http://") {
		// Rejected in checkSettings; never build a client that would send the
		// personal access token in cleartext.
		return ""
	}
	if strings.HasPrefix(organization, "https://") {
		return strings.TrimRight(organization, "/")
	}
	return azureDevOpsHostedBaseURL + "/" + url.PathEscape(organization)
}

func splitAzureDevOpsList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func (c *AzureDevOpsConnector) indexesCode() bool {
	return c.contentTypes == azureDevOpsContentCode || c.contentTypes == azureDevOpsContentBoth
}

func (c *AzureDevOpsConnector) indexesPullRequests() bool {
	return c.contentTypes == azureDevOpsContentPullRequests || c.contentTypes == azureDevOpsContentBoth
}

func (c *AzureDevOpsConnector) checkSettings() error {
	if c == nil {
		return fmt.Errorf("azure devops connector is nil")
	}
	if c.organization == "" {
		return fmt.Errorf("Invalid connector settings: 'organization' must be provided")
	}
	if c.pat == "" {
		return fmt.Errorf("Missing azure_devops_pat in credentials")
	}
	if strings.HasPrefix(c.organization, "http://") {
		return fmt.Errorf("Invalid connector settings: Azure DevOps collection URLs must use HTTPS, the personal access token is sent in the Authorization header")
	}
	switch c.indexMode {
	case azureDevOpsIndexModeOrganization, azureDevOpsIndexModeProjects, azureDevOpsIndexModeRepositories:
	default:
		return fmt.Errorf("Invalid connector settings: unsupported index mode %q", c.indexMode)
	}
	switch c.contentTypes {
	case azureDevOpsContentCode, azureDevOpsContentPullRequests, azureDevOpsContentBoth:
	default:
		return fmt.Errorf("Invalid connector settings: unsupported content types %q", c.contentTypes)
	}
	if c.indexMode == azureDevOpsIndexModeProjects && len(c.projects) == 0 {
		return fmt.Errorf("Invalid connector settings: at least one project is required when indexing by project")
	}
	if c.indexMode == azureDevOpsIndexModeRepositories && len(c.repositories) == 0 {
		return fmt.Errorf("Invalid connector settings: at least one repository is required when indexing by repository")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	return nil
}

// Validate validates Azure DevOps connector settings and credentials.
func (c *AzureDevOpsConnector) Validate(ctx context.Context) error {
	if err := c.checkSettings(); err != nil {
		return err
	}
	repos, err := c.listRepositories(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("found no repositories for Azure DevOps organization %s", c.organization)
	}
	return nil
}

// ValidateConnectorSetting validates Azure DevOps settings from an unsaved config.
func (c *AzureDevOpsConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()

	if err := c.checkSettings(); err != nil {
		return err
	}

	var payload map[string]any
	err := c.getJSON(ctx, c.apiURL("/_apis/projects", url.Values{"$top": {"1"}}), &payload)
	if err == nil {
		return nil
	}

	httpErr, ok := err.(*azureDevOpsHTTPError)
	if !ok {
		return err
	}
	switch httpErr.Status {
	case http.StatusNonAuthoritativeInfo, http.StatusUnauthorized:
		return fmt.Errorf("Invalid or expired Azure DevOps personal access token.")
	case http.StatusForbidden:
		return fmt.Errorf("Personal access token lacks the required 'Code (Read)' scope (HTTP 403).")
	case http.StatusNotFound:
		return fmt.Errorf("Azure DevOps organization not found: %s", c.organization)
	}
	return err
}

// OpenSync opens one Azure DevOps sync session.
func (c *AzureDevOpsConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	repos, err := c.listRepositories(ctx)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("found no repositories for Azure DevOps organization %s", c.organization)
	}

	session := &azureDevOpsSyncSession{
		connector: c,
		repos:     repos,
		batchSize: c.batchSize,
		request:   request,
		stage:     c.initialStage(),
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Azure DevOps prune snapshot session.
func (c *AzureDevOpsConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	repos, err := c.listRepositories(ctx)
	if err != nil {
		return nil, err
	}
	return &azureDevOpsPruneSession{connector: c, repos: repos, batchSize: c.batchSize, stage: c.initialStage()}, nil
}

func (c *AzureDevOpsConnector) initialStage() string {
	if c.indexesCode() {
		return azureDevOpsStageCode
	}
	return azureDevOpsStagePullRequests
}

// apiURL builds an absolute API URL with the api-version query parameter set.
func (c *AzureDevOpsConnector) apiURL(apiPath string, query url.Values) string {
	if query == nil {
		query = url.Values{}
	}
	query.Set("api-version", azureDevOpsAPIVersion)
	return c.baseURL + apiPath + "?" + query.Encode()
}

// repoAPIURL builds the git API root of one repository.
func (c *AzureDevOpsConnector) repoAPIURL(repo azureDevOpsRepository) string {
	return "/" + url.PathEscape(repo.Project) + "/_apis/git/repositories/" + url.PathEscape(repo.Name)
}

// get performs an authenticated GET, retrying throttling and server errors.
//
// Azure DevOps answers an invalid or unauthorized personal access token with
// HTTP 203 and an HTML sign-in page rather than 401, so a naive status check
// treats the sign-in page as a successful response and fails later while
// decoding JSON. That case is detected here and surfaced as an auth error.
// A maxBytes of zero reads the whole response; a positive value stops one byte
// past the limit so the caller can tell an oversized payload apart without ever
// allocating it in full.
func (c *AzureDevOpsConnector) get(ctx context.Context, apiURL string, expectJSON bool, maxBytes int64) ([]byte, error) {
	delay := azureDevOpsRetryBaseDelay
	var lastErr error

	for attempt := 0; attempt < azureDevOpsRetryTries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+c.pat)))

		response, err := c.httpClient.Do(request)
		if err != nil {
			lastErr = err
			if sleepErr := sleepForAzureDevOps(ctx, delay); sleepErr != nil {
				return nil, sleepErr
			}
			delay = nextAzureDevOpsDelay(delay)
			continue
		}

		var reader io.Reader = response.Body
		if maxBytes > 0 {
			reader = io.LimitReader(response.Body, maxBytes+1)
		}
		body, readErr := io.ReadAll(reader)
		_ = response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		contentType := response.Header.Get("Content-Type")
		// A repository can legitimately contain .html files, so the sign-in page
		// heuristic only applies to endpoints that return JSON.
		if response.StatusCode == http.StatusNonAuthoritativeInfo || (expectJSON && strings.Contains(contentType, "text/html")) {
			return nil, &azureDevOpsHTTPError{Status: http.StatusNonAuthoritativeInfo, Body: "sign-in page returned; the personal access token is invalid or unauthorized"}
		}

		switch {
		case response.StatusCode == http.StatusTooManyRequests:
			lastErr = &azureDevOpsHTTPError{Status: response.StatusCode, Body: "rate limit exceeded"}
			wait := azureDevOpsRetryAfter(response, delay)
			if sleepErr := sleepForAzureDevOps(ctx, wait); sleepErr != nil {
				return nil, sleepErr
			}
			delay = nextAzureDevOpsDelay(delay)
		case response.StatusCode >= 500:
			lastErr = &azureDevOpsHTTPError{Status: response.StatusCode, Body: truncateAzureDevOpsBody(body)}
			if sleepErr := sleepForAzureDevOps(ctx, delay); sleepErr != nil {
				return nil, sleepErr
			}
			delay = nextAzureDevOpsDelay(delay)
		case response.StatusCode >= 400:
			return nil, &azureDevOpsHTTPError{Status: response.StatusCode, Body: truncateAzureDevOpsBody(body)}
		default:
			return body, nil
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("azure devops request failed after %d attempts", azureDevOpsRetryTries)
	}
	return nil, lastErr
}

func (c *AzureDevOpsConnector) getJSON(ctx context.Context, apiURL string, out any) error {
	body, err := c.get(ctx, apiURL, true, 0)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// azureDevOpsRetryAfter honours a Retry-After header, clamped to the backoff
// ceiling. Azure DevOps can ask for a long pause, and waiting it out verbatim
// would park a sync worker for hours.
func azureDevOpsRetryAfter(response *http.Response, fallback time.Duration) time.Duration {
	if value := response.Header.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			// Clamp before the multiplication: a large enough value overflows
			// int64, and the negative result would slip past the cap and make
			// the timer fire immediately.
			if seconds >= int(azureDevOpsRetryMaxDelay/time.Second) {
				return azureDevOpsRetryMaxDelay
			}
			return time.Duration(seconds) * time.Second
		}
	}
	return fallback
}

func nextAzureDevOpsDelay(delay time.Duration) time.Duration {
	next := time.Duration(float64(delay) * azureDevOpsRetryBackoff)
	if next > azureDevOpsRetryMaxDelay {
		return azureDevOpsRetryMaxDelay
	}
	return next
}

func sleepForAzureDevOps(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncateAzureDevOpsBody(body []byte) string {
	const limit = 300
	if len(body) > limit {
		return string(body[:limit])
	}
	return string(body)
}

// listRepositories resolves the repositories to index, deterministically ordered.
func (c *AzureDevOpsConnector) listRepositories(ctx context.Context) ([]azureDevOpsRepository, error) {
	scopes := c.repositoryScopes()
	seen := map[string]struct{}{}
	repos := make([]azureDevOpsRepository, 0, 16)

	for _, scope := range scopes {
		apiPath := "/_apis/git/repositories"
		if scope != "" {
			apiPath = "/" + url.PathEscape(scope) + "/_apis/git/repositories"
		}

		var payload struct {
			Value []struct {
				Name          string `json:"name"`
				DefaultBranch string `json:"defaultBranch"`
				IsDisabled    bool   `json:"isDisabled"`
				Project       struct {
					Name string `json:"name"`
				} `json:"project"`
			} `json:"value"`
		}
		if err := c.getJSON(ctx, c.apiURL(apiPath, nil), &payload); err != nil {
			return nil, err
		}

		for _, item := range payload.Value {
			if item.IsDisabled || item.Name == "" {
				continue
			}
			project := item.Project.Name
			if project == "" {
				project = scope
			}
			if project == "" || !c.matchesRepositoryFilter(project, item.Name) {
				continue
			}
			repo := azureDevOpsRepository{
				Project: project,
				Name:    item.Name,
				Branch:  strings.TrimPrefix(firstNonEmpty(item.DefaultBranch, "refs/heads/main"), "refs/heads/"),
			}
			if _, exists := seen[repo.Key()]; exists {
				continue
			}
			seen[repo.Key()] = struct{}{}
			repos = append(repos, repo)
		}
	}

	sort.Slice(repos, func(i, j int) bool { return repos[i].Key() < repos[j].Key() })
	return repos, nil
}

// repositoryScopes returns the projects to query, or a single empty scope to
// use the organization-wide endpoint that returns every repository at once.
func (c *AzureDevOpsConnector) repositoryScopes() []string {
	switch c.indexMode {
	case azureDevOpsIndexModeProjects:
		if len(c.projects) > 0 {
			return c.projects
		}
	case azureDevOpsIndexModeRepositories:
		qualified := map[string]struct{}{}
		for _, entry := range c.repositories {
			if project, _, found := strings.Cut(entry, "/"); found && project != "" {
				qualified[project] = struct{}{}
			}
		}
		if len(qualified) > 0 {
			scopes := make([]string, 0, len(qualified))
			for project := range qualified {
				scopes = append(scopes, project)
			}
			sort.Strings(scopes)
			return scopes
		}
	}
	return []string{""}
}

// matchesRepositoryFilter accepts "project/repo" and bare repository names.
//
// Azure DevOps repository names are unique per project rather than per
// organization, so the qualified form is the unambiguous one.
func (c *AzureDevOpsConnector) matchesRepositoryFilter(project, name string) bool {
	if c.indexMode != azureDevOpsIndexModeRepositories || len(c.repositories) == 0 {
		return true
	}
	qualified := project + "/" + name
	for _, entry := range c.repositories {
		if entry == name || entry == qualified {
			return true
		}
	}
	return false
}

// listItems lists every indexable file of a repository at its default branch.
//
// latestProcessedChange returns the last commit of each item in the same
// response, which supplies both the update timestamp and the fingerprint
// without one extra request per file.
func (c *AzureDevOpsConnector) listItems(ctx context.Context, repo azureDevOpsRepository) ([]azureDevOpsItem, error) {
	query := url.Values{
		"recursionLevel":                {"Full"},
		"includeContentMetadata":        {"true"},
		"latestProcessedChange":         {"true"},
		"versionDescriptor.versionType": {"branch"},
		"versionDescriptor.version":     {repo.Branch},
	}

	var payload struct {
		Value []azureDevOpsItem `json:"value"`
	}
	if err := c.getJSON(ctx, c.apiURL(c.repoAPIURL(repo)+"/items", query), &payload); err != nil {
		return nil, err
	}

	items := make([]azureDevOpsItem, 0, len(payload.Value))
	for _, item := range payload.Value {
		if item.GitObjectType != "blob" || item.IsFolder || shouldSkipAzureDevOpsPath(item.Path) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// fetchFile downloads one file, returning nil when it is too large to index.
func (c *AzureDevOpsConnector) fetchFile(ctx context.Context, repo azureDevOpsRepository, filePath string) ([]byte, error) {
	query := url.Values{
		"path":                          {filePath},
		"includeContent":                {"true"},
		"$format":                       {"text"},
		"versionDescriptor.versionType": {"branch"},
		"versionDescriptor.version":     {repo.Branch},
	}
	body, err := c.get(ctx, c.apiURL(c.repoAPIURL(repo)+"/items", query), false, azureDevOpsMaxFileBytes)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > azureDevOpsMaxFileBytes {
		return nil, nil
	}
	return body, nil
}

// listPullRequests returns one page of pull requests in every state.
func (c *AzureDevOpsConnector) listPullRequests(ctx context.Context, repo azureDevOpsRepository, skip int) ([]azureDevOpsPullRequest, error) {
	query := url.Values{
		"searchCriteria.status": {"all"},
		"$top":                  {strconv.Itoa(azureDevOpsPRPageSize)},
		"$skip":                 {strconv.Itoa(skip)},
	}
	var payload struct {
		Value []azureDevOpsPullRequest `json:"value"`
	}
	if err := c.getJSON(ctx, c.apiURL(c.repoAPIURL(repo)+"/pullrequests", query), &payload); err != nil {
		return nil, err
	}
	return payload.Value, nil
}

// fetchPullRequest fetches one pull request with its untruncated description.
func (c *AzureDevOpsConnector) fetchPullRequest(ctx context.Context, repo azureDevOpsRepository, pullRequestID int) (azureDevOpsPullRequest, error) {
	var pullRequest azureDevOpsPullRequest
	apiPath := fmt.Sprintf("%s/pullrequests/%d", c.repoAPIURL(repo), pullRequestID)
	if err := c.getJSON(ctx, c.apiURL(apiPath, nil), &pullRequest); err != nil {
		return azureDevOpsPullRequest{}, err
	}
	return pullRequest, nil
}

// azureDevOpsPullRequestMayBeTruncated reports whether a listed pull request
// needs a detail fetch.
//
// Descriptions shorter than the limit came back whole, so the extra request is
// only paid for the few pull requests that could have been cut off.
func azureDevOpsPullRequestMayBeTruncated(pullRequest azureDevOpsPullRequest) bool {
	return utf8.RuneCountInString(pullRequest.Description) >= azureDevOpsPRDescriptionLimit
}

// shouldSkipAzureDevOpsPath drops build output, vendored code, version-control
// metadata and binary assets.
func shouldSkipAzureDevOpsPath(itemPath string) bool {
	lowered := strings.ToLower(itemPath)
	for _, segment := range azureDevOpsExcludedSegments {
		if strings.Contains(lowered, segment) {
			return true
		}
	}
	if _, skipped := azureDevOpsSkippedFilenames[path.Base(lowered)]; skipped {
		return true
	}
	_, binary := azureDevOpsBinaryExtensions[path.Ext(lowered)]
	return binary
}

// azureDevOpsDocumentExtension resolves the extension the file is parsed with.
//
// Files such as Dockerfile, Makefile and LICENSE carry no extension, and an
// empty extension would leave the downstream parser without a handler.
func azureDevOpsDocumentExtension(itemPath string) string {
	if extension := strings.ToLower(path.Ext(itemPath)); extension != "" {
		return extension
	}
	return ".txt"
}

func azureDevOpsCodeSourceID(organization string, repo azureDevOpsRepository, filePath string) string {
	return fmt.Sprintf("azure_devops:%s:%s:%s:file:%s", organization, repo.Project, repo.Name, strings.TrimPrefix(filePath, "/"))
}

func azureDevOpsPullRequestSourceID(organization string, repo azureDevOpsRepository, pullRequestID int) string {
	return fmt.Sprintf("azure_devops:%s:%s:%s:pr:%d", organization, repo.Project, repo.Name, pullRequestID)
}

// includeAzureDevOpsItem decides whether a file needs to be re-synced.
//
// The commit id of the last change acts as the fingerprint, so unchanged files
// are skipped before their content is downloaded.
func includeAzureDevOpsItem(request SyncRequest, sourceID string, item azureDevOpsItem) bool {
	if request.FromBeginning {
		return true
	}
	fingerprint := azureDevOpsItemFingerprint(item)
	if len(request.Fingerprints) > 0 {
		stored, ok := request.Fingerprints[sourceID]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	updatedAt := azureDevOpsItemUpdatedAt(item)
	if updatedAt.IsZero() {
		return true
	}
	return !beforeOrAtWindowStart(updatedAt, request.WindowStart) && !afterWindowEnd(updatedAt, request.WindowEnd)
}

func azureDevOpsItemFingerprint(item azureDevOpsItem) string {
	if item.LatestProcessedChange == nil {
		return ""
	}
	return item.LatestProcessedChange.CommitID
}

func azureDevOpsItemUpdatedAt(item azureDevOpsItem) time.Time {
	if item.LatestProcessedChange == nil {
		return time.Time{}
	}
	if !item.LatestProcessedChange.Committer.Date.IsZero() {
		return item.LatestProcessedChange.Committer.Date
	}
	return item.LatestProcessedChange.Author.Date
}

func azureDevOpsPullRequestUpdatedAt(pullRequest azureDevOpsPullRequest) time.Time {
	if pullRequest.ClosedDate != nil && !pullRequest.ClosedDate.IsZero() {
		return *pullRequest.ClosedDate
	}
	if pullRequest.CreationDate != nil {
		return *pullRequest.CreationDate
	}
	return time.Time{}
}

// includeAzureDevOpsPullRequest applies the sync window client-side; the Azure
// DevOps pull request endpoint has no dependable "updated since" filter.
func includeAzureDevOpsPullRequest(request SyncRequest, sourceID string, pullRequest azureDevOpsPullRequest) bool {
	if request.FromBeginning {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := azureDevOpsPullRequestFingerprint(pullRequest)
		stored, ok := request.Fingerprints[sourceID]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}

	// Azure DevOps exposes no dependable "last updated" timestamp for pull
	// requests. closedDate is reliable, so completed and abandoned ones are
	// filtered on it; an active pull request can change at any time, and
	// filtering it on creationDate would leave the indexed document stale.
	status := strings.ToLower(pullRequest.Status)
	if status != "completed" && status != "abandoned" {
		return true
	}
	updatedAt := azureDevOpsPullRequestUpdatedAt(pullRequest)
	if updatedAt.IsZero() {
		return true
	}
	return !beforeOrAtWindowStart(updatedAt, request.WindowStart) && !afterWindowEnd(updatedAt, request.WindowEnd)
}

func azureDevOpsPullRequestFingerprint(pullRequest azureDevOpsPullRequest) string {
	updatedAt := azureDevOpsPullRequestUpdatedAt(pullRequest)
	if updatedAt.IsZero() {
		return ""
	}
	return pullRequest.Status + ":" + updatedAt.UTC().Format(time.RFC3339Nano)
}

// buildAzureDevOpsCodeDocument maps a repository file to a source document.
func (c *AzureDevOpsConnector) buildAzureDevOpsCodeDocument(repo azureDevOpsRepository, item azureDevOpsItem, content []byte) SourceDocument {
	relativePath := strings.TrimPrefix(item.Path, "/")
	updatedAt := azureDevOpsItemUpdatedAt(item)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	webURL := fmt.Sprintf("%s/%s/_git/%s?path=/%s&version=GB%s",
		c.baseURL, url.PathEscape(repo.Project), url.PathEscape(repo.Name), relativePath, repo.Branch)

	commitID := azureDevOpsItemFingerprint(item)
	committer := ""
	if item.LatestProcessedChange != nil {
		committer = firstNonEmpty(item.LatestProcessedChange.Committer.Name, item.LatestProcessedChange.Author.Name)
	}

	return SourceDocument{
		SourceID:           azureDevOpsCodeSourceID(c.organization, repo, relativePath),
		SemanticIdentifier: path.Base(relativePath),
		Extension:          azureDevOpsDocumentExtension(relativePath),
		Blob:               content,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(content)),
		Fingerprint:        commitID,
		Metadata: map[string]any{
			"type":       "CodeFile",
			"path":       relativePath,
			"ref":        repo.Branch,
			"project":    repo.Project,
			"repository": repo.Name,
			"commit_id":  commitID,
			"committer":  committer,
			"web_url":    webURL,
		},
	}
}

// buildAzureDevOpsPullRequestDocument maps a pull request to a source document.
func (c *AzureDevOpsConnector) buildAzureDevOpsPullRequestDocument(repo azureDevOpsRepository, pullRequest azureDevOpsPullRequest) SourceDocument {
	sourceBranch := strings.TrimPrefix(pullRequest.SourceRefName, "refs/heads/")
	targetBranch := strings.TrimPrefix(pullRequest.TargetRefName, "refs/heads/")

	reviewers := make([]string, 0, len(pullRequest.Reviewers))
	for _, reviewer := range pullRequest.Reviewers {
		if reviewer.DisplayName != "" {
			reviewers = append(reviewers, reviewer.DisplayName)
		}
	}

	createdOn := "N/A"
	if pullRequest.CreationDate != nil {
		createdOn = pullRequest.CreationDate.UTC().Format("2006-01-02")
	}
	closedOn := "N/A"
	if pullRequest.ClosedDate != nil {
		closedOn = pullRequest.ClosedDate.UTC().Format("2006-01-02")
	}

	var builder strings.Builder
	builder.WriteString("Pull Request Information:\n")
	fmt.Fprintf(&builder, "- Pull Request ID: %d\n", pullRequest.PullRequestID)
	fmt.Fprintf(&builder, "- Title: %s\n", pullRequest.Title)
	fmt.Fprintf(&builder, "- Status: %s\n", pullRequest.Status)
	fmt.Fprintf(&builder, "- Repository: %s/%s\n", repo.Project, repo.Name)
	fmt.Fprintf(&builder, "- Source Branch: %s\n", sourceBranch)
	fmt.Fprintf(&builder, "- Target Branch: %s\n", targetBranch)
	fmt.Fprintf(&builder, "- Created By: %s\n", pullRequest.CreatedBy.DisplayName)
	fmt.Fprintf(&builder, "- Reviewers: %s\n", firstNonEmpty(strings.Join(reviewers, ", "), "N/A"))
	fmt.Fprintf(&builder, "- Created On: %s\n", createdOn)
	fmt.Fprintf(&builder, "- Closed On: %s\n", closedOn)
	fmt.Fprintf(&builder, "\nDescription:\n%s\n", pullRequest.Description)

	blob := []byte(builder.String())
	updatedAt := azureDevOpsPullRequestUpdatedAt(pullRequest)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	return SourceDocument{
		SourceID:           azureDevOpsPullRequestSourceID(c.organization, repo, pullRequest.PullRequestID),
		SemanticIdentifier: fmt.Sprintf("PR #%d: %s", pullRequest.PullRequestID, pullRequest.Title),
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Fingerprint:        azureDevOpsPullRequestFingerprint(pullRequest),
		Metadata: map[string]any{
			"type":            "PullRequest",
			"pull_request_id": strconv.Itoa(pullRequest.PullRequestID),
			"status":          pullRequest.Status,
			"project":         repo.Project,
			"repository":      repo.Name,
			"source_branch":   sourceBranch,
			"target_branch":   targetBranch,
			"web_url":         fmt.Sprintf("%s/%s/_git/%s/pullrequest/%d", c.baseURL, url.PathEscape(repo.Project), url.PathEscape(repo.Name), pullRequest.PullRequestID),
		},
	}
}

// azureDevOpsSyncCursor is the resume position persisted between batches.
// azureDevOpsSyncCursor is the resume position persisted between batches.
//
// SourceID is the anchor: offsets alone are positions in a remote listing that
// shifts whenever a file or pull request is added or removed, so resuming on an
// offset can silently skip or repeat an item. The offset is kept only as a fast
// lookup hint, and correctness is decided by the anchor.
type azureDevOpsSyncCursor struct {
	RepoKey    string `json:"repo_key"`
	Stage      string `json:"stage"`
	FileOffset int    `json:"file_offset,omitempty"`
	PRSkip     int    `json:"pr_skip,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
}

type azureDevOpsSyncSession struct {
	connector    *AzureDevOpsConnector
	repos        []azureDevOpsRepository
	repoIndex    int
	stage        string
	fileOffset   int
	prSkip       int
	items        []azureDevOpsItem
	itemsRepo    string
	batchSize    int
	request      SyncRequest
	lastSourceID string

	// Resume anchor, consumed by the first batch produced after a resume.
	resumeRepoKey  string
	resumeStage    string
	resumeSourceID string
}

// NextBatch returns the next Azure DevOps document batch.
func (s *azureDevOpsSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)

	for len(documents) < s.batchSize {
		if s.repoIndex >= len(s.repos) {
			if len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			break
		}

		repo := s.repos[s.repoIndex]
		var (
			produced []SourceDocument
			err      error
		)
		if s.stage == azureDevOpsStageCode {
			produced, err = s.nextCodeDocuments(ctx, repo, s.batchSize-len(documents))
		} else {
			produced, err = s.nextPullRequestDocuments(ctx, repo)
		}
		if err != nil {
			return SyncBatch{}, err
		}
		documents = append(documents, produced...)
	}

	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	s.lastSourceID = documents[len(documents)-1].SourceID
	return SyncBatch{Documents: documents, Checkpoint: s.checkpoint()}, nil
}

// Close closes the Azure DevOps sync session.
func (s *azureDevOpsSyncSession) Close() error {
	return nil
}

func (s *azureDevOpsSyncSession) checkpoint() *SyncCheckpoint {
	if s.repoIndex >= len(s.repos) {
		return nil
	}
	cursor := azureDevOpsSyncCursor{
		RepoKey:    s.repos[s.repoIndex].Key(),
		Stage:      s.stage,
		FileOffset: s.fileOffset,
		PRSkip:     s.prSkip,
		SourceID:   s.lastSourceID,
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	return &SyncCheckpoint{Cursor: string(encoded), SourceID: s.lastSourceID}
}

// nextCodeDocuments emits up to limit files, advancing the stage when the
// repository file list is exhausted.
func (s *azureDevOpsSyncSession) nextCodeDocuments(ctx context.Context, repo azureDevOpsRepository, limit int) ([]SourceDocument, error) {
	if s.itemsRepo != repo.Key() {
		items, err := s.connector.listItems(ctx, repo)
		if err != nil {
			return nil, err
		}
		s.items = items
		s.itemsRepo = repo.Key()
		if err := s.applyFileAnchor(repo); err != nil {
			return nil, err
		}
	}

	documents := make([]SourceDocument, 0, limit)
	for s.fileOffset < len(s.items) && len(documents) < limit {
		item := s.items[s.fileOffset]
		s.fileOffset++

		sourceID := azureDevOpsCodeSourceID(s.connector.organization, repo, strings.TrimPrefix(item.Path, "/"))
		if !includeAzureDevOpsItem(s.request, sourceID, item) {
			continue
		}
		content, err := s.connector.fetchFile(ctx, repo, item.Path)
		if err != nil {
			return nil, err
		}
		if content == nil {
			continue
		}
		documents = append(documents, s.connector.buildAzureDevOpsCodeDocument(repo, item, content))
	}

	if s.fileOffset >= len(s.items) {
		s.advanceStage()
	}
	return documents, nil
}

// applyFileAnchor positions the file walk after the last committed document.
//
// The listing is re-fetched on every resume and can have shifted since the
// checkpoint was written, so the stored offset is treated as a hint: it is
// checked first, then the whole listing is searched for the anchor. A missing
// anchor means the saved progress no longer maps to the source listing.
func (s *azureDevOpsSyncSession) applyFileAnchor(repo azureDevOpsRepository) error {
	if s.resumeSourceID == "" || s.resumeRepoKey != repo.Key() || s.resumeStage != azureDevOpsStageCode {
		return nil
	}
	defer s.clearResume()

	if s.fileOffset > 0 && s.fileOffset <= len(s.items) {
		previous := s.items[s.fileOffset-1]
		if azureDevOpsCodeSourceID(s.connector.organization, repo, strings.TrimPrefix(previous.Path, "/")) == s.resumeSourceID {
			return nil
		}
	}

	for index, item := range s.items {
		if azureDevOpsCodeSourceID(s.connector.organization, repo, strings.TrimPrefix(item.Path, "/")) == s.resumeSourceID {
			s.fileOffset = index + 1
			return nil
		}
	}
	return fmt.Errorf("azure devops file resume anchor %q was not found in repo %s: %w", s.resumeSourceID, repo.Key(), ErrSyncResumeInvalid)
}

// filterResumedPullRequests drops the pull requests already committed.
//
// $skip indexes into a listing that shifts as pull requests are opened, so the
// anchor decides where the page really resumes; the skip value only positions
// the request.
func (s *azureDevOpsSyncSession) filterResumedPullRequests(repo azureDevOpsRepository, pullRequests []azureDevOpsPullRequest) ([]azureDevOpsPullRequest, error) {
	if s.resumeSourceID == "" || s.resumeRepoKey != repo.Key() || s.resumeStage != azureDevOpsStagePullRequests {
		return pullRequests, nil
	}
	defer s.clearResume()

	for index, pullRequest := range pullRequests {
		if azureDevOpsPullRequestSourceID(s.connector.organization, repo, pullRequest.PullRequestID) == s.resumeSourceID {
			return pullRequests[index+1:], nil
		}
	}
	return nil, fmt.Errorf("azure devops pull request resume anchor %q was not found in repo %s: %w", s.resumeSourceID, repo.Key(), ErrSyncResumeInvalid)
}

func (s *azureDevOpsSyncSession) clearResume() {
	s.resumeRepoKey = ""
	s.resumeStage = ""
	s.resumeSourceID = ""
}

// nextPullRequestDocuments emits one pull request page, advancing to the next
// repository once the last page is consumed.
func (s *azureDevOpsSyncSession) nextPullRequestDocuments(ctx context.Context, repo azureDevOpsRepository) ([]SourceDocument, error) {
	pullRequests, err := s.connector.listPullRequests(ctx, repo, s.prSkip)
	if err != nil {
		return nil, err
	}
	pageSize := len(pullRequests)
	pullRequests, err = s.filterResumedPullRequests(repo, pullRequests)
	if err != nil {
		return nil, err
	}

	documents := make([]SourceDocument, 0, len(pullRequests))
	for _, pullRequest := range pullRequests {
		sourceID := azureDevOpsPullRequestSourceID(s.connector.organization, repo, pullRequest.PullRequestID)
		if !includeAzureDevOpsPullRequest(s.request, sourceID, pullRequest) {
			continue
		}
		if azureDevOpsPullRequestMayBeTruncated(pullRequest) {
			detailed, err := s.connector.fetchPullRequest(ctx, repo, pullRequest.PullRequestID)
			if err != nil {
				return nil, err
			}
			pullRequest = detailed
		}
		documents = append(documents, s.connector.buildAzureDevOpsPullRequestDocument(repo, pullRequest))
	}

	if pageSize < azureDevOpsPRPageSize {
		s.advanceRepo()
	} else {
		s.prSkip += azureDevOpsPRPageSize
	}
	return documents, nil
}

func (s *azureDevOpsSyncSession) advanceStage() {
	s.fileOffset = 0
	s.items = nil
	s.itemsRepo = ""
	if s.connector.indexesPullRequests() {
		s.stage = azureDevOpsStagePullRequests
		return
	}
	s.advanceRepo()
}

func (s *azureDevOpsSyncSession) advanceRepo() {
	s.repoIndex++
	s.stage = s.connector.initialStage()
	s.fileOffset = 0
	s.prSkip = 0
	s.items = nil
	s.itemsRepo = ""
}

// applyResume advances the session to the last committed position.
func (s *azureDevOpsSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("azure devops sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor azureDevOpsSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("azure devops sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	if cursor.RepoKey == "" {
		return fmt.Errorf("azure devops sync cursor has no repo anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.Stage != azureDevOpsStageCode && cursor.Stage != azureDevOpsStagePullRequests {
		return fmt.Errorf("azure devops sync cursor has no valid stage: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" {
		return fmt.Errorf("azure devops sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, repo := range s.repos {
		if repo.Key() != cursor.RepoKey {
			continue
		}
		s.repoIndex = index
		if cursor.Stage != "" {
			s.stage = cursor.Stage
		}
		s.fileOffset = cursor.FileOffset
		s.prSkip = cursor.PRSkip
		s.resumeRepoKey = cursor.RepoKey
		s.resumeStage = s.stage
		s.resumeSourceID = sourceID
		return nil
	}
	return fmt.Errorf("azure devops resume repo %q was not found in the current listing: %w", cursor.RepoKey, ErrSyncResumeInvalid)
}

type azureDevOpsPruneSession struct {
	connector *AzureDevOpsConnector
	repos     []azureDevOpsRepository
	repoIndex int
	stage     string
	prSkip    int
	batchSize int
}

// NextBatch returns the next Azure DevOps slim snapshot batch.
func (s *azureDevOpsPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	for s.repoIndex < len(s.repos) {
		repo := s.repos[s.repoIndex]

		if s.stage == azureDevOpsStageCode {
			items, err := s.connector.listItems(ctx, repo)
			if err != nil {
				return PruneBatch{}, err
			}
			documents := make([]SlimDocument, 0, len(items))
			for _, item := range items {
				documents = append(documents, SlimDocument{
					SourceID: azureDevOpsCodeSourceID(s.connector.organization, repo, strings.TrimPrefix(item.Path, "/")),
				})
			}
			if s.connector.indexesPullRequests() {
				s.stage = azureDevOpsStagePullRequests
			} else {
				s.advanceRepo()
			}
			if len(documents) > 0 {
				return PruneBatch{Documents: documents}, nil
			}
			continue
		}

		pullRequests, err := s.connector.listPullRequests(ctx, repo, s.prSkip)
		if err != nil {
			return PruneBatch{}, err
		}
		documents := make([]SlimDocument, 0, len(pullRequests))
		for _, pullRequest := range pullRequests {
			documents = append(documents, SlimDocument{
				SourceID: azureDevOpsPullRequestSourceID(s.connector.organization, repo, pullRequest.PullRequestID),
			})
		}
		if len(pullRequests) < azureDevOpsPRPageSize {
			s.advanceRepo()
		} else {
			s.prSkip += azureDevOpsPRPageSize
		}
		if len(documents) > 0 {
			return PruneBatch{Documents: documents}, nil
		}
	}
	return PruneBatch{}, io.EOF
}

// Close closes the Azure DevOps prune session.
func (s *azureDevOpsPruneSession) Close() error {
	return nil
}

func (s *azureDevOpsPruneSession) advanceRepo() {
	s.repoIndex++
	s.stage = s.connector.initialStage()
	s.prSkip = 0
}
