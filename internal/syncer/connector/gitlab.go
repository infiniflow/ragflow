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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/utility"
)

const (
	defaultGitlabBatchSize = 32
	gitlabRequestTimeout   = 60 * time.Second
)

var gitlabExcludePatterns = []string{"logs", ".github", ".gitlab"}

// GitlabConnector reads GitLab merge requests, issues, and code files.
type GitlabConnector struct {
	projectOwner     string
	projectName      string
	gitlabURL        string
	token            string
	batchSize        int
	includeMRs       bool
	includeIssues    bool
	includeCodeFiles bool
	baseURL          string
	doJSON           func(ctx context.Context, apiURL string, out any) (http.Header, error)
	doRaw            func(ctx context.Context, apiURL string) ([]byte, error)
}

// NewGitlabConnector creates a GitLab connector from config.
func NewGitlabConnector(config map[string]any) (*GitlabConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	token, _ := credentials["gitlab_access_token"].(string)
	gitlabURL := strings.TrimRight(stringConfig(config["gitlab_url"]), "/")
	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}
	return &GitlabConnector{
		projectOwner:     strings.TrimSpace(stringConfig(config["project_owner"])),
		projectName:      strings.TrimSpace(stringConfig(config["project_name"])),
		gitlabURL:        gitlabURL,
		token:            strings.TrimSpace(token),
		batchSize:        configInt(config["batch_size"], defaultGitlabBatchSize),
		includeMRs:       configBoolDefault(config["include_mrs"], true),
		includeIssues:    configBoolDefault(config["include_issues"], true),
		includeCodeFiles: configBoolDefault(config["include_code_files"], true),
		baseURL:          gitlabURL + "/api/v4",
	}, nil
}

// Validate validates GitLab connector settings and credentials.
func (c *GitlabConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("gitlab connector is nil")
	}
	if c.projectOwner == "" {
		return fmt.Errorf("Invalid connector settings: 'project_owner' must be provided")
	}
	if c.projectName == "" {
		return fmt.Errorf("Invalid connector settings: 'project_name' must be provided")
	}
	if c.token == "" {
		return fmt.Errorf("Missing gitlab_access_token in credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if _, err := c.getProject(ctx); err != nil {
		return err
	}
	return nil
}

