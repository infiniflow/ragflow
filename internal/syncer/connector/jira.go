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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultJiraBatchSize           = 32
	defaultJiraAttachmentSizeLimit = 10 * 1024 * 1024
	defaultJiraTicketSizeLimit     = 100 * 1024
	defaultJiraTimeBuffer          = time.Minute
	jiraRequestTimeout             = 60 * time.Second
	jiraMaxFullPageSize            = 50
	jiraSlimPageSize               = 500
	jiraCloudAPIVersion            = "3"
	jiraServerAPIVersion           = "2"
	jiraDefaultFields              = "summary,description,updated,created,status,priority,assignee,reporter,labels,issuetype,project,comment,attachment"
	jiraSlimFields                 = "key,project"
)

// JiraConnector reads Jira issues and optional attachments.
type JiraConnector struct {
	baseURL               string
	projectKey            string
	jqlQuery              string
	userEmail             string
	apiToken              string
	password              string
	restAPIVersion        string
	includeComments       bool
	includeAttachments    bool
	scopedToken           bool
	batchSize             int
	attachmentSizeLimit   int64
	maxTicketSize         int
	timezoneOffset        float64
	timezoneOffsetSet     bool
	timeBuffer            time.Duration
	labelsToSkip          map[string]struct{}
	commentEmailBlacklist map[string]struct{}
	client                *http.Client
	doJSON                func(ctx context.Context, method, apiPath string, query url.Values, body any, out any) (http.Header, error)
	download              func(ctx context.Context, rawURL string) ([]byte, error)
}

// NewJiraConnector creates a Jira connector from Python-compatible config.
func NewJiraConnector(config map[string]any) (*JiraConnector, error) {
	credentials := configAnyMap(config["credentials"])
	baseURL := strings.TrimRight(strings.TrimSpace(stringConfig(config["base_url"])), "/")
	timezoneOffset, timezoneOffsetSet := jiraFloatConfig(config["timezone_offset"])
	timeBuffer := defaultJiraTimeBuffer
	if config["time_buffer_seconds"] != nil {
		timeBuffer = time.Duration(configInt(config["time_buffer_seconds"], int(defaultJiraTimeBuffer.Seconds()))) * time.Second
	}
	restVersion := strings.TrimSpace(stringConfig(credentials["rest_api_version"]))
	if restVersion == "" {
		if firstNonEmpty(stringConfig(credentials["jira_api_token"]), stringConfig(credentials["token"]), stringConfig(credentials["api_token"])) != "" {
			restVersion = jiraCloudAPIVersion
		} else {
			restVersion = jiraServerAPIVersion
		}
	}
	c := &JiraConnector{
		baseURL:               baseURL,
		projectKey:            strings.TrimSpace(stringConfig(config["project_key"])),
		jqlQuery:              strings.TrimSpace(stringConfig(config["jql_query"])),
		userEmail:             strings.TrimSpace(firstNonEmpty(stringConfig(credentials["jira_user_email"]), stringConfig(credentials["jira_username"]))),
		apiToken:              strings.TrimSpace(firstNonEmpty(stringConfig(credentials["jira_api_token"]), stringConfig(credentials["token"]), stringConfig(credentials["api_token"]))),
		password:              stringConfig(firstNonEmpty(stringConfig(credentials["jira_password"]), stringConfig(credentials["password"]))),
		restAPIVersion:        restVersion,
		includeComments:       configBoolDefault(config["include_comments"], true),
		includeAttachments:    configBoolDefault(config["include_attachments"], false),
		scopedToken:           configBoolDefault(config["scoped_token"], false),
		batchSize:             configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultJiraBatchSize),
		attachmentSizeLimit:   int64(configInt(config["attachment_size_limit"], defaultJiraAttachmentSizeLimit)),
		maxTicketSize:         jiraEnvInt("JIRA_CONNECTOR_MAX_TICKET_SIZE", defaultJiraTicketSizeLimit),
		timezoneOffset:        timezoneOffset,
		timezoneOffsetSet:     timezoneOffsetSet,
		timeBuffer:            timeBuffer,
		labelsToSkip:          jiraStringSet(jiraStringSlice(config["labels_to_skip"])),
		commentEmailBlacklist: jiraStringSet(jiraStringSlice(config["comment_email_blacklist"])),
		client:                &http.Client{Timeout: jiraRequestTimeout},
	}
	c.client.CheckRedirect = c.checkRedirect
	if len(c.labelsToSkip) == 0 {
		c.labelsToSkip = jiraStringSet(strings.Split(os.Getenv("JIRA_CONNECTOR_LABELS_TO_SKIP"), ","))
	}
	c.doJSON = c.doJiraJSON
	c.download = c.downloadURL
	return c, nil
}

