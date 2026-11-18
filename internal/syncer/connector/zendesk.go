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
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultZendeskBatchSize = 30
	zendeskMaxPageSize      = 30
	zendeskPrunePageSize    = 1000
	zendeskRequestTimeout   = 60 * time.Second
	zendeskAPIBaseTemplate  = "https://%s.zendesk.com/api/v2"

	zendeskContentTypeArticles = "articles"
	zendeskContentTypeTickets  = "tickets"
)

var zendeskSubdomainRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// ZendeskConnector reads Zendesk Help Center articles or Support tickets.
type ZendeskConnector struct {
	contentType       string
	subdomain         string
	email             string
	token             string
	batchSize         int
	skipArticleLabels map[string]struct{}
	baseURL           string
	httpClient        *http.Client

	doJSON func(ctx context.Context, endpoint string, query url.Values, out any) error
}

// NewZendeskConnector creates a Zendesk connector from Python-compatible config.
func NewZendeskConnector(config map[string]any) (*ZendeskConnector, error) {
	credentials := configAnyMap(config["credentials"])
	contentType := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringConfig(config["zendesk_content_type"]), zendeskContentTypeArticles)))
	if contentType != zendeskContentTypeArticles && contentType != zendeskContentTypeTickets {
		return nil, &ConnectorValidationError{Message: "zendesk_content_type must be 'articles' or 'tickets'"}
	}
	subdomain, err := normalizeZendeskSubdomain(stringConfig(credentials["zendesk_subdomain"]))
	if err != nil {
		return nil, err
	}
	connector := &ZendeskConnector{
		contentType:       contentType,
		subdomain:         subdomain,
		email:             strings.TrimSpace(stringConfig(credentials["zendesk_email"])),
		token:             strings.TrimSpace(stringConfig(credentials["zendesk_token"])),
		batchSize:         zendeskBatchSize(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"]))),
		skipArticleLabels: zendeskSkipArticleLabels(),
		baseURL:           fmt.Sprintf(zendeskAPIBaseTemplate, subdomain),
		httpClient:        &http.Client{Timeout: zendeskRequestTimeout},
	}
	connector.doJSON = connector.doZendeskJSON
	return connector, nil
}

// Validate validates Zendesk settings and credentials against the configured API.
func (c *ZendeskConnector) Validate(ctx context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}
	if c.contentType == zendeskContentTypeTickets {
		if _, err := c.listTicketsPage(ctx, 0); err != nil {
			return classifyZendeskError(err)
		}
		return nil
	}
	query := url.Values{
		"page[size]": {"1"},
		"sort_by":    {"updated_at"},
		"sort_order": {"asc"},
		"start_time": {"0"},
	}
	var page zendeskArticlePage
	if err := c.doJSON(ctx, "help_center/articles", query, &page); err != nil {
		return classifyZendeskError(err)
	}
	return nil
}

// ValidateConnectorSetting validates an unsaved Zendesk config.
func (c *ZendeskConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	if c == nil {
		return &ConnectorValidationError{Message: "Zendesk connector is nil"}
	}
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	connector, err := NewZendeskConnector(request)
	if err != nil {
		return err
	}
	connector.httpClient = c.httpClient
	return connector.Validate(ctx)
}

// OpenSync opens one Zendesk sync session without listing the source up front.
func (c *ZendeskConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateConfig(); err != nil {
		return nil, err
	}
	return &zendeskSyncSession{
		connector:       c,
		request:         request,
		batchSize:       c.batchSize,
		startTime:       zendeskRequestStart(request),
		ticketStartTime: zendeskRequestStart(request),
		articleHasMore:  true,
		ticketHasMore:   true,
	}, nil
}

// OpenPrune opens one complete Zendesk prune snapshot session.
func (c *ZendeskConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateConfig(); err != nil {
		return nil, err
	}
	return &zendeskPruneSession{connector: c, articleHasMore: true, ticketHasMore: true}, nil
}