// ValidateConnectorSetting validates GitLab settings from an unsaved config.
func (c *GitlabConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if c == nil {
		return fmt.Errorf("gitlab connector is nil")
	}
	if c.projectOwner == "" {
		return fmt.Errorf("Invalid connector settings: 'project_owner' must be provided")
	}
	if c.projectName == "" {
		return fmt.Errorf("Invalid connector settings: 'project_name' must be provided")
	}
	if c.token == "" {
		return fmt.Errorf("Missing gitlab_access_token in credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	var user map[string]any
	if _, err := c.getJSON(ctx, c.apiURL("/user", nil), &user); err != nil {
		return err
	}
	return nil
}

// OpenSync opens one GitLab sync session.
func (c *GitlabConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	project, err := c.getProject(ctx)
	if err != nil {
		return nil, err
	}
	session := &gitlabSyncSession{
		connector:   c,
		project:     project,
		batchSize:   c.batchSize,
		stage:       gitlabStageCodeFiles,
		treeQueue:   []string{""},
		treePage:    1,
		page:        1,
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
	}
	if !c.includeCodeFiles {
		session.stage = gitlabStageMRs
		session.treeQueue = nil
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete GitLab prune snapshot session.
func (c *GitlabConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	project, err := c.getProject(ctx)
	if err != nil {
		return nil, err
	}
	session := &gitlabPruneSession{
		connector: c,
		project:   project,
		batchSize: c.batchSize,
		stage:     gitlabStageCodeFiles,
		treeQueue: []string{""},
		treePage:  1,
		page:      1,
	}
	if !c.includeCodeFiles {
		session.stage = gitlabStageMRs
		session.treeQueue = nil
	}
	return session, nil
}

// getProject fetches the GitLab project info.
func (c *GitlabConnector) getProject(ctx context.Context) (gitlabProject, error) {
	encoded := gitlabProjectPath(c.projectOwner, c.projectName)
	var project gitlabProject
	if _, err := c.getJSON(ctx, c.apiURL("/projects/"+encoded, nil), &project); err != nil {
		return gitlabProject{}, err
	}
	return project, nil
}

// listMergeRequestPage returns one page of GitLab merge requests.
func (c *GitlabConnector) listMergeRequestPage(ctx context.Context, projectID int, page, pageSize int, windowStart *time.Time, windowEnd time.Time) ([]gitlabBufferedDocument, bool, error) {
	query := gitlabListQuery(page, pageSize)
	query.Set("order_by", "updated_at")
	query.Set("sort", "desc")
	var batch []gitlabMergeRequest
	headers, err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/projects/%d/merge_requests", projectID), query), &batch)
	if err != nil {
		return nil, false, err
	}
	documents := make([]gitlabBufferedDocument, 0, len(batch))
	doneByWindow := false
	pageOffset := 0
	for _, mr := range batch {
		if beforeOrAtWindowStart(mr.UpdatedAt, windowStart) {
			doneByWindow = true
			break
		}
		if afterWindowEnd(mr.UpdatedAt, windowEnd) {
			continue
		}
		doc := mr.toSourceDocument()
		pageOffset++
		documents = append(documents, gitlabBufferedDocument{
			document:   doc,
			checkpoint: gitlabSyncCheckpoint(gitlabSyncCursor{Stage: gitlabStageMRs, Page: page, Offset: pageOffset, SourceID: doc.SourceID}, doc),
			offset:     pageOffset,
			sourceID:   doc.SourceID,
		})
	}
	done := doneByWindow || !gitlabHasNextPage(headers) || len(batch) == 0
	return documents, done, nil
}

// listIssuePage returns one page of GitLab issues.
func (c *GitlabConnector) listIssuePage(ctx context.Context, projectID int, page, pageSize int, windowStart *time.Time, windowEnd time.Time) ([]gitlabBufferedDocument, bool, error) {
	query := gitlabListQuery(page, pageSize)
	query.Set("order_by", "updated_at")
	query.Set("sort", "desc")
	var batch []gitlabIssue
	headers, err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/projects/%d/issues", projectID), query), &batch)
	if err != nil {
		return nil, false, err
	}
	documents := make([]gitlabBufferedDocument, 0, len(batch))
	doneByWindow := false
	pageOffset := 0
	for _, issue := range batch {
		if beforeOrAtWindowStart(issue.UpdatedAt, windowStart) {
			doneByWindow = true
			break
		}
		if afterWindowEnd(issue.UpdatedAt, windowEnd) {
			continue
		}
		doc := issue.toSourceDocument()
		pageOffset++
		documents = append(documents, gitlabBufferedDocument{
			document:   doc,
			checkpoint: gitlabSyncCheckpoint(gitlabSyncCursor{Stage: gitlabStageIssues, Page: page, Offset: pageOffset, SourceID: doc.SourceID}, doc),
			offset:     pageOffset,
			sourceID:   doc.SourceID,
		})
	}
	done := doneByWindow || !gitlabHasNextPage(headers) || len(batch) == 0
	return documents, done, nil
}

// listTreePage returns one page of GitLab repository tree items.
func (c *GitlabConnector) listTreePage(ctx context.Context, projectID int, branch, path string, page, pageSize int) ([]gitlabTreeItem, bool, error) {
	query := url.Values{
		"per_page": {strconv.Itoa(pageSize)},
		"page":     {strconv.Itoa(page)},
		"ref":      {branch},
	}
	if path != "" {
		query.Set("path", path)
	}
	var batch []gitlabTreeItem
	headers, err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/projects/%d/repository/tree", projectID), query), &batch)
	if err != nil {
		return nil, false, err
	}
	done := !gitlabHasNextPage(headers) || len(batch) == 0
	return batch, done, nil
}

// fetchLastCommit returns the most recent commit for a file path.
func (c *GitlabConnector) fetchLastCommit(ctx context.Context, projectID int, branch, path string) (gitlabCommit, error) {
	query := url.Values{
		"ref_name": {branch},
		"path":     {path},
		"per_page": {"1"},
	}
	var commits []gitlabCommit
	if _, err := c.getJSON(ctx, c.apiURL(fmt.Sprintf("/projects/%d/repository/commits", projectID), query), &commits); err != nil {
		return gitlabCommit{}, err
	}
	if len(commits) == 0 {
		return gitlabCommit{}, nil
	}
	return commits[0], nil
}

// fetchFileRaw downloads raw file content from GitLab.
func (c *GitlabConnector) fetchFileRaw(ctx context.Context, projectID int, branch, path string) ([]byte, error) {
	encoded := url.PathEscape(path)
	apiURL := c.apiURL(fmt.Sprintf("/projects/%d/repository/files/%s/raw", projectID, encoded), url.Values{"ref": {branch}})
	return c.getRaw(ctx, apiURL)
}

// getJSON fetches a GitLab API JSON response into out.
func (c *GitlabConnector) getJSON(ctx context.Context, apiURL string, out any) (http.Header, error) {
	if c.doJSON != nil {
		return c.doJSON(ctx, apiURL, out)
	}
	hostname, resolvedIP, err := utility.AssertURLSafe(apiURL)
	if err != nil {
		return nil, err
	}
	client := utility.PinnedHTTPClient(hostname, resolvedIP, gitlabRequestTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GitLab API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitLab API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err = json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, err
	}
	return resp.Header.Clone(), nil
}

// getRaw fetches raw bytes from a GitLab API URL.
func (c *GitlabConnector) getRaw(ctx context.Context, apiURL string) ([]byte, error) {
	if c.doRaw != nil {
		return c.doRaw(ctx, apiURL)
	}
	hostname, resolvedIP, err := utility.AssertURLSafe(apiURL)
	if err != nil {
		return nil, err
	}
	client := utility.PinnedHTTPClient(hostname, resolvedIP, gitlabRequestTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GitLab raw file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitLab API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

// apiURL builds a GitLab API URL.
func (c *GitlabConnector) apiURL(path string, query url.Values) string {
	path = strings.TrimLeft(path, "/")
	u := strings.TrimRight(c.baseURL, "/") + "/" + path
	if len(query) == 0 {
		return u
	}
	return u + "?" + query.Encode()
}

// fileURL builds the web URL for a repository file.
func (c *GitlabConnector) fileURL(branch, path string) string {
	return fmt.Sprintf("%s/%s/%s/-/blob/%s/%s", c.gitlabURL, c.projectOwner, c.projectName, branch, path)
}

// projectFullName returns "owner/name".
func (c *GitlabConnector) projectFullName() string {
	return c.projectOwner + "/" + c.projectName
}

// gitlabSyncSession streams GitLab documents for one fixed sync window.
type gitlabSyncSession struct {
	connector      *GitlabConnector
	project        gitlabProject
	batchSize      int
	stage          string
	page           int
	treeQueue      []string
	treePage       int
	windowStart    *time.Time
	windowEnd      time.Time
	buffer         []gitlabBufferedDocument
	resumeStage    string
	resumePage     int
	resumeOffset   int
	resumeSourceID string
}

// NextBatch returns the next GitLab document batch.
func (s *gitlabSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
		batch, err := s.nextDocumentPage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		if len(batch) == 0 {
			if s.stage == gitlabStageDone {
				if len(documents) == 0 {
					return SyncBatch{}, io.EOF
				}
				break
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

// Close closes the GitLab sync session.
func (s *gitlabSyncSession) Close() error {
	return nil
}

// Fetch downloads a GitLab file body for a delayed source document.
func (s *gitlabSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch gitlabFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	return s.connector.fetchFileRaw(ctx, fetch.ProjectID, fetch.Ref, fetch.FilePath)
}

const (
	gitlabStageCodeFiles = "code_files"
	gitlabStageMRs       = "merge_requests"
	gitlabStageIssues    = "issues"
	gitlabStageDone      = "done"
)

// nextDocumentPage fetches one GitLab API page for sync.
func (s *gitlabSyncSession) nextDocumentPage(ctx context.Context) ([]gitlabBufferedDocument, error) {
	switch s.stage {
	case gitlabStageCodeFiles:
		if !s.connector.includeCodeFiles {
			s.advanceStage()
			return nil, nil
		}
		return s.nextCodeFilesPage(ctx)
	case gitlabStageMRs:
		if !s.connector.includeMRs {
			s.advanceStage()
			return nil, nil
		}
		docs, done, err := s.connector.listMergeRequestPage(ctx, s.project.ID, s.page, s.batchSize, s.windowStart, s.windowEnd)
		if err != nil {
			return nil, err
		}
		docs, err = s.filterResumedDocuments(gitlabStageMRs, s.page, docs)
		if err != nil {
			return nil, err
		}
		if done {
			s.advanceStage()
		} else {
			s.page++
		}
		return docs, nil
	case gitlabStageIssues:
		if !s.connector.includeIssues {
			s.advanceStage()
			return nil, nil
		}
		docs, done, err := s.connector.listIssuePage(ctx, s.project.ID, s.page, s.batchSize, s.windowStart, s.windowEnd)
		if err != nil {
			return nil, err
		}
		docs, err = s.filterResumedDocuments(gitlabStageIssues, s.page, docs)
		if err != nil {
			return nil, err
		}
		if done {
			s.advanceStage()
		} else {
			s.page++
		}
		return docs, nil
	default:
		s.stage = gitlabStageDone
		return nil, nil
	}
}

// nextCodeFilesPage fetches one tree page and converts blobs into documents.
// pendingPaths is captured before processing so resume re-discovers subtrees
// without duplicating queue entries.
func (s *gitlabSyncSession) nextCodeFilesPage(ctx context.Context) ([]gitlabBufferedDocument, error) {
	if len(s.treeQueue) == 0 {
		s.advanceStage()
		return nil, nil
	}
	currentPath := s.treeQueue[0]
	pendingPaths := append([]string(nil), s.treeQueue[1:]...)
	items, done, err := s.connector.listTreePage(ctx, s.project.ID, s.project.DefaultBranch, currentPath, s.treePage, s.batchSize)
	if err != nil {
		return nil, err
	}
	documents := make([]gitlabBufferedDocument, 0, len(items))
	pageOffset := 0
	for _, item := range items {
		if shouldExcludeGitlabPath(item.Path) {
			continue
		}
		if item.Type == "tree" {
			s.treeQueue = append(s.treeQueue, item.Path)
			continue
		}
		if item.Type != "blob" {
			continue
		}
		// The tree API does not expose a committed date, so this request still
		// supplies the update time used by window filtering.
		commit, err := s.connector.fetchLastCommit(ctx, s.project.ID, s.project.DefaultBranch, item.Path)
		if err != nil {
			return nil, err
		}
		updatedAt := time.Now().UTC()
		if !commit.CommittedDate.IsZero() {
			updatedAt = commit.CommittedDate.UTC()
		}
		if beforeOrAtWindowStart(updatedAt, s.windowStart) {
			continue
		}
		if afterWindowEnd(updatedAt, s.windowEnd) {
			continue
		}
		fileURL := s.connector.fileURL(s.project.DefaultBranch, item.Path)
		fetchRef := gitlabFetchReference{ProjectID: s.project.ID, FilePath: item.Path, Ref: s.project.DefaultBranch}
		fetchKey, _ := json.Marshal(fetchRef)
		doc := SourceDocument{
			SourceID:           fileURL,
			SemanticIdentifier: item.Name,
			Extension:          gitlabFileExt(item.Name),
			FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: 0},
			UpdatedAt:          updatedAt,
			Metadata: map[string]any{
				"type":    "CodeFile",
				"path":    item.Path,
				"ref":     s.project.DefaultBranch,
				"project": s.connector.projectFullName(),
				"web_url": fileURL,
			},
			Fingerprint: gitlabCodeFileFingerprint(item, s.project.DefaultBranch),
		}
		pageOffset++
		cursor := gitlabSyncCursor{
			Stage:        gitlabStageCodeFiles,
			Page:         s.treePage,
			Offset:       pageOffset,
			SourceID:     doc.SourceID,
			TreePath:     currentPath,
			PendingPaths: append([]string(nil), pendingPaths...),
		}
		documents = append(documents, gitlabBufferedDocument{
			document:   doc,
			checkpoint: gitlabSyncCheckpoint(cursor, doc),
			offset:     pageOffset,
			sourceID:   doc.SourceID,
		})
	}
	documents, err = s.filterResumedDocuments(gitlabStageCodeFiles, s.treePage, documents)
	if err != nil {
		return nil, err
	}
	if done {
		s.treeQueue = s.treeQueue[1:]
		s.treePage = 1
		if len(s.treeQueue) == 0 {
			s.advanceStage()
		}
	} else {
		s.treePage++
	}
	return documents, nil
}

// advanceStage moves a sync session to the next GitLab stage.
func (s *gitlabSyncSession) advanceStage() {
	switch s.stage {
	case gitlabStageCodeFiles:
		s.stage = gitlabStageMRs
	case gitlabStageMRs:
		s.stage = gitlabStageIssues
	default:
		s.stage = gitlabStageDone
	}
	s.page = 1
	s.treeQueue = nil
	s.treePage = 1
	s.clearResume()
}

// applyResume advances a sync session to the last committed GitLab position.
func (s *gitlabSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("gitlab sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor gitlabSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("gitlab sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	if cursor.Stage == "" || cursor.Page <= 0 {
		return fmt.Errorf("gitlab sync cursor has no resume anchor: %w", ErrSyncResumeInvalid)
	}
	s.stage = cursor.Stage
	s.resumeStage = cursor.Stage
	s.resumePage = cursor.Page
	s.resumeOffset = cursor.Offset
	s.resumeSourceID = firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if s.resumeSourceID == "" {
		return fmt.Errorf("gitlab sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.Stage == gitlabStageCodeFiles {
		s.treeQueue = append([]string{cursor.TreePath}, cursor.PendingPaths...)
		s.treePage = cursor.Page
	} else {
		s.page = cursor.Page
	}
	return nil
}

// filterResumedDocuments drops documents through the committed checkpoint.
func (s *gitlabSyncSession) filterResumedDocuments(stage string, page int, candidates []gitlabBufferedDocument) ([]gitlabBufferedDocument, error) {
	if s.resumeStage == "" || stage != s.resumeStage || page != s.resumePage {
		return candidates, nil
	}
	if s.resumeSourceID != "" {
		for index, candidate := range candidates {
			if candidate.sourceID == s.resumeSourceID {
				s.clearResume()
				return candidates[index+1:], nil
			}
		}
		return nil, fmt.Errorf("gitlab resume anchor %q was not found on page %d: %w", s.resumeSourceID, page, ErrSyncResumeInvalid)
	}
	return nil, fmt.Errorf("gitlab sync cursor has no source anchor: %w", ErrSyncResumeInvalid)
}

func (s *gitlabSyncSession) clearResume() {
	s.resumeStage = ""
	s.resumePage = 0
	s.resumeOffset = 0
	s.resumeSourceID = ""
}

// gitlabPruneSession streams a complete GitLab slim snapshot.
type gitlabPruneSession struct {
	connector *GitlabConnector
	project   gitlabProject
	batchSize int
	stage     string
	page      int
	treeQueue []string
	treePage  int
	buffer    []SlimDocument
}

// NextBatch returns the next GitLab prune snapshot batch.
func (s *gitlabPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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
		batch, err := s.nextSlimPage(ctx)
		if err != nil {
			return PruneBatch{}, err
		}
		if len(batch) == 0 {
			if s.stage == gitlabStageDone {
				if len(documents) == 0 {
					return PruneBatch{}, io.EOF
				}
				break
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

// Close closes the GitLab prune session.
func (s *gitlabPruneSession) Close() error {
	return nil
}

// nextSlimPage fetches one GitLab API page for prune.
func (s *gitlabPruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	switch s.stage {
	case gitlabStageCodeFiles:
		if !s.connector.includeCodeFiles {
			s.advanceStage()
			return nil, nil
		}
		return s.nextCodeFilesSlimPage(ctx)
	case gitlabStageMRs:
		if !s.connector.includeMRs {
			s.advanceStage()
			return nil, nil
		}
		return s.nextMRSlimPage(ctx)
	case gitlabStageIssues:
		if !s.connector.includeIssues {
			s.advanceStage()
			return nil, nil
		}
		return s.nextIssueSlimPage(ctx)
	default:
		s.stage = gitlabStageDone
		return nil, nil
	}
}

// nextCodeFilesSlimPage fetches one tree page for prune.
func (s *gitlabPruneSession) nextCodeFilesSlimPage(ctx context.Context) ([]SlimDocument, error) {
	if len(s.treeQueue) == 0 {
		s.advanceStage()
		return nil, nil
	}
	currentPath := s.treeQueue[0]
	items, done, err := s.connector.listTreePage(ctx, s.project.ID, s.project.DefaultBranch, currentPath, s.treePage, s.batchSize)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(items))
	for _, item := range items {
		if shouldExcludeGitlabPath(item.Path) {
			continue
		}
		if item.Type == "tree" {
			s.treeQueue = append(s.treeQueue, item.Path)
			continue
		}
		if item.Type != "blob" {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: s.connector.fileURL(s.project.DefaultBranch, item.Path)})
	}
	if done {
		s.treeQueue = s.treeQueue[1:]
		s.treePage = 1
		if len(s.treeQueue) == 0 {
			s.advanceStage()
		}
	} else {
		s.treePage++
	}
	return documents, nil
}

// nextMRSlimPage fetches one MR page for prune.
func (s *gitlabPruneSession) nextMRSlimPage(ctx context.Context) ([]SlimDocument, error) {
	query := gitlabListQuery(s.page, s.batchSize)
	var batch []gitlabMergeRequest
	headers, err := s.connector.getJSON(ctx, s.connector.apiURL(fmt.Sprintf("/projects/%d/merge_requests", s.project.ID), query), &batch)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(batch))
	for _, mr := range batch {
		documents = append(documents, SlimDocument{SourceID: mr.WebURL})
	}
	if !gitlabHasNextPage(headers) || len(batch) == 0 {
		s.advanceStage()
	} else {
		s.page++
	}
	return documents, nil
}

// nextIssueSlimPage fetches one issue page for prune.
func (s *gitlabPruneSession) nextIssueSlimPage(ctx context.Context) ([]SlimDocument, error) {
	query := gitlabListQuery(s.page, s.batchSize)
	var batch []gitlabIssue
	headers, err := s.connector.getJSON(ctx, s.connector.apiURL(fmt.Sprintf("/projects/%d/issues", s.project.ID), query), &batch)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(batch))
	for _, issue := range batch {
		documents = append(documents, SlimDocument{SourceID: issue.WebURL})
	}
	if !gitlabHasNextPage(headers) || len(batch) == 0 {
		s.advanceStage()
	} else {
		s.page++
	}
	return documents, nil
}

// advanceStage moves a prune session to the next GitLab stage.
func (s *gitlabPruneSession) advanceStage() {
	switch s.stage {
	case gitlabStageCodeFiles:
		s.stage = gitlabStageMRs
	case gitlabStageMRs:
		s.stage = gitlabStageIssues
	default:
		s.stage = gitlabStageDone
	}
	s.page = 1
	s.treeQueue = nil
	s.treePage = 1
}

type gitlabProject struct {
	ID            int    `json:"id"`
	DefaultBranch string `json:"default_branch"`
	WebURL        string `json:"web_url"`
}

type gitlabSyncCursor struct {
	Stage        string   `json:"stage"`
	Page         int      `json:"page"`
	Offset       int      `json:"offset"`
	SourceID     string   `json:"source_id"`
	TreePath     string   `json:"tree_path,omitempty"`
	PendingPaths []string `json:"pending_paths,omitempty"`
}

type gitlabBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
	sourceID   string
}

type gitlabFetchReference struct {
	ProjectID int    `json:"project_id"`
	FilePath  string `json:"file_path"`
	Ref       string `json:"ref"`
}

func gitlabSyncCheckpoint(cursor gitlabSyncCursor, doc SourceDocument) *SyncCheckpoint {
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    string(data),
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
}

type gitlabMergeRequest struct {
	IID         int         `json:"iid"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	State       string      `json:"state"`
	WebURL      string      `json:"web_url"`
	UpdatedAt   time.Time   `json:"updated_at"`
	Author      *gitlabUser `json:"author"`
}

// toSourceDocument converts a merge request into the syncer model.
func (m gitlabMergeRequest) toSourceDocument() SourceDocument {
	body := []byte(m.Description)
	author := m.Author.metadata()
	return SourceDocument{
		SourceID:           m.WebURL,
		SemanticIdentifier: m.Title,
		Extension:          ".md",
		Blob:               body,
		UpdatedAt:          m.UpdatedAt.UTC(),
		SizeBytes:          int64(len(body)),
		Metadata: map[string]any{
			"state":   m.State,
			"type":    "MergeRequest",
			"web_url": m.WebURL,
			"author":  author,
		},
		Fingerprint: stableFingerprint(map[string]any{
			"type":        "MergeRequest",
			"url":         m.WebURL,
			"title":       m.Title,
			"description": m.Description,
			"state":       m.State,
			"updated_at":  m.UpdatedAt.UTC(),
			"author":      author,
		}),
	}
}

type gitlabIssue struct {
	IID         int         `json:"iid"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	State       string      `json:"state"`
	WebURL      string      `json:"web_url"`
	UpdatedAt   time.Time   `json:"updated_at"`
	Author      *gitlabUser `json:"author"`
	Type        string      `json:"type"`
}

// toSourceDocument converts an issue into the syncer model.
func (i gitlabIssue) toSourceDocument() SourceDocument {
	body := []byte(i.Description)
	author := i.Author.metadata()
	issueType := i.Type
	if issueType == "" {
		issueType = "Issue"
	}
	return SourceDocument{
		SourceID:           i.WebURL,
		SemanticIdentifier: i.Title,
		Extension:          ".md",
		Blob:               body,
		UpdatedAt:          i.UpdatedAt.UTC(),
		SizeBytes:          int64(len(body)),
		Metadata: map[string]any{
			"state":   i.State,
			"type":    issueType,
			"web_url": i.WebURL,
			"author":  author,
		},
		Fingerprint: stableFingerprint(map[string]any{
			"type":        issueType,
			"url":         i.WebURL,
			"title":       i.Title,
			"description": i.Description,
			"state":       i.State,
			"updated_at":  i.UpdatedAt.UTC(),
			"author":      author,
		}),
	}
}

type gitlabTreeItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Mode string `json:"mode"`
}

func gitlabCodeFileFingerprint(item gitlabTreeItem, ref string) string {
	return stableFingerprint(map[string]any{
		"type": "CodeFile",
		"path": item.Path,
		"ref":  ref,
		"id":   item.ID,
	})
}

type gitlabCommit struct {
	ID            string    `json:"id"`
	CommittedDate time.Time `json:"committed_date"`
}

type gitlabUser struct {
	Name     string `json:"name"`
	Username string `json:"username"`
}

// metadata returns user metadata.
func (u *gitlabUser) metadata() map[string]string {
	if u == nil {
		return nil
	}
	out := map[string]string{}
	if u.Name != "" {
		out["name"] = u.Name
	}
	if u.Username != "" {
		out["username"] = u.Username
	}
	return out
}

// gitlabListQuery builds standard GitLab list query parameters.
func gitlabListQuery(page, pageSize int) url.Values {
	if pageSize <= 0 {
		pageSize = defaultGitlabBatchSize
	}
	return url.Values{
		"state":    {"all"},
		"per_page": {strconv.Itoa(pageSize)},
		"page":     {strconv.Itoa(page)},
	}
}

// gitlabHasNextPage reports whether a GitLab response has a next page.
func gitlabHasNextPage(headers http.Header) bool {
	nextPage := strings.TrimSpace(headers.Get("X-Next-Page"))
	if nextPage != "" && nextPage != "0" {
		return true
	}
	return strings.Contains(headers.Get("Link"), `rel="next"`)
}

// shouldExcludeGitlabPath reports whether a path matches an exclude pattern.
func shouldExcludeGitlabPath(path string) bool {
	for _, pattern := range gitlabExcludePatterns {
		pattern = strings.TrimSuffix(pattern, "/")
		if path == pattern || strings.HasPrefix(path, pattern+"/") {
			return true
		}
	}
	return false
}

// gitlabFileExt returns the lowercased file extension.
func gitlabFileExt(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return ""
	}
	return strings.ToLower(ext)
}

// gitlabProjectPath URL-encodes "owner/name" for the GitLab projects API.
func gitlabProjectPath(owner, name string) string {
	return url.QueryEscape(owner + "/" + name)
}