// Validate validates Jira settings and credentials.
func (c *JiraConnector) Validate(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "Jira connector is nil"}
	}
	if c.baseURL == "" {
		return &ConnectorValidationError{Message: "Jira base URL must be provided."}
	}
	parsed, err := url.Parse(c.baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return &ConnectorValidationError{Message: "invalid Jira base URL"}
	}
	if c.apiToken == "" && (c.userEmail == "" || c.password == "") {
		return &ConnectorMissingCredentialError{Message: "Jira credentials must include either an API token or username/password."}
	}
	if c.projectKey == "" && c.jqlQuery == "" {
		return &ConnectorValidationError{Message: "Either project_key or jql_query must be provided for Jira connector."}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}
	if !c.timezoneOffsetSet {
		_ = c.syncTimezoneFromServer(ctx)
	}
	if c.jqlQuery != "" {
		var page jiraSearchPage
		err = c.searchJQL(ctx, c.buildJQL(nil, time.Now().UTC()), 0, 1, jiraSlimFields, "", &page)
	} else {
		var project map[string]any
		_, err = c.doJSON(ctx, http.MethodGet, "project/"+url.PathEscape(c.projectKey), nil, nil, &project)
	}
	if err != nil {
		return classifyJiraError(err)
	}
	return nil
}

// ValidateConnectorSetting validates Jira settings from an unsaved config.
func (c *JiraConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Jira sync session.
func (c *JiraConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	end := request.WindowEnd
	if end.IsZero() {
		end = time.Now().UTC()
	}
	session := &jiraSyncSession{
		connector:     c,
		batchSize:     c.fullPageSize(),
		windowStart:   request.WindowStart,
		windowEnd:     end,
		fromBeginning: request.FromBeginning,
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Jira prune snapshot session.
func (c *JiraConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	return &jiraPruneSession{connector: c, batchSize: jiraSlimPageSize}, nil
}

func (c *JiraConnector) fullPageSize() int {
	if c.batchSize <= 0 {
		return jiraMaxFullPageSize
	}
	return min(c.batchSize, jiraMaxFullPageSize)
}

func (c *JiraConnector) isCloud() bool {
	return strings.TrimSpace(c.restAPIVersion) == jiraCloudAPIVersion
}

func (c *JiraConnector) buildJQL(start *time.Time, end time.Time) string {
	clauses := []string{}
	orderBy := ""
	if c.jqlQuery != "" {
		jqlQuery, ordering := splitJiraOrderBy(c.jqlQuery)
		orderBy = ordering
		if jqlQuery != "" {
			clauses = append(clauses, "("+jqlQuery+")")
		}
	} else if c.projectKey != "" {
		clauses = append(clauses, fmt.Sprintf("project = %q", c.projectKey))
	}
	if len(c.labelsToSkip) > 0 {
		labels := make([]string, 0, len(c.labelsToSkip))
		for label := range c.labelsToSkip {
			labels = append(labels, fmt.Sprintf("%q", label))
		}
		sort.Strings(labels)
		clauses = append(clauses, "labels NOT IN ("+strings.Join(labels, ", ")+")")
	}
	if start != nil {
		adjusted := *start
		if c.timeBuffer > 0 {
			adjusted = adjusted.Add(-c.timeBuffer)
			if adjusted.Before(time.Unix(0, 0)) {
				adjusted = time.Unix(0, 0).UTC()
			}
		}
		clauses = append(clauses, fmt.Sprintf("updated >= %q", c.formatJQLTime(adjusted)))
	}
	if !end.IsZero() {
		clauses = append(clauses, fmt.Sprintf("updated <= %q", c.formatJQLTime(end)))
	}
	jql := strings.Join(clauses, " AND ")
	if orderBy == "" {
		orderBy = "ORDER BY updated ASC"
	}
	if jql == "" {
		return orderBy
	}
	return jql + " " + orderBy
}

func (c *JiraConnector) formatJQLTime(t time.Time) string {
	offsetSeconds := int(c.timezoneOffset * 3600)
	return t.UTC().In(time.FixedZone("jira", offsetSeconds)).Format("2006-01-02 15:04")
}

func splitJiraOrderBy(query string) (string, string) {
	cleaned := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), ";"))
	orderStart := trailingJiraOrderByIndex(cleaned)
	if orderStart < 0 {
		return cleaned, ""
	}
	return strings.TrimSpace(cleaned[:orderStart]), strings.TrimSpace(cleaned[orderStart:])
}