func (c *ZendeskConnector) validateConfig() error {
	if c == nil {
		return &ConnectorValidationError{Message: "Zendesk connector is nil"}
	}
	if _, err := normalizeZendeskSubdomain(c.subdomain); err != nil {
		return err
	}
	if c.email == "" || c.token == "" {
		return &ConnectorMissingCredentialError{Message: "Zendesk credentials must include zendesk_email and zendesk_token"}
	}
	if c.contentType != zendeskContentTypeArticles && c.contentType != zendeskContentTypeTickets {
		return &ConnectorValidationError{Message: "zendesk_content_type must be 'articles' or 'tickets'"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}
	return nil
}

func (c *ZendeskConnector) listArticlesPage(ctx context.Context, startTime int64, afterCursor string, pageSize int) (zendeskArticlePage, error) {
	if pageSize <= 0 {
		pageSize = zendeskMaxPageSize
	}
	query := url.Values{
		"page[size]": {strconv.Itoa(pageSize)},
		"sort_by":    {"updated_at"},
		"sort_order": {"asc"},
	}
	if startTime > 0 {
		query.Set("start_time", strconv.FormatInt(startTime, 10))
	}
	if afterCursor != "" {
		query.Set("page[after]", afterCursor)
	}
	var page zendeskArticlePage
	err := c.doJSON(ctx, "help_center/articles", query, &page)
	return page, err
}

func (c *ZendeskConnector) listTicketsPage(ctx context.Context, startTime int64) (zendeskTicketPage, error) {
	query := url.Values{"start_time": {strconv.FormatInt(startTime, 10)}}
	var page zendeskTicketPage
	err := c.doJSON(ctx, "incremental/tickets.json", query, &page)
	return page, err
}

func (c *ZendeskConnector) loadContentTags(ctx context.Context) (map[string]string, error) {
	tags := map[string]string{}
	after := ""
	for {
		query := url.Values{"page[size]": {strconv.Itoa(zendeskMaxPageSize)}}
		if after != "" {
			query.Set("page[after]", after)
		}
		var page zendeskContentTagPage
		if err := c.doJSON(ctx, "guide/content_tags", query, &page); err != nil {
			return nil, err
		}
		for _, tag := range page.Records {
			if id := tag.ID.String(); id != "" && tag.Name != "" {
				tags[id] = tag.Name
			}
		}
		if !page.Meta.HasMore {
			return tags, nil
		}
		if page.Meta.AfterCursor == "" || page.Meta.AfterCursor == after {
			return nil, fmt.Errorf("Zendesk content tags pagination did not advance")
		}
		after = page.Meta.AfterCursor
	}
}

func (c *ZendeskConnector) fetchAuthor(ctx context.Context, authorID json.Number) (zendeskUser, bool) {
	if authorID.String() == "" || authorID.String() == "-1" {
		return zendeskUser{}, false
	}
	var page zendeskUserResponse
	if err := c.doJSON(ctx, "users/"+authorID.String(), nil, &page); err != nil {
		return zendeskUser{}, false
	}
	return page.User, page.User.Name != "" && page.User.Email != ""
}

func (c *ZendeskConnector) isIndexableArticle(article zendeskArticle) bool {
	if strings.TrimSpace(article.Body) == "" || article.Draft {
		return false
	}
	for _, label := range article.LabelNames {
		if _, ok := c.skipArticleLabels[strings.ToLower(label)]; ok {
			return false
		}
	}
	return true
}

func (c *ZendeskConnector) doZendeskJSON(ctx context.Context, endpoint string, query url.Values, out any) error {
	requestCtx, cancel := context.WithTimeout(ctx, zendeskRequestTimeout)
	defer cancel()
	endpointURL := c.baseURL + "/" + strings.TrimLeft(endpoint, "/")
	if len(query) > 0 {
		endpointURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.email+"/token", c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &zendeskAPIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode Zendesk response: %w", err)
	}
	return nil
}

func normalizeZendeskSubdomain(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "https://")
	if idx := strings.IndexByte(value, '/'); idx >= 0 {
		value = value[:idx]
	}
	value = strings.TrimSuffix(value, "/")
	if strings.HasSuffix(value, ".zendesk.com") {
		value = strings.TrimSuffix(value, ".zendesk.com")
	}
	value = strings.ToLower(value)
	if !zendeskSubdomainRE.MatchString(value) {
		return "", &ConnectorValidationError{Message: fmt.Sprintf("invalid Zendesk subdomain %q", raw)}
	}
	return value, nil
}

