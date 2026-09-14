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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/utility"
)

const (
	asanaDefaultBatchSize     = 2
	asanaDefaultSizeThreshold = 20 * 1024 * 1024
	asanaAPIBaseURL           = "https://app.asana.com/api/1.0"
	asanaRequestTimeout       = 60 * time.Second
	asanaRetryCount           = 4
	asanaRetryBaseDelay       = 200 * time.Millisecond
	asanaMaxRetryDelay        = 30 * time.Second
	asanaMaxJSONResponseSize  = 32 * 1024 * 1024
	asanaPageSize             = 100
	asanaTaskOptFields        = "gid,name,notes,permalink_url,created_at,modified_at,completed_at,due_on,created_by.name,memberships.project.gid,memberships.project.name"
	asanaProjectOptFields     = "gid,name,archived,team.gid,privacy_setting"
	asanaStoryOptFields       = "text,created_at,created_by.name,resource_subtype"
	asanaAttachmentOptFields  = "gid,name,download_url,size,created_at"
	asanaSourcePrefix         = "asana:"
)

// AsanaConnector reads Asana tasks, comments, and attachments.
type AsanaConnector struct {
	workspaceID   string
	projectIDs    []string
	teamID        string
	token         string
	batchSize     int
	sizeThreshold int64
	apiBaseURL    string
	httpClient    *http.Client

	doJSON   func(ctx context.Context, apiPath string, query url.Values, out any) error
	download func(ctx context.Context, rawURL string, maxSize int64) ([]byte, error)
}

type asanaAPIError struct {
	Status  int
	Message string
}

func (e *asanaAPIError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("Asana API returned HTTP %d: %s", e.Status, e.Message)
}

type asanaNextPage struct {
	Offset string `json:"offset"`
}

type asanaListEnvelope struct {
	Data     json.RawMessage `json:"data"`
	NextPage *asanaNextPage  `json:"next_page"`
}

type asanaObjectEnvelope struct {
	Data json.RawMessage `json:"data"`
}