func trailingJiraOrderByIndex(query string) int {
	depth := 0
	inQuote := rune(0)
	escaped := false
	for i, r := range query {
		if inQuote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			inQuote = r
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && hasJiraOrderByAt(query, i) {
				return i
			}
		}
	}
	return -1
}

func hasJiraOrderByAt(query string, index int) bool {
	if index > 0 && isJiraWordByte(query[index-1]) {
		return false
	}
	if !strings.EqualFold(query[index:min(index+5, len(query))], "order") {
		return false
	}
	pos := index + 5
	if pos >= len(query) || !isJiraWhitespace(query[pos]) {
		return false
	}
	for pos < len(query) && isJiraWhitespace(query[pos]) {
		pos++
	}
	if pos+2 > len(query) || !strings.EqualFold(query[pos:pos+2], "by") {
		return false
	}
	after := pos + 2
	return after == len(query) || !isJiraWordByte(query[after])
}

func isJiraWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isJiraWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func (c *JiraConnector) searchJQL(ctx context.Context, jql string, startAt, maxResults int, fields, nextPageToken string, out *jiraSearchPage) error {
	if c.isCloud() {
		query := url.Values{"jql": {jql}, "maxResults": {strconv.Itoa(maxResults)}, "fields": {"id"}}
		if nextPageToken != "" {
			query.Set("nextPageToken", nextPageToken)
		}
		var ids jiraSearchPage
		if _, err := c.doJSON(ctx, http.MethodGet, "search/jql", query, nil, &ids); err != nil {
			return err
		}
		out.NextPageToken = ids.NextPageToken
		issueIDs := make([]string, 0, len(ids.Issues))
		for _, issue := range ids.Issues {
			if issue.ID != "" {
				issueIDs = append(issueIDs, issue.ID)
			}
		}
		if len(issueIDs) == 0 {
			return nil
		}
		payload := map[string]any{"issueIdsOrKeys": issueIDs, "fields": strings.Split(fields, ",")}
		_, err := c.doJSON(ctx, http.MethodPost, "issue/bulkfetch", nil, payload, out)
		return err
	}
	query := url.Values{
		"jql":        {jql},
		"startAt":    {strconv.Itoa(startAt)},
		"maxResults": {strconv.Itoa(maxResults)},
		"fields":     {fields},
		"expand":     {"renderedFields"},
	}
	_, err := c.doJSON(ctx, http.MethodGet, "search", query, nil, out)
	return err
}