func zendeskBatchSize(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return defaultZendeskBatchSize
}

func zendeskSkipArticleLabels() map[string]struct{} {
	labels := map[string]struct{}{}
	for _, label := range splitCommaList(os.Getenv("ZENDESK_CONNECTOR_SKIP_ARTICLE_LABELS")) {
		labels[strings.ToLower(label)] = struct{}{}
	}
	return labels
}

func zendeskRequestStart(request SyncRequest) int64 {
	if request.FromBeginning {
		return 0
	}
	if request.WindowStart != nil {
		return request.WindowStart.UTC().Unix()
	}
	return 0
}

func includeZendeskDocument(request SyncRequest, sourceID string, updatedAt time.Time, fingerprint string) bool {
	if request.FromBeginning {
		return true
	}
	if len(request.Fingerprints) > 0 {
		if fingerprint == "" {
			return true
		}
		stored, ok := request.Fingerprints[sourceID]
		return !ok || stored == "" || stored != fingerprint
	}
	if updatedAt.IsZero() {
		return true
	}
	return !beforeOrAtWindowStart(updatedAt, request.WindowStart) && !afterWindowEnd(updatedAt, request.WindowEnd)
}

func parseZendeskTime(value string) time.Time {
	return parseOutlookTime(value)
}

type zendeskSyncSession struct {
	connector *ZendeskConnector
	request   SyncRequest
	batchSize int
	done      bool

	startTime int64

	articleAfterCursor     string
	nextArticleAfterCursor string
	articleBuffer          []zendeskArticle
	articleIndex           int

	ticketStartTime     int64
	ticketPageStartTime int64
	ticketBuffer        []zendeskTicket
	ticketIndex         int

	articleHasMore bool
	ticketHasMore  bool

	contentTags       map[string]string
	contentTagsLoaded bool

	lastDocAfterCursor string
	lastDocStartTime   int64

	resumeSource  string
	resumeSkip    bool
	resumeChecked bool
}

func (s *zendeskSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.done {
		return SyncBatch{}, io.EOF
	}
	if err := s.validateResume(ctx); err != nil {
		return SyncBatch{}, err
	}
	documents := make([]SourceDocument, 0, s.batchSize)
	var last SourceDocument
	for len(documents) < s.batchSize && !s.done {
		var doc SourceDocument
		var ok bool
		var err error
		if s.connector.contentType == zendeskContentTypeArticles {
			doc, ok, err = s.nextArticle(ctx)
		} else {
			doc, ok, err = s.nextTicket(ctx)
		}
		if err != nil {
			return SyncBatch{}, err
		}
		if !ok {
			break
		}
		documents = append(documents, doc)
		last = doc
	}
	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	return SyncBatch{Documents: documents, Checkpoint: s.checkpoint(last)}, nil
}

func (s *zendeskSyncSession) Close() error { return nil }