type asanaWorkspace struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type asanaUser struct {
	GID   string `json:"gid"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type asanaProject struct {
	GID            string `json:"gid"`
	Name           string `json:"name"`
	Archived       bool   `json:"archived"`
	PrivacySetting string `json:"privacy_setting"`
	Team           *struct {
		GID string `json:"gid"`
	} `json:"team"`
}

type asanaTaskMembership struct {
	Project asanaProject `json:"project"`
}

type asanaTask struct {
	GID          string                `json:"gid"`
	Name         string                `json:"name"`
	Notes        string                `json:"notes"`
	PermalinkURL string                `json:"permalink_url"`
	CreatedAt    string                `json:"created_at"`
	ModifiedAt   string                `json:"modified_at"`
	CompletedAt  string                `json:"completed_at"`
	DueOn        string                `json:"due_on"`
	CreatedBy    asanaUser             `json:"created_by"`
	Memberships  []asanaTaskMembership `json:"memberships"`
}

type asanaStory struct {
	GID             string    `json:"gid"`
	ResourceSubtype string    `json:"resource_subtype"`
	Text            string    `json:"text"`
	CreatedAt       string    `json:"created_at"`
	CreatedBy       asanaUser `json:"created_by"`
}

type asanaAttachment struct {
	GID         string `json:"gid"`
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	CreatedAt   string `json:"created_at"`
}

type asanaFetchReference struct {
	TaskGID       string `json:"task_gid"`
	AttachmentGID string `json:"attachment_gid"`
	Filename      string `json:"filename"`
	DownloadURL   string `json:"download_url"`
	Size          int64  `json:"size"`
}

type asanaSyncCursor struct {
	ProjectGID string `json:"project_gid"`
	PageOffset string `json:"page_offset,omitempty"`
	SourceID   string `json:"source_id"`
}

// NewAsanaConnector creates an Asana connector from Python-compatible config.
func NewAsanaConnector(config map[string]any) (*AsanaConnector, error) {
	credentials := configAnyMap(config["credentials"])
	connector := &AsanaConnector{
		workspaceID: strings.TrimSpace(stringConfig(config["asana_workspace_id"])),
		projectIDs:  asanaProjectIDList(stringConfig(config["asana_project_ids"])),
		teamID:      strings.TrimSpace(stringConfig(config["asana_team_id"])),
		token:       strings.TrimSpace(stringConfig(credentials["asana_api_token_secret"])),
		batchSize:   configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), asanaDefaultBatchSize),
		apiBaseURL:  asanaAPIBaseURL,
		httpClient:  &http.Client{Timeout: asanaRequestTimeout},
	}
	connector.sizeThreshold = int64(configInt(config["size_threshold"], asanaDefaultSizeThreshold))
	if connector.sizeThreshold <= 0 {
		connector.sizeThreshold = asanaDefaultSizeThreshold
	}
	connector.doJSON = connector.doAsanaJSON
	connector.download = connector.downloadAsanaFile
	return connector, nil
}

func asanaProjectIDList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

// Validate validates Asana settings and credentials.
func (c *AsanaConnector) Validate(ctx context.Context) error {
	if err := c.validateStatic(); err != nil {
		return err
	}
	var envelope asanaObjectEnvelope
	if err := c.doJSON(ctx, "workspaces/"+url.PathEscape(c.workspaceID), url.Values{"opt_fields": {"gid", "name"}}, &envelope); err != nil {
		return classifyAsanaError(err)
	}
	var workspace asanaWorkspace
	if err := json.Unmarshal(envelope.Data, &workspace); err != nil {
		return err
	}
	if workspace.GID == "" {
		return &ConnectorValidationError{Message: "Asana returned an invalid workspace response."}
	}
	return nil
}

// ValidateConnectorSetting validates an unsaved Asana config.
func (c *AsanaConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	candidate, err := NewAsanaConnector(request)
	if err != nil {
		return err
	}
	if c != nil {
		if c.httpClient != nil {
			candidate.httpClient = c.httpClient
		}
		if c.apiBaseURL != "" {
			candidate.apiBaseURL = c.apiBaseURL
		}
	}
	return candidate.Validate(ctx)
}

// OpenSync opens one Asana sync session.
func (c *AsanaConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	session := &asanaSyncSession{
		connector: c,
		request:   request,
		batchSize: c.effectiveBatchSize(),
		seenTasks: map[string]bool{},
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Asana prune snapshot session.
func (c *AsanaConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	return &asanaPruneSession{
		connector: c,
		batchSize: c.effectiveBatchSize(),
		seenTasks: map[string]bool{},
	}, nil
}

// Fetch downloads a delayed Asana attachment.
func (c *AsanaConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch asanaFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	if fetch.TaskGID == "" || fetch.AttachmentGID == "" || fetch.DownloadURL == "" {
		return nil, fmt.Errorf("asana fetch reference is incomplete")
	}
	if fetch.Size > c.sizeThreshold {
		return nil, fmt.Errorf("%s exceeds size threshold of %d", firstNonEmpty(fetch.Filename, fetch.AttachmentGID), c.sizeThreshold)
	}
	if c.download != nil {
		return c.download(ctx, fetch.DownloadURL, c.sizeThreshold)
	}
	return c.downloadAsanaFile(ctx, fetch.DownloadURL, c.sizeThreshold)
}

func (c *AsanaConnector) validateStatic() error {
	if c == nil {
		return &ConnectorValidationError{Message: "Asana connector is nil"}
	}
	if c.workspaceID == "" {
		return &ConnectorValidationError{Message: "Asana workspace_id is required"}
	}
	if c.token == "" {
		return &ConnectorMissingCredentialError{Message: "Asana asana_api_token_secret is required"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "Asana connector batch_size must be a positive integer"}
	}
	if c.sizeThreshold <= 0 {
		c.sizeThreshold = asanaDefaultSizeThreshold
	}
	if c.apiBaseURL == "" {
		c.apiBaseURL = asanaAPIBaseURL
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: asanaRequestTimeout}
	}
	return nil
}

func (c *AsanaConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return asanaDefaultBatchSize
}

func (c *AsanaConnector) apiURL(apiPath string, query url.Values) string {
	base := strings.TrimRight(c.apiBaseURL, "/") + "/" + strings.TrimPrefix(apiPath, "/")
	if len(query) == 0 {
		return base
	}
	return base + "?" + query.Encode()
}

func (c *AsanaConnector) doAsanaJSON(ctx context.Context, apiPath string, query url.Values, out any) error {
	delay := asanaRetryBaseDelay
	var lastErr error
	for attempt := 1; attempt <= asanaRetryCount; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, asanaRequestTimeout)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.apiURL(apiPath, query), nil)
		if err != nil {
			cancel()
			return err
		}
		request.Header.Set("Authorization", "Bearer "+c.token)
		request.Header.Set("Accept", "application/json")

		response, err := c.httpClient.Do(request)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, asanaMaxJSONResponseSize+1))
			_ = response.Body.Close()
			cancel()
			if readErr != nil {
				return readErr
			}
			if response.StatusCode >= 400 {
				lastErr = &asanaAPIError{Status: response.StatusCode, Message: strings.TrimSpace(string(body))}
				if !isAsanaRetryable(response.StatusCode) {
					return lastErr
				}
			} else {
				if int64(len(body)) > asanaMaxJSONResponseSize {
					return fmt.Errorf("asana API response exceeds maximum size of %d bytes", asanaMaxJSONResponseSize)
				}
				if err := json.Unmarshal(body, out); err != nil {
					return err
				}
				return nil
			}
		}
		if attempt == asanaRetryCount {
			break
		}
		wait := delay
		if response != nil {
			wait = asanaRetryAfter(response, delay)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		delay = min(delay*2, asanaMaxRetryDelay)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Asana request failed after %d attempts", asanaRetryCount)
	}
	return lastErr
}

func isAsanaRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

func asanaRetryAfter(response *http.Response, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(response.Header.Get("Retry-After"))
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		wait := time.Duration(seconds) * time.Second
		return min(wait, asanaMaxRetryDelay)
	}
	if retryTime, err := http.ParseTime(value); err == nil {
		wait := time.Until(retryTime)
		if wait < 0 {
			wait = 0
		}
		return min(wait, asanaMaxRetryDelay)
	}
	return fallback
}

func classifyAsanaError(err error) error {
	var apiErr *asanaAPIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Status {
	case http.StatusUnauthorized:
		return &ConnectorMissingCredentialError{Message: "Asana access token is invalid or expired."}
	case http.StatusForbidden:
		return &ConnectorValidationError{Message: "Asana token does not have permission to read the requested workspace or project."}
	case http.StatusNotFound:
		return &ConnectorValidationError{Message: "Asana workspace or project was not found."}
	case http.StatusTooManyRequests:
		return &ConnectorValidationError{Message: "Asana rate limit exceeded during validation."}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("Asana validation failed (HTTP %d): %s", apiErr.Status, apiErr.Message)}
	}
}

func (c *AsanaConnector) listPage(ctx context.Context, apiPath string, query url.Values, offset string, out any) (string, error) {
	query.Set("limit", strconv.Itoa(asanaPageSize))
	if offset != "" {
		query.Set("offset", offset)
	}
	var envelope asanaListEnvelope
	if err := c.doJSON(ctx, apiPath, query, &envelope); err != nil {
		return "", err
	}
	if len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return "", err
		}
	}
	if envelope.NextPage == nil {
		return "", nil
	}
	return envelope.NextPage.Offset, nil
}

func (c *AsanaConnector) selectProjects(ctx context.Context) ([]asanaProject, error) {
	projects := make([]asanaProject, 0)
	if len(c.projectIDs) > 0 {
		for _, projectID := range c.projectIDs {
			project, err := c.getProject(ctx, projectID)
			if err != nil {
				return nil, err
			}
			if c.isProjectProcessable(project) {
				projects = append(projects, project)
			}
		}
	} else {
		query := url.Values{
			"workspace":  {c.workspaceID},
			"opt_fields": {asanaProjectOptFields},
		}
		offset := ""
		for {
			var page []asanaProject
			next, err := c.listPage(ctx, "projects", query, offset, &page)
			if err != nil {
				return nil, err
			}
			for _, project := range page {
				if c.isProjectProcessable(project) {
					projects = append(projects, project)
				}
			}
			if next == "" {
				break
			}
			if next == offset {
				return nil, fmt.Errorf("Asana projects pagination did not advance")
			}
			offset = next
		}
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].GID < projects[j].GID
	})
	return projects, nil
}

func (c *AsanaConnector) getProject(ctx context.Context, projectGID string) (asanaProject, error) {
	var envelope asanaObjectEnvelope
	if err := c.doJSON(ctx, "projects/"+url.PathEscape(projectGID), url.Values{"opt_fields": {asanaProjectOptFields}}, &envelope); err != nil {
		return asanaProject{}, err
	}
	var project asanaProject
	if err := json.Unmarshal(envelope.Data, &project); err != nil {
		return asanaProject{}, err
	}
	return project, nil
}

func (c *AsanaConnector) isProjectProcessable(project asanaProject) bool {
	if project.GID == "" || project.Archived {
		return false
	}
	if project.Team == nil || project.Team.GID == "" {
		return false
	}
	if project.PrivacySetting == "private" && c.teamID != "" && project.Team.GID != c.teamID {
		return false
	}
	return true
}

func (c *AsanaConnector) listTasksPage(ctx context.Context, projectGID, offset string, since *time.Time) ([]asanaTask, string, error) {
	query := url.Values{
		"project":    {projectGID},
		"opt_fields": {asanaTaskOptFields},
	}
	if since != nil && !since.IsZero() {
		query.Set("modified_since", since.UTC().Format(time.RFC3339Nano))
	}
	var tasks []asanaTask
	next, err := c.listPage(ctx, "tasks", query, offset, &tasks)
	return tasks, next, err
}

func (c *AsanaConnector) listStories(ctx context.Context, taskGID string) ([]asanaStory, error) {
	query := url.Values{"opt_fields": {asanaStoryOptFields}}
	offset := ""
	stories := make([]asanaStory, 0)
	for {
		var page []asanaStory
		next, err := c.listPage(ctx, "tasks/"+url.PathEscape(taskGID)+"/stories", query, offset, &page)
		if err != nil {
			return nil, err
		}
		stories = append(stories, page...)
		if next == "" {
			break
		}
		if next == offset {
			return nil, fmt.Errorf("Asana stories pagination did not advance")
		}
		offset = next
	}
	sort.Slice(stories, func(i, j int) bool {
		left := parseOutlookTime(stories[i].CreatedAt)
		right := parseOutlookTime(stories[j].CreatedAt)
		if !left.Equal(right) {
			return left.Before(right)
		}
		return stories[i].GID < stories[j].GID
	})
	return stories, nil
}

func (c *AsanaConnector) listAttachments(ctx context.Context, taskGID string) ([]asanaAttachment, error) {
	query := url.Values{
		"parent":     {taskGID},
		"opt_fields": {asanaAttachmentOptFields},
	}
	offset := ""
	attachments := make([]asanaAttachment, 0)
	for {
		var page []asanaAttachment
		next, err := c.listPage(ctx, "attachments", query, offset, &page)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, page...)
		if next == "" {
			break
		}
		if next == offset {
			return nil, fmt.Errorf("Asana attachments pagination did not advance")
		}
		offset = next
	}
	sort.Slice(attachments, func(i, j int) bool {
		return attachments[i].GID < attachments[j].GID
	})
	return attachments, nil
}

func (c *AsanaConnector) taskDocument(task asanaTask, project asanaProject, comments []asanaStory) (SourceDocument, bool) {
	if strings.TrimSpace(task.GID) == "" {
		return SourceDocument{}, false
	}
	updatedAt := parseOutlookTime(task.ModifiedAt)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	blob := []byte(asanaTaskBlob(task, project, comments))
	name := strings.TrimSpace(task.Name)
	if name == "" {
		name = "Untitled task " + task.GID
	}
	return SourceDocument{
		SourceID:           asanaTaskSourceID(task.GID),
		SemanticIdentifier: name,
		Extension:          ".md",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata: map[string]any{
			"task_gid":   task.GID,
			"project":    project.Name,
			"project_id": project.GID,
			"url":        task.PermalinkURL,
		},
		Fingerprint: contentFingerprint(blob),
	}, true
}

func asanaTaskSourceID(taskGID string) string {
	return asanaSourcePrefix + taskGID
}

func asanaAttachmentSourceID(taskGID, attachmentGID string) string {
	return asanaTaskSourceID(taskGID) + ":" + attachmentGID
}

func asanaTaskBlob(task asanaTask, project asanaProject, comments []asanaStory) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(firstNonEmpty(task.Name, "Untitled task")))
	builder.WriteString("\n\n")
	if notes := strings.TrimSpace(task.Notes); notes != "" {
		builder.WriteString(notes)
		builder.WriteString("\n\n")
	}
	if project.Name != "" {
		builder.WriteString("Project: ")
		builder.WriteString(project.Name)
		builder.WriteString("\n")
	}
	if task.PermalinkURL != "" {
		builder.WriteString("URL: ")
		builder.WriteString(task.PermalinkURL)
		builder.WriteString("\n")
	}
	if task.CreatedBy.Name != "" {
		builder.WriteString("Created by: ")
		builder.WriteString(task.CreatedBy.Name)
		builder.WriteString("\n")
	}
	if created := parseOutlookTime(task.CreatedAt); !created.IsZero() {
		builder.WriteString("Created: ")
		builder.WriteString(created.Format("2006-01-02"))
		builder.WriteString("\n")
	}
	if due := strings.TrimSpace(task.DueOn); due != "" {
		builder.WriteString("Due date: ")
		builder.WriteString(due)
		builder.WriteString("\n")
	}
	if completed := parseOutlookTime(task.CompletedAt); !completed.IsZero() {
		builder.WriteString("Completed on: ")
		builder.WriteString(completed.Format("2006-01-02"))
		builder.WriteString("\n")
	}

	var taskComments []asanaStory
	for _, story := range comments {
		if story.ResourceSubtype != "comment_added" || strings.TrimSpace(story.Text) == "" {
			continue
		}
		taskComments = append(taskComments, story)
	}
	if len(taskComments) > 0 {
		builder.WriteString("\n## Comments\n\n")
		for _, comment := range taskComments {
			if comment.CreatedBy.Name != "" {
				builder.WriteString("Comment by ")
				builder.WriteString(comment.CreatedBy.Name)
				builder.WriteString(": ")
			} else {
				builder.WriteString("Comment: ")
			}
			builder.WriteString(strings.TrimSpace(comment.Text))
			builder.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(builder.String()) + "\n"
}

func (c *AsanaConnector) attachmentDocument(task asanaTask, project asanaProject, attachment asanaAttachment) (SourceDocument, bool) {
	if task.GID == "" || attachment.GID == "" || attachment.DownloadURL == "" || attachment.Name == "" {
		return SourceDocument{}, false
	}
	if attachment.Size < 0 || (attachment.Size > 0 && attachment.Size > c.sizeThreshold) {
		return SourceDocument{}, false
	}
	updatedAt := parseOutlookTime(task.ModifiedAt)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	fetch, _ := json.Marshal(asanaFetchReference{
		TaskGID:       task.GID,
		AttachmentGID: attachment.GID,
		Filename:      attachment.Name,
		DownloadURL:   attachment.DownloadURL,
		Size:          attachment.Size,
	})
	return SourceDocument{
		SourceID:           asanaAttachmentSourceID(task.GID, attachment.GID),
		SemanticIdentifier: attachment.Name,
		Extension:          strings.ToLower(filepath.Ext(attachment.Name)),
		FetchRef:           &FetchReference{Key: string(fetch), SizeHint: attachment.Size},
		UpdatedAt:          updatedAt,
		SizeBytes:          attachment.Size,
		Metadata: map[string]any{
			"task_gid":       task.GID,
			"attachment_gid": attachment.GID,
			"project":        project.Name,
			"project_id":     project.GID,
			"filename":       attachment.Name,
			"url":            attachment.DownloadURL,
			"size":           attachment.Size,
		},
		Fingerprint: asanaAttachmentFingerprint(task.GID, attachment),
	}, true
}

func asanaAttachmentFingerprint(taskGID string, attachment asanaAttachment) string {
	return stableFingerprint(map[string]any{
		"task_gid":       taskGID,
		"attachment_gid": attachment.GID,
		"filename":       attachment.Name,
		"size":           attachment.Size,
		"created_at":     attachment.CreatedAt,
	})
}

func includeAsanaDocument(request SyncRequest, document SourceDocument) bool {
	if request.FromBeginning {
		return true
	}
	if len(request.Fingerprints) > 0 {
		stored, ok := request.Fingerprints[document.SourceID]
		return !ok || stored == "" || stored != document.Fingerprint
	}
	return !beforeOrAtWindowStart(document.UpdatedAt, request.WindowStart) && !afterWindowEnd(document.UpdatedAt, request.WindowEnd)
}

func (c *AsanaConnector) downloadAsanaFile(ctx context.Context, rawURL string, maxSize int64) ([]byte, error) {
	data, _, _, err := utility.FetchRemoteFileSafely(ctx, rawURL, maxSize)
	return data, err
}

// asanaSyncSession streams Asana task and attachment documents for one window.
type asanaSyncSession struct {
	connector         *AsanaConnector
	request           SyncRequest
	batchSize         int
	projects          []asanaProject
	projectIdx        int
	pageOffset        string
	currentPageOffset string
	taskBuffer        []asanaTask
	taskIdx           int
	taskPageDone      bool
	current           asanaProject
	documents         []asanaBufferedDocument
	docIdx            int
	seenTasks         map[string]bool
	projectsOK        bool
	done              bool

	resumeProjectGID string
	resumePageOffset string
	resumeSourceID   string
	resumeMatched    bool
}

type asanaBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
}

func (s *asanaSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	var checkpoint *SyncCheckpoint

	for len(documents) < s.batchSize {
		if s.docIdx < len(s.documents) {
			buffered := s.documents[s.docIdx]
			s.docIdx++
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
			continue
		}
		if s.done {
			if s.resumeSourceID != "" && !s.resumeMatched {
				return SyncBatch{}, fmt.Errorf("Asana resume anchor %q was not found in the current listing: %w", s.resumeSourceID, ErrSyncResumeInvalid)
			}
			if len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			break
		}
		if !s.projectsOK {
			if err := s.loadProjects(ctx); err != nil {
				return SyncBatch{}, err
			}
			if len(s.projects) == 0 {
				s.done = true
				continue
			}
		}
		if s.taskIdx >= len(s.taskBuffer) {
			if s.projectIdx >= len(s.projects) {
				s.done = true
				continue
			}
			if s.taskPageDone {
				s.projectIdx++
				s.pageOffset = ""
				s.taskBuffer = nil
				s.taskPageDone = false
				continue
			}
			nextPage, err := s.loadTaskPage(ctx)
			if err != nil {
				return SyncBatch{}, err
			}
			if len(s.taskBuffer) == 0 {
				if nextPage == "" {
					s.projectIdx++
					s.pageOffset = ""
					s.taskBuffer = nil
					s.taskPageDone = false
				}
				continue
			}
		}

		task := s.taskBuffer[s.taskIdx]
		s.taskIdx++
		if s.seenTasks[task.GID] {
			continue
		}
		s.seenTasks[task.GID] = true
		docs, err := s.taskDocuments(ctx, task)
		if err != nil {
			return SyncBatch{}, err
		}
		docs, err = s.filterResumedDocuments(docs)
		if err != nil {
			return SyncBatch{}, err
		}
		s.documents = docs
		s.docIdx = 0
	}

	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

func (s *asanaSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed Asana attachment for this sync session.
func (s *asanaSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *asanaSyncSession) loadProjects(ctx context.Context) error {
	projects, err := s.connector.selectProjects(ctx)
	if err != nil {
		return err
	}
	s.projects = projects
	s.projectsOK = true
	if s.resumeProjectGID != "" {
		found := false
		for index, project := range projects {
			if project.GID == s.resumeProjectGID {
				s.projectIdx = index
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Asana resume project %q was not found in the current listing: %w", s.resumeProjectGID, ErrSyncResumeInvalid)
		}
		s.pageOffset = s.resumePageOffset
	}
	return nil
}

func (s *asanaSyncSession) loadTaskPage(ctx context.Context) (string, error) {
	project := s.projects[s.projectIdx]
	s.current = project
	s.currentPageOffset = s.pageOffset
	tasks, next, err := s.connector.listTasksPage(ctx, project.GID, s.pageOffset, asanaSyncSince(s.request))
	if err != nil {
		return "", err
	}
	if next != "" && next == s.pageOffset {
		return "", fmt.Errorf("Asana tasks pagination did not advance for project %s", project.GID)
	}
	s.taskBuffer = tasks
	s.taskIdx = 0
	s.pageOffset = next
	s.taskPageDone = next == ""
	return next, nil
}

func asanaSyncSince(request SyncRequest) *time.Time {
	if request.FromBeginning {
		return nil
	}
	return request.WindowStart
}

func (s *asanaSyncSession) taskDocuments(ctx context.Context, task asanaTask) ([]asanaBufferedDocument, error) {
	stories, err := s.connector.listStories(ctx, task.GID)
	if err != nil {
		return nil, err
	}
	taskDoc, ok := s.connector.taskDocument(task, s.current, stories)
	if !ok {
		return nil, nil
	}
	documents := []asanaBufferedDocument{}
	if includeAsanaDocument(s.request, taskDoc) {
		documents = append(documents, asanaBufferedDocument{
			document:   taskDoc,
			checkpoint: s.checkpoint(taskDoc),
		})
	}

	attachments, err := s.connector.listAttachments(ctx, task.GID)
	if err != nil {
		return nil, err
	}
	for _, attachment := range attachments {
		doc, ok := s.connector.attachmentDocument(task, s.current, attachment)
		if !ok {
			continue
		}
		if includeAsanaDocument(s.request, doc) {
			documents = append(documents, asanaBufferedDocument{
				document:   doc,
				checkpoint: s.checkpoint(doc),
			})
		}
	}
	return documents, nil
}

func (s *asanaSyncSession) checkpoint(document SourceDocument) *SyncCheckpoint {
	cursor, err := json.Marshal(asanaSyncCursor{
		ProjectGID: s.current.GID,
		PageOffset: s.currentPageOffset,
		SourceID:   document.SourceID,
	})
	if err != nil {
		return nil
	}
	updatedAt := document.UpdatedAt
	return &SyncCheckpoint{Cursor: string(cursor), SourceID: document.SourceID, UpdatedAt: &updatedAt}
}

func (s *asanaSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("Asana sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor asanaSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("Asana sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" || !strings.HasPrefix(sourceID, asanaSourcePrefix) {
		return fmt.Errorf("Asana sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.ProjectGID == "" {
		return fmt.Errorf("Asana sync checkpoint has no project anchor: %w", ErrSyncResumeInvalid)
	}
	s.resumeProjectGID = cursor.ProjectGID
	s.resumePageOffset = cursor.PageOffset
	s.resumeSourceID = sourceID
	return nil
}

func (s *asanaSyncSession) filterResumedDocuments(candidates []asanaBufferedDocument) ([]asanaBufferedDocument, error) {
	if s.resumeSourceID == "" || s.resumeMatched {
		return candidates, nil
	}
	for index, candidate := range candidates {
		if candidate.document.SourceID == s.resumeSourceID {
			s.resumeMatched = true
			s.resumeSourceID = ""
			return candidates[index+1:], nil
		}
	}
	return nil, nil
}

// asanaPruneSession streams the complete Asana slim snapshot.
type asanaPruneSession struct {
	connector    *AsanaConnector
	batchSize    int
	projects     []asanaProject
	projectIdx   int
	pageOffset   string
	taskBuffer   []asanaTask
	taskIdx      int
	taskPageDone bool
	seenTasks    map[string]bool
	projectsOK   bool
	done         bool
}

func (s *asanaPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		if s.done {
			if len(documents) == 0 {
				return PruneBatch{}, io.EOF
			}
			break
		}
		if !s.projectsOK {
			projects, err := s.connector.selectProjects(ctx)
			if err != nil {
				return PruneBatch{}, err
			}
			s.projects = projects
			s.projectsOK = true
			if len(projects) == 0 {
				s.done = true
				continue
			}
		}
		if s.taskIdx >= len(s.taskBuffer) {
			if s.projectIdx >= len(s.projects) {
				s.done = true
				continue
			}
			if s.taskPageDone {
				s.projectIdx++
				s.pageOffset = ""
				s.taskBuffer = nil
				s.taskPageDone = false
				continue
			}
			next, err := s.loadTaskPage(ctx)
			if err != nil {
				return PruneBatch{}, err
			}
			if len(s.taskBuffer) == 0 {
				if next == "" {
					s.projectIdx++
					s.pageOffset = ""
					s.taskBuffer = nil
					s.taskPageDone = false
				}
				continue
			}
		}

		task := s.taskBuffer[s.taskIdx]
		s.taskIdx++
		if task.GID == "" || s.seenTasks[task.GID] {
			continue
		}
		s.seenTasks[task.GID] = true
		documents = append(documents, SlimDocument{SourceID: asanaTaskSourceID(task.GID)})
		attachments, err := s.connector.listAttachments(ctx, task.GID)
		if err != nil {
			return PruneBatch{}, err
		}
		for _, attachment := range attachments {
			if attachment.GID != "" {
				documents = append(documents, SlimDocument{SourceID: asanaAttachmentSourceID(task.GID, attachment.GID)})
			}
		}
	}
	if len(documents) == 0 {
		return PruneBatch{}, io.EOF
	}
	return PruneBatch{Documents: documents}, nil
}

func (s *asanaPruneSession) Close() error {
	return nil
}

func (s *asanaPruneSession) loadTaskPage(ctx context.Context) (string, error) {
	project := s.projects[s.projectIdx]
	tasks, next, err := s.connector.listTasksPage(ctx, project.GID, s.pageOffset, nil)
	if err != nil {
		return "", err
	}
	if next != "" && next == s.pageOffset {
		return "", fmt.Errorf("Asana prune tasks pagination did not advance for project %s", project.GID)
	}
	s.taskBuffer = tasks
	s.taskIdx = 0
	s.pageOffset = next
	s.taskPageDone = next == ""
	return next, nil
}