func (c *JiraConnector) issueDocument(issue jiraIssue) (SourceDocument, bool) {
	if issue.shouldSkip(c.labelsToSkip) {
		return SourceDocument{}, false
	}
	updatedAt := issue.Fields.Updated.Time()
	if updatedAt.IsZero() {
		updatedAt = issue.Fields.Created.Time()
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	summary := issue.Fields.Summary
	issueURL := c.issueURL(issue.Key)
	description := jiraBodyText(issue.Fields.Description)
	comments := ""
	if c.includeComments {
		comments = jiraFormatComments(issue.Fields.Comment.Comments, c.commentEmailBlacklist)
	}
	attachments := jiraFormatAttachments(issue.Fields.Attachments)
	lines := []string{
		"---",
		"key: " + issue.Key,
		"url: " + issueURL,
		"summary: " + summary,
		"status: " + firstNonEmpty(issue.Fields.Status.Name, "Unknown"),
		"priority: " + firstNonEmpty(issue.Fields.Priority.Name, "Unspecified"),
		"issue_type: " + firstNonEmpty(issue.Fields.IssueType.Name, "Unknown"),
		"project: " + issue.Fields.Project.Name,
		"project_key: " + firstNonEmpty(issue.Fields.Project.Key, c.projectKey),
	}
	if issue.Fields.Reporter.DisplayName != "" {
		lines = append(lines, "reporter: "+issue.Fields.Reporter.DisplayName)
	}
	if issue.Fields.Reporter.EmailAddress != "" {
		lines = append(lines, "reporter_email: "+issue.Fields.Reporter.EmailAddress)
	}
	if issue.Fields.Assignee.DisplayName != "" {
		lines = append(lines, "assignee: "+issue.Fields.Assignee.DisplayName)
	}
	if issue.Fields.Assignee.EmailAddress != "" {
		lines = append(lines, "assignee_email: "+issue.Fields.Assignee.EmailAddress)
	}
	if len(issue.Fields.Labels) > 0 {
		lines = append(lines, "labels: "+strings.Join(issue.Fields.Labels, ", "))
	}
	lines = append(lines, "created: "+issue.Fields.Created.Time().Format(time.RFC3339), "updated: "+updatedAt.Format(time.RFC3339), "---", "", "## Description", firstNonEmpty(description, "No description provided."))
	if comments != "" {
		lines = append(lines, "", "## Comments", comments)
	}
	if attachments != "" {
		lines = append(lines, "", "## Attachments", attachments)
	}
	blob := []byte(strings.TrimSpace(strings.Join(lines, "\n")) + "\n")
	if len(blob) > c.maxTicketSize {
		return SourceDocument{}, false
	}
	return SourceDocument{
		SourceID:           issueURL,
		SemanticIdentifier: firstNonEmpty(strings.TrimSpace(issue.Key+": "+summary), issue.Key),
		Extension:          ".md",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Fingerprint:        contentFingerprint(blob),
	}, true
}

func (c *JiraConnector) attachmentDocument(ctx context.Context, issue jiraIssue, attachment jiraAttachment) (SourceDocument, bool, error) {
	if !c.includeAttachments || attachment.Filename == "" || attachment.Content == "" {
		return SourceDocument{}, false, nil
	}
	if attachment.Size > 0 && attachment.Size > c.attachmentSizeLimit {
		return SourceDocument{}, false, nil
	}
	blob, err := c.download(ctx, attachment.Content)
	if err != nil {
		return SourceDocument{}, false, err
	}
	if int64(len(blob)) > c.attachmentSizeLimit {
		return SourceDocument{}, false, nil
	}
	updatedAt := attachment.Created.Time()
	if updatedAt.IsZero() {
		updatedAt = issue.Fields.Updated.Time()
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	sourceID := issue.Key + "::attachment::" + firstNonEmpty(attachment.ID, attachment.Filename)
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: issue.Key + " attachment: " + attachment.Filename,
		Extension:          filepath.Ext(attachment.Filename),
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Fingerprint:        contentFingerprint(blob),
	}, true, nil
}

func (c *JiraConnector) issueURL(key string) string {
	return strings.TrimRight(c.baseURL, "/") + "/browse/" + key
}

func (c *JiraConnector) syncTimezoneFromServer(ctx context.Context) error {
	var info struct {
		ServerTime string `json:"serverTime"`
		TimeZone   string `json:"timeZone"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "serverInfo", nil, nil, &info); err != nil {
		return err
	}
	if offset, ok := jiraParseOffset(info.ServerTime); ok {
		c.timezoneOffset = offset
	}
	return nil
}

func (c *JiraConnector) doJiraJSON(ctx context.Context, method, apiPath string, query url.Values, body any, out any) (http.Header, error) {
	apiURL := c.apiURL(apiPath, query)
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL, reader)
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return resp.Header, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Header, &jiraAPIError{Status: resp.StatusCode, Message: jiraErrorMessage(data)}
	}
	if out == nil || len(data) == 0 {
		return resp.Header, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return resp.Header, err
	}
	return resp.Header, nil
}

func (c *JiraConnector) downloadURL(ctx context.Context, rawURL string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("unsupported Jira attachment URL")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid Jira base URL")
	}
	if !sameJiraOrigin(parsed, base) {
		return nil, fmt.Errorf("Jira attachment origin %q does not match the configured base URL", parsed.Scheme+"://"+parsed.Host)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &jiraAPIError{Status: resp.StatusCode, Message: resp.Status}
	}
	return io.ReadAll(io.LimitReader(resp.Body, c.attachmentSizeLimit+1))
}

func (c *JiraConnector) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return fmt.Errorf("invalid Jira base URL")
	}
	if !sameJiraOrigin(req.URL, base) {
		return fmt.Errorf("Jira redirect origin %q does not match the configured base URL", req.URL.Scheme+"://"+req.URL.Host)
	}
	c.authorize(req)
	return nil
}

func sameJiraOrigin(candidate, base *url.URL) bool {
	return strings.EqualFold(candidate.Scheme, base.Scheme) && strings.EqualFold(candidate.Host, base.Host)
}

func (c *JiraConnector) apiURL(apiPath string, query url.Values) string {
	base := c.baseURL
	if c.scopedToken && strings.Contains(base, ".atlassian.net") {
		base = strings.TrimRight(base, "/") + "/ex/jira"
	}
	u := strings.TrimRight(base, "/") + "/rest/api/" + c.restAPIVersion + "/" + strings.TrimLeft(apiPath, "/")
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c *JiraConnector) authorize(req *http.Request) {
	switch {
	case c.userEmail != "" && c.apiToken != "":
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.userEmail+":"+c.apiToken)))
	case c.apiToken != "":
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	case c.userEmail != "" && c.password != "":
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.userEmail+":"+c.password)))
	}
}

type jiraSyncSession struct {
	connector     *JiraConnector
	batchSize     int
	windowStart   *time.Time
	windowEnd     time.Time
	fromBeginning bool
	startAt       int
	nextPageToken string
	done          bool
	resumeSource  string
	resumeSkip    bool
	resumeChecked bool
}

func (s *jiraSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.done {
		return SyncBatch{}, io.EOF
	}
	if err := s.validateResumeSource(ctx); err != nil {
		return SyncBatch{}, err
	}
	documents := make([]SourceDocument, 0, s.batchSize)
	var checkpoint *SyncCheckpoint
	for len(documents) < s.batchSize && !s.done {
		page, err := s.nextIssuePage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		for _, issue := range page.Issues {
			if s.resumeSkip && s.resumeSource != "" {
				if s.connector.issueURL(issue.Key) == s.resumeSource {
					s.resumeSource = ""
				}
				continue
			}
			updatedAt := issue.Fields.Updated.Time()
			if s.windowStart != nil && !s.fromBeginning && !updatedAt.IsZero() && !updatedAt.After(*s.windowStart) {
				continue
			}
			doc, ok := s.connector.issueDocument(issue)
			if ok {
				documents = append(documents, doc)
				checkpoint = jiraCheckpoint(s.startAt, s.nextPageToken, doc)
			}
			if s.connector.includeAttachments {
				for _, attachment := range issue.Fields.Attachments {
					attachmentDoc, ok, err := s.connector.attachmentDocument(ctx, issue, attachment)
					if err != nil {
						return SyncBatch{}, err
					}
					if ok {
						documents = append(documents, attachmentDoc)
						checkpoint = jiraCheckpoint(s.startAt, s.nextPageToken, attachmentDoc)
					}
				}
			}
		}
	}
	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

func (s *jiraSyncSession) validateResumeSource(ctx context.Context) error {
	if s.resumeSource == "" || s.resumeChecked {
		return nil
	}
	s.resumeChecked = true
	prefix := strings.TrimRight(s.connector.baseURL, "/") + "/browse/"
	key := strings.TrimPrefix(s.resumeSource, prefix)
	if key == "" || key == s.resumeSource {
		return fmt.Errorf("jira sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	var page jiraSearchPage
	if err := s.connector.searchJQL(ctx, fmt.Sprintf(`key = "%s"`, strings.ReplaceAll(key, `"`, `\"`)), 0, 1, jiraSlimFields, "", &page); err != nil {
		return err
	}
	for _, issue := range page.Issues {
		if s.connector.issueURL(issue.Key) == s.resumeSource {
			return nil
		}
	}
	return fmt.Errorf("jira resume anchor %q was not found in the current listing: %w", s.resumeSource, ErrSyncResumeInvalid)
}

func (s *jiraSyncSession) Close() error { return nil }

func (s *jiraSyncSession) nextIssuePage(ctx context.Context) (jiraSearchPage, error) {
	jql := s.connector.buildJQL(jiraWindowStart(s.windowStart, s.fromBeginning), s.windowEnd)
	var page jiraSearchPage
	if err := s.connector.searchJQL(ctx, jql, s.startAt, s.batchSize, jiraDefaultFields, s.nextPageToken, &page); err != nil {
		return jiraSearchPage{}, err
	}
	if s.connector.isCloud() {
		s.nextPageToken = page.NextPageToken
		s.done = page.NextPageToken == "" || len(page.Issues) == 0
	} else {
		s.startAt += len(page.Issues)
		s.done = len(page.Issues) < s.batchSize
	}
	return page, nil
}

func (s *jiraSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("jira sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor jiraSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("jira sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	prefix := strings.TrimRight(s.connector.baseURL, "/") + "/browse/"
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("jira sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	s.resumeSource = sourceID
	s.startAt = cursor.StartAt
	s.nextPageToken = cursor.NextPageToken
	s.resumeSkip = s.startAt == 0 && s.nextPageToken == ""
	return nil
}

type jiraPruneSession struct {
	connector     *JiraConnector
	batchSize     int
	startAt       int
	nextPageToken string
	done          bool
}

func (s *jiraPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.done {
		return PruneBatch{}, io.EOF
	}
	documents := make([]SlimDocument, 0, s.batchSize)
	for len(documents) < s.batchSize && !s.done {
		var page jiraSearchPage
		jql := s.connector.buildJQL(nil, time.Now().UTC())
		if err := s.connector.searchJQL(ctx, jql, s.startAt, s.batchSize, jiraSlimFields, s.nextPageToken, &page); err != nil {
			return PruneBatch{}, err
		}
		for _, issue := range page.Issues {
			if !issue.shouldSkip(s.connector.labelsToSkip) {
				documents = append(documents, SlimDocument{SourceID: s.connector.issueURL(issue.Key)})
			}
		}
		if s.connector.isCloud() {
			s.nextPageToken = page.NextPageToken
			s.done = page.NextPageToken == "" || len(page.Issues) == 0
		} else {
			s.startAt += len(page.Issues)
			s.done = len(page.Issues) < s.batchSize
		}
	}
	if len(documents) == 0 {
		return PruneBatch{}, io.EOF
	}
	return PruneBatch{Documents: documents}, nil
}

func (s *jiraPruneSession) Close() error { return nil }

type jiraSyncCursor struct {
	StartAt       int    `json:"start_at"`
	NextPageToken string `json:"next_page_token,omitempty"`
	SourceID      string `json:"source_id"`
}

func jiraCheckpoint(startAt int, nextPageToken string, doc SourceDocument) *SyncCheckpoint {
	cursor, err := json.Marshal(jiraSyncCursor{StartAt: startAt, NextPageToken: nextPageToken, SourceID: doc.SourceID})
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{Cursor: string(cursor), SourceID: doc.SourceID, UpdatedAt: &updatedAt}
}

type jiraSearchPage struct {
	Issues        []jiraIssue `json:"issues"`
	NextPageToken string      `json:"nextPageToken"`
}

type jiraIssue struct {
	ID     string     `json:"id"`
	Key    string     `json:"key"`
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Summary     string           `json:"summary"`
	Description any              `json:"description"`
	Updated     jiraTime         `json:"updated"`
	Created     jiraTime         `json:"created"`
	Status      jiraNamed        `json:"status"`
	Priority    jiraNamed        `json:"priority"`
	Assignee    jiraUser         `json:"assignee"`
	Reporter    jiraUser         `json:"reporter"`
	Labels      []string         `json:"labels"`
	IssueType   jiraNamed        `json:"issuetype"`
	Project     jiraNamed        `json:"project"`
	Comment     jiraCommentBlock `json:"comment"`
	Attachments []jiraAttachment `json:"attachment"`
}

type jiraNamed struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type jiraUser struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	Name         string `json:"name"`
}

type jiraCommentBlock struct {
	Comments []jiraComment `json:"comments"`
}

type jiraComment struct {
	Author  jiraUser `json:"author"`
	Created jiraTime `json:"created"`
	Body    any      `json:"body"`
}

type jiraAttachment struct {
	ID       string   `json:"id"`
	Filename string   `json:"filename"`
	Content  string   `json:"content"`
	Size     int64    `json:"size"`
	Created  jiraTime `json:"created"`
}

type jiraTime struct {
	raw string
}

func (t *jiraTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, &t.raw)
}

func (t jiraTime) Time() time.Time {
	value := strings.TrimSpace(t.raw)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05.000Z0700", "2006-01-02T15:04:05-0700"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func (i jiraIssue) shouldSkip(labelsToSkip map[string]struct{}) bool {
	if len(labelsToSkip) == 0 {
		return false
	}
	for _, label := range i.Fields.Labels {
		if _, ok := labelsToSkip[strings.ToLower(label)]; ok {
			return true
		}
	}
	return false
}

type jiraAPIError struct {
	Status  int
	Message string
}

func (e *jiraAPIError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("Jira API returned HTTP %d: %s", e.Status, e.Message)
}

func classifyJiraError(err error) error {
	var apiErr *jiraAPIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Status {
	case http.StatusUnauthorized:
		return &ConnectorMissingCredentialError{Message: "Jira credential appears to be invalid or expired (HTTP 401)."}
	case http.StatusForbidden:
		return &ConnectorValidationError{Message: "Jira token does not have permission to access the requested resources (HTTP 403)."}
	case http.StatusNotFound:
		return &ConnectorValidationError{Message: "Jira resource not found (HTTP 404)."}
	case http.StatusTooManyRequests:
		return &ConnectorValidationError{Message: "Jira rate limit exceeded during validation (HTTP 429)."}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("Unexpected Jira error (status=%d): %s", apiErr.Status, apiErr.Message)}
	}
}