func (s *zendeskSyncSession) validateResume(ctx context.Context) error {
	if s.resumeChecked || s.request.Resume == nil {
		return nil
	}
	s.resumeChecked = true
	checkpoint := s.request.Resume
	if checkpoint.Cursor == "" {
		return fmt.Errorf("zendesk sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor zendeskSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("zendesk sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	expectedPrefix := "article:"
	if s.connector.contentType == zendeskContentTypeTickets {
		expectedPrefix = "zendesk_ticket_"
	}
	if sourceID == "" || !strings.HasPrefix(sourceID, expectedPrefix) {
		return fmt.Errorf("zendesk sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.ContentType != "" && cursor.ContentType != s.connector.contentType {
		return fmt.Errorf("zendesk sync checkpoint belongs to a different content type: %w", ErrSyncResumeInvalid)
	}
	s.resumeSource = sourceID
	s.resumeSkip = true
	if s.connector.contentType == zendeskContentTypeArticles {
		s.startTime = cursor.StartTime
		s.nextArticleAfterCursor = cursor.AfterCursor
	} else {
		s.ticketStartTime = cursor.StartTime
	}
	return nil
}

func (s *zendeskSyncSession) nextArticle(ctx context.Context) (SourceDocument, bool, error) {
	for {
		if err := s.fillArticleBuffer(ctx); err != nil {
			return SourceDocument{}, false, err
		}
		if s.resumeSkip && s.resumeSource != "" && s.articleIndex >= len(s.articleBuffer) {
			return SourceDocument{}, false, fmt.Errorf("zendesk resume anchor %q was not found in the current article page: %w", s.resumeSource, ErrSyncResumeInvalid)
		}
		if s.articleIndex >= len(s.articleBuffer) {
			if !s.articleHasMore {
				s.done = true
				return SourceDocument{}, false, nil
			}
			continue
		}
		article := s.articleBuffer[s.articleIndex]
		s.articleIndex++
		sourceID := zendeskArticleSourceID(article)
		if s.resumeSkip && s.resumeSource != "" {
			if sourceID == s.resumeSource {
				s.resumeSource = ""
			}
			continue
		}
		if !s.connector.isIndexableArticle(article) {
			continue
		}
		doc, ok, err := s.articleDocument(ctx, article)
		if err != nil {
			return SourceDocument{}, false, err
		}
		if !ok || !includeZendeskDocument(s.request, doc.SourceID, doc.UpdatedAt, doc.Fingerprint) {
			continue
		}
		s.lastDocAfterCursor = s.articleAfterCursor
		return doc, true, nil
	}
}

func (s *zendeskSyncSession) fillArticleBuffer(ctx context.Context) error {
	for s.articleIndex >= len(s.articleBuffer) {
		if !s.articleHasMore {
			return nil
		}
		if s.done {
			return nil
		}
		after := s.nextArticleAfterCursor
		page, err := s.connector.listArticlesPage(ctx, s.startTime, after, zendeskMaxPageSize)
		if err != nil {
			return err
		}
		if page.Meta.HasMore && (page.Meta.AfterCursor == "" || page.Meta.AfterCursor == after) {
			return fmt.Errorf("Zendesk articles pagination did not advance")
		}
		s.articleAfterCursor = after
		s.nextArticleAfterCursor = page.Meta.AfterCursor
		s.articleBuffer = page.Articles
		s.articleIndex = 0
		if !page.Meta.HasMore {
			s.articleHasMore = false
		}
	}
	return nil
}

func (s *zendeskSyncSession) articleDocument(ctx context.Context, article zendeskArticle) (SourceDocument, bool, error) {
	body := htmlToText(article.Body)
	if strings.TrimSpace(body) == "" {
		return SourceDocument{}, false, nil
	}
	tagNames, err := s.contentTagNames(ctx, article.ContentTagIDs)
	if err != nil {
		return SourceDocument{}, false, err
	}
	blob := []byte(body)
	metadata := map[string]any{}
	if len(article.LabelNames) > 0 {
		metadata["labels"] = article.LabelNames
	}
	if len(tagNames) > 0 {
		metadata["content_tags"] = tagNames
	}
	if author, ok := s.connector.fetchAuthor(ctx, article.AuthorID); ok {
		metadata["primary_owners"] = []map[string]string{zendeskOwnerMetadata(author)}
	}
	doc := SourceDocument{
		SourceID:           zendeskArticleSourceID(article),
		SemanticIdentifier: article.Title,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          parseZendeskTime(article.UpdatedAt),
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        contentFingerprint(blob),
	}
	return doc, true, nil
}

func (s *zendeskSyncSession) contentTagNames(ctx context.Context, ids []json.Number) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if !s.contentTagsLoaded {
		tags, err := s.connector.loadContentTags(ctx)
		if err != nil {
			return nil, err
		}
		s.contentTags = tags
		s.contentTagsLoaded = true
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if name := s.contentTags[id.String()]; name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (s *zendeskSyncSession) nextTicket(ctx context.Context) (SourceDocument, bool, error) {
	for {
		if err := s.fillTicketBuffer(ctx); err != nil {
			return SourceDocument{}, false, err
		}
		if s.resumeSkip && s.resumeSource != "" && s.ticketIndex >= len(s.ticketBuffer) {
			return SourceDocument{}, false, fmt.Errorf("zendesk resume anchor %q was not found in the current ticket page: %w", s.resumeSource, ErrSyncResumeInvalid)
		}
		if s.ticketIndex >= len(s.ticketBuffer) {
			if !s.ticketHasMore {
				s.done = true
				return SourceDocument{}, false, nil
			}
			continue
		}
		ticket := s.ticketBuffer[s.ticketIndex]
		s.ticketIndex++
		sourceID := zendeskTicketSourceID(ticket)
		if s.resumeSkip && s.resumeSource != "" {
			if sourceID == s.resumeSource {
				s.resumeSource = ""
			}
			continue
		}
		if !s.connector.isIndexableTicket(ticket) {
			continue
		}
		doc, ok, err := s.ticketDocument(ctx, ticket)
		if err != nil {
			return SourceDocument{}, false, err
		}
		if !ok || !includeZendeskDocument(s.request, doc.SourceID, doc.UpdatedAt, doc.Fingerprint) {
			continue
		}
		s.lastDocStartTime = s.ticketPageStartTime
		return doc, true, nil
	}
}

func (s *zendeskSyncSession) fillTicketBuffer(ctx context.Context) error {
	for s.ticketIndex >= len(s.ticketBuffer) {
		if !s.ticketHasMore {
			return nil
		}
		if s.done {
			return nil
		}
		s.ticketPageStartTime = s.ticketStartTime
		page, err := s.connector.listTicketsPage(ctx, s.ticketStartTime)
		if err != nil {
			return err
		}
		if !page.EndOfStream && page.EndTime <= s.ticketStartTime {
			return fmt.Errorf("Zendesk tickets pagination did not advance")
		}
		s.ticketStartTime = page.EndTime
		s.ticketBuffer = page.Tickets
		s.ticketIndex = 0
		if page.EndOfStream {
			s.ticketHasMore = false
		}
	}
	return nil
}

func (s *zendeskSyncSession) ticketDocument(ctx context.Context, ticket zendeskTicket) (SourceDocument, bool, error) {
	var commentsPage zendeskCommentsPage
	if err := s.connector.doJSON(ctx, "tickets/"+ticket.ID.String()+"/comments", nil, &commentsPage); err != nil {
		return SourceDocument{}, false, err
	}
	commentTexts := make([]string, 0, len(commentsPage.Comments))
	for _, comment := range commentsPage.Comments {
		commentTexts = append(commentTexts, s.commentText(ctx, comment))
	}
	subject := ticket.Subject
	if subject == "" {
		subject = "No Subject"
	}
	body := "Ticket Subject:\n" + subject + "\n\nComments:\n" + strings.Join(commentTexts, "\n\n")
	blob := []byte(body)
	metadata := map[string]any{}
	if ticket.Status != "" {
		metadata["status"] = ticket.Status
	}
	if ticket.Priority != "" {
		metadata["priority"] = ticket.Priority
	}
	if len(ticket.Tags) > 0 {
		metadata["tags"] = ticket.Tags
	}
	if ticket.TicketType != "" {
		metadata["ticket_type"] = ticket.TicketType
	}
	if author, ok := s.connector.fetchAuthor(ctx, ticket.Submitter); ok {
		metadata["primary_owners"] = []map[string]string{zendeskOwnerMetadata(author)}
	}
	doc := SourceDocument{
		SourceID:           zendeskTicketSourceID(ticket),
		SemanticIdentifier: fmt.Sprintf("Ticket #%s: %s", ticket.ID.String(), subject),
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          parseZendeskTime(ticket.UpdatedAt),
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        contentFingerprint(blob),
	}
	return doc, true, nil
}

func (s *zendeskSyncSession) commentText(ctx context.Context, comment zendeskComment) string {
	header := "Comment"
	if author, ok := s.connector.fetchAuthor(ctx, comment.AuthorID); ok && author.Name != "" {
		header += " by " + author.Name
	}
	if comment.CreatedAt != "" {
		header += " at " + comment.CreatedAt
	}
	return header + ":\n" + comment.Body
}

func (s *zendeskSyncSession) checkpoint(last SourceDocument) *SyncCheckpoint {
	cursor := zendeskSyncCursor{
		ContentType: s.connector.contentType,
		SourceID:    last.SourceID,
	}
	if s.connector.contentType == zendeskContentTypeArticles {
		cursor.StartTime = s.startTime
		cursor.AfterCursor = s.lastDocAfterCursor
		cursor.NextAfterCursor = s.nextArticleAfterCursor
	} else {
		cursor.StartTime = s.lastDocStartTime
		cursor.NextStartTime = s.ticketStartTime
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	updatedAt := last.UpdatedAt
	return &SyncCheckpoint{Cursor: string(data), SourceID: last.SourceID, UpdatedAt: &updatedAt}
}

func (c *ZendeskConnector) isIndexableTicket(ticket zendeskTicket) bool {
	return ticket.Status != "deleted"
}

type zendeskPruneSession struct {
	connector *ZendeskConnector
	done      bool

	articleAfterCursor string
	articleBuffer      []zendeskArticle
	articleIndex       int

	ticketStartTime int64
	ticketBuffer    []zendeskTicket
	ticketIndex     int

	articleHasMore bool
	ticketHasMore  bool
}

func (s *zendeskPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.done {
		return PruneBatch{}, io.EOF
	}
	documents := make([]SlimDocument, 0, zendeskPrunePageSize)
	for len(documents) < zendeskPrunePageSize && !s.done {
		if s.connector.contentType == zendeskContentTypeArticles {
			if err := s.fillArticlePage(ctx); err != nil {
				return PruneBatch{}, err
			}
			for s.articleIndex < len(s.articleBuffer) {
				article := s.articleBuffer[s.articleIndex]
				s.articleIndex++
				if s.connector.isIndexableArticle(article) {
					documents = append(documents, SlimDocument{SourceID: zendeskArticleSourceID(article)})
					if len(documents) >= zendeskPrunePageSize {
						break
					}
				}
			}
		} else {
			if err := s.fillTicketPage(ctx); err != nil {
				return PruneBatch{}, err
			}
			for s.ticketIndex < len(s.ticketBuffer) {
				ticket := s.ticketBuffer[s.ticketIndex]
				s.ticketIndex++
				if s.connector.isIndexableTicket(ticket) {
					documents = append(documents, SlimDocument{SourceID: zendeskTicketSourceID(ticket)})
					if len(documents) >= zendeskPrunePageSize {
						break
					}
				}
			}
		}
		if s.connector.contentType == zendeskContentTypeArticles && s.articleIndex >= len(s.articleBuffer) && !s.articleHasMore {
			s.done = true
		}
		if s.connector.contentType == zendeskContentTypeTickets && s.ticketIndex >= len(s.ticketBuffer) && !s.ticketHasMore {
			s.done = true
		}
	}
	if len(documents) == 0 {
		return PruneBatch{}, io.EOF
	}
	return PruneBatch{Documents: documents}, nil
}

func (s *zendeskPruneSession) Close() error { return nil }

func (s *zendeskPruneSession) fillArticlePage(ctx context.Context) error {
	for s.articleIndex >= len(s.articleBuffer) {
		if !s.articleHasMore {
			return nil
		}
		if s.done {
			return nil
		}
		page, err := s.connector.listArticlesPage(ctx, 0, s.articleAfterCursor, zendeskMaxPageSize)
		if err != nil {
			return err
		}
		if page.Meta.HasMore && (page.Meta.AfterCursor == "" || page.Meta.AfterCursor == s.articleAfterCursor) {
			return fmt.Errorf("Zendesk articles pagination did not advance")
		}
		s.articleAfterCursor = page.Meta.AfterCursor
		s.articleBuffer = page.Articles
		s.articleIndex = 0
		if !page.Meta.HasMore {
			s.articleHasMore = false
		}
	}
	return nil
}

func (s *zendeskPruneSession) fillTicketPage(ctx context.Context) error {
	for s.ticketIndex >= len(s.ticketBuffer) {
		if !s.ticketHasMore {
			return nil
		}
		if s.done {
			return nil
		}
		page, err := s.connector.listTicketsPage(ctx, s.ticketStartTime)
		if err != nil {
			return err
		}
		if !page.EndOfStream && page.EndTime <= s.ticketStartTime {
			return fmt.Errorf("Zendesk tickets pagination did not advance")
		}
		s.ticketStartTime = page.EndTime
		s.ticketBuffer = page.Tickets
		s.ticketIndex = 0
		if page.EndOfStream {
			s.ticketHasMore = false
		}
	}
	return nil
}

type zendeskArticlePage struct {
	Articles []zendeskArticle   `json:"articles"`
	Meta     zendeskArticleMeta `json:"meta"`
}

type zendeskArticleMeta struct {
	HasMore     bool   `json:"has_more"`
	AfterCursor string `json:"after_cursor"`
}

type zendeskArticle struct {
	ID            json.Number   `json:"id"`
	Title         string        `json:"title"`
	Body          string        `json:"body"`
	Draft         bool          `json:"draft"`
	UpdatedAt     string        `json:"updated_at"`
	LabelNames    []string      `json:"label_names"`
	ContentTagIDs []json.Number `json:"content_tag_ids"`
	AuthorID      json.Number   `json:"author_id"`
}

type zendeskContentTagPage struct {
	Records []zendeskContentTag   `json:"records"`
	Meta    zendeskContentTagMeta `json:"meta"`
}

type zendeskContentTagMeta struct {
	HasMore     bool   `json:"has_more"`
	AfterCursor string `json:"after_cursor"`
}

type zendeskContentTag struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
}

type zendeskTicketPage struct {
	Tickets     []zendeskTicket `json:"tickets"`
	EndTime     int64           `json:"end_time"`
	EndOfStream bool            `json:"end_of_stream"`
}

type zendeskTicket struct {
	ID         json.Number `json:"id"`
	Subject    string      `json:"subject"`
	UpdatedAt  string      `json:"updated_at"`
	Status     string      `json:"status"`
	Priority   string      `json:"priority"`
	Tags       []string    `json:"tags"`
	TicketType string      `json:"type"`
	Submitter  json.Number `json:"submitter"`
}

type zendeskCommentsPage struct {
	Comments []zendeskComment `json:"comments"`
}

type zendeskComment struct {
	Body      string      `json:"body"`
	CreatedAt string      `json:"created_at"`
	AuthorID  json.Number `json:"author_id"`
}

type zendeskUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type zendeskUserResponse struct {
	User zendeskUser `json:"user"`
}

type zendeskSyncCursor struct {
	ContentType     string `json:"content_type"`
	StartTime       int64  `json:"start_time"`
	AfterCursor     string `json:"after_cursor,omitempty"`
	NextAfterCursor string `json:"next_after_cursor,omitempty"`
	NextStartTime   int64  `json:"next_start_time,omitempty"`
	SourceID        string `json:"source_id"`
}

type zendeskAPIError struct {
	Status  int
	Message string
}

func (e *zendeskAPIError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("Zendesk API returned HTTP %d: %s", e.Status, e.Message)
}

func classifyZendeskError(err error) error {
	var apiErr *zendeskAPIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Status {
	case http.StatusUnauthorized:
		return &ConnectorMissingCredentialError{Message: "Zendesk credentials appear to be invalid or expired (HTTP 401)."}
	case http.StatusForbidden:
		return &ConnectorValidationError{Message: "Zendesk token does not have permission to access the requested resources (HTTP 403)."}
	case http.StatusNotFound:
		return &ConnectorValidationError{Message: "Zendesk resource not found (HTTP 404)."}
	case http.StatusTooManyRequests:
		return &ConnectorValidationError{Message: "Zendesk rate limit exceeded during validation (HTTP 429)."}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("Unexpected Zendesk error (status=%d): %s", apiErr.Status, apiErr.Message)}
	}
}

func zendeskArticleSourceID(article zendeskArticle) string {
	return "article:" + article.ID.String()
}

func zendeskTicketSourceID(ticket zendeskTicket) string {
	return "zendesk_ticket_" + ticket.ID.String()
}

func zendeskOwnerMetadata(user zendeskUser) map[string]string {
	return map[string]string{"name": user.Name, "email": user.Email}
}