func jiraWindowStart(start *time.Time, fromBeginning bool) *time.Time {
	if fromBeginning {
		return nil
	}
	return start
}

func jiraBodyText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		var parts []string
		jiraWalkADF(typed, &parts)
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func jiraWalkADF(value any, parts *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "text" {
			if text := stringConfig(typed["text"]); text != "" {
				*parts = append(*parts, text)
			}
		}
		if children, ok := typed["content"].([]any); ok {
			for _, child := range children {
				jiraWalkADF(child, parts)
			}
		}
	case []any:
		for _, child := range typed {
			jiraWalkADF(child, parts)
		}
	}
}

func jiraFormatComments(comments []jiraComment, blacklist map[string]struct{}) string {
	lines := make([]string, 0, len(comments))
	for _, comment := range comments {
		email := strings.ToLower(comment.Author.EmailAddress)
		if email != "" {
			if _, ok := blacklist[email]; ok {
				continue
			}
		}
		body := jiraBodyText(comment.Body)
		if body == "" {
			continue
		}
		author := firstNonEmpty(comment.Author.DisplayName, comment.Author.Name, comment.Author.EmailAddress, "Unknown")
		created := "Unknown time"
		if t := comment.Created.Time(); !t.IsZero() {
			created = t.Format(time.RFC3339)
		}
		lines = append(lines, fmt.Sprintf("- %s (%s):\n%s", author, created, body))
	}
	return strings.Join(lines, "\n\n")
}

func jiraFormatAttachments(attachments []jiraAttachment) string {
	lines := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Filename == "" {
			continue
		}
		size := ""
		if attachment.Size > 0 {
			size = fmt.Sprintf(" (%d bytes)", attachment.Size)
		}
		suffix := ""
		if attachment.Content != "" {
			suffix = " -> " + attachment.Content
		}
		lines = append(lines, "- "+attachment.Filename+size+suffix)
	}
	return strings.Join(lines, "\n")
}

func jiraStringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func jiraStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(stringConfig(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func jiraFloatConfig(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func jiraEnvInt(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func jiraParseOffset(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	layouts := []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05.000Z0700", "2006-01-02T15:04:05-0700"}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		_, offset := parsed.Zone()
		return float64(offset) / 3600, true
	}
	return 0, false
}

func jiraErrorMessage(data []byte) string {
	var payload struct {
		ErrorMessages []string       `json:"errorMessages"`
		Errors        map[string]any `json:"errors"`
		Message       string         `json:"message"`
	}
	if json.Unmarshal(data, &payload) == nil {
		if len(payload.ErrorMessages) > 0 {
			return strings.Join(payload.ErrorMessages, "; ")
		}
		if payload.Message != "" {
			return payload.Message
		}
		if len(payload.Errors) > 0 {
			parts := make([]string, 0, len(payload.Errors))
			for key, value := range payload.Errors {
				parts = append(parts, key+": "+fmt.Sprint(value))
			}
			sort.Strings(parts)
			return strings.Join(parts, "; ")
		}
	}
	if mediaType, _, err := mime.ParseMediaType(http.DetectContentType(data)); err == nil && strings.HasPrefix(mediaType, "text/") {
		return string(data)
	}
	return strings.TrimSpace(string(data))
}
