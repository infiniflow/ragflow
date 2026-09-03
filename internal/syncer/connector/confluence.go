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
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

const (
	defaultConfluenceBatchSize           = 32
	defaultConfluenceAttachmentThreshold = 10 * 1024 * 1024
	confluenceRequestTimeout             = 60 * time.Second
	maxConfluenceResponseSize            = 32 * 1024 * 1024
	maxConfluenceSearchPages             = 1000
)

var (
	confluenceWhitespaceRE = regexp.MustCompile(`[ \t]+`)
	confluenceNewlineRE    = regexp.MustCompile(`\n{3,}`)
)

// ConfluenceConnector reads pages, comments, and attachments from Confluence.
type ConfluenceConnector struct {
	wikiBase            string
	apiBase             string
	isCloud             bool
	indexMode           string
	space               string
	pageID              string
	indexRecursively    bool
	cqlQuery            string
	username            string
	accessToken         string
	batchSize           int
	attachmentThreshold int64
	client              *http.Client

	getJSON        func(ctx context.Context, path string, out any) error
	download       func(ctx context.Context, rawURL string) ([]byte, error)
	getCurrentUser func(ctx context.Context, userID string) (string, error)
}

// NewConfluenceConnector creates a Confluence connector from Python-compatible config.
func NewConfluenceConnector(config map[string]any) (*ConfluenceConnector, error) {
	credentials := configAnyMap(config["credentials"])
	wikiBase := strings.TrimRight(strings.TrimSpace(stringConfig(config["wiki_base"])), "/")
	isCloud := configBoolDefault(config["is_cloud"], true)
	indexMode := strings.ToLower(firstNonEmpty(stringConfig(config["index_mode"]), "everything"))
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultConfluenceBatchSize)
	c := &ConfluenceConnector{
		wikiBase:            wikiBase,
		apiBase:             confluenceAPIBase(wikiBase, isCloud),
		isCloud:             isCloud,
		indexMode:           indexMode,
		space:               strings.TrimSpace(stringConfig(config["space"])),
		pageID:              strings.TrimSpace(stringConfig(config["page_id"])),
		indexRecursively:    configBoolDefault(config["index_recursively"], false),
		cqlQuery:            strings.TrimSpace(stringConfig(config["cql_query"])),
		username:            strings.TrimSpace(stringConfig(credentials["confluence_username"])),
		accessToken:         stringConfig(credentials["confluence_access_token"]),
		batchSize:           batchSize,
		attachmentThreshold: confluenceAttachmentThreshold(),
		client:              &http.Client{Timeout: confluenceRequestTimeout},
	}
	c.getJSON = c.doJSON
	c.download = c.downloadURL
	c.getCurrentUser = c.currentUserName
	return c, nil
}

// Validate validates Confluence settings and credentials.
func (c *ConfluenceConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("confluence connector is nil")
	}
	if c.wikiBase == "" {
		return fmt.Errorf("Confluence wiki_base is required")
	}
	parsed, err := url.Parse(c.wikiBase)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid Confluence wiki_base URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("Confluence wiki_base must use HTTP or HTTPS")
	}
	if c.accessToken == "" {
		return fmt.Errorf("Confluence access token is required")
	}
	if c.isCloud && c.username == "" {
		return fmt.Errorf("Confluence Cloud username is required")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	switch c.indexMode {
	case "everything", "space", "page", "":
	default:
		return fmt.Errorf("invalid Confluence index_mode %q", c.indexMode)
	}
	if c.indexMode == "space" && c.space == "" {
		return fmt.Errorf("Confluence space key is required when index_mode is space")
	}
	if c.indexMode == "page" && c.pageID == "" {
		return fmt.Errorf("Confluence page_id is required when index_mode is page")
	}

	var spaces confluenceSearchResponse
	if err := c.getJSON(ctx, "rest/api/space?limit=1", &spaces); err != nil {
		return confluenceValidationError(err)
	}
	if len(spaces.Results) == 0 {
		return fmt.Errorf("no Confluence spaces found")
	}
	if c.indexMode == "space" {
		var space confluenceSpace
		if err := c.getJSON(ctx, "rest/api/space/"+url.PathEscape(c.space), &space); err != nil {
			return fmt.Errorf("invalid Confluence space key provided: %w", confluenceValidationError(err))
		}
	}
	return nil
}

// ValidateConnectorSetting validates Confluence settings from an unsaved config.
func (c *ConfluenceConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Confluence sync session.
func (c *ConfluenceConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	end := request.WindowEnd
	if end.IsZero() {
		end = time.Now().UTC()
	}
	session := &confluenceSyncSession{
		connector:     c,
		batchSize:     c.batchSize,
		windowStart:   request.WindowStart,
		windowEnd:     end,
		fromBeginning: request.FromBeginning,
		pageCursor:    newConfluenceSearchCursor(c, c.pageCQL(request.WindowStart, end, request.FromBeginning), strings.Join(confluencePageExpansionFields, ",")),
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Confluence prune snapshot session.
func (c *ConfluenceConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	documents, err := c.loadSlimDocuments(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })
	return &confluencePruneSession{documents: documents, batchSize: c.batchSize}, nil
}

func (c *ConfluenceConnector) loadSlimDocuments(ctx context.Context) ([]SlimDocument, error) {
	pages, err := c.searchAll(ctx, c.basePageCQL(), "")
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(pages))
	for _, page := range pages {
		documents = append(documents, SlimDocument{SourceID: c.documentURL(page.Links.WebUI)})
		attachments, err := c.searchAll(ctx, c.attachmentCQL(page.ID.String(), nil, time.Time{}, true), strings.Join(confluenceAttachmentExpansionFields, ","))
		if err != nil {
			return nil, err
		}
		for _, attachment := range attachments {
			if !isConfluenceAttachmentAccepted(attachment) {
				continue
			}
			documents = append(documents, SlimDocument{SourceID: c.documentURL(attachment.Links.WebUI)})
		}
	}
	return documents, nil
}

func (c *ConfluenceConnector) pageDocument(ctx context.Context, page confluenceContent) (SourceDocument, error) {
	pageURL := c.documentURL(page.Links.WebUI)
	content := confluenceHTMLText(page.bodyHTML())
	comments, err := c.pageComments(ctx, page.ID.String())
	if err != nil {
		return SourceDocument{}, err
	}
	if comments != "" {
		content = strings.TrimSpace(content + "\n\nComments:\n" + comments)
	}
	blob := []byte(content)
	updatedAt := parseConfluenceTime(page.Version.When)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	metadata := map[string]any{}
	if page.Space.Name != "" {
		metadata["space"] = page.Space.Name
	}
	if len(page.Metadata.Labels.Results) > 0 {
		labels := make([]string, 0, len(page.Metadata.Labels.Results))
		for _, label := range page.Metadata.Labels.Results {
			if label.Name != "" {
				labels = append(labels, label.Name)
			}
		}
		if len(labels) > 0 {
			metadata["labels"] = labels
		}
	}
	return SourceDocument{
		SourceID:    pageURL,
		Extension:   ".txt",
		Blob:        blob,
		UpdatedAt:   updatedAt,
		SizeBytes:   int64(len(blob)),
		Metadata:    metadataOrNil(metadata),
		Fingerprint: contentFingerprint(blob),
	}, nil
}

func (c *ConfluenceConnector) pageComments(ctx context.Context, pageID string) (string, error) {
	comments, err := c.searchAll(ctx, fmt.Sprintf("type=comment and container='%s'", confluenceCQLQuote(pageID)), "body.storage.value,body.view.value")
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(comments))
	for _, comment := range comments {
		text := confluenceHTMLText(comment.bodyHTML())
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func (c *ConfluenceConnector) attachmentDocument(ctx context.Context, page confluenceContent, attachment confluenceContent) (SourceDocument, bool, error) {
	if !isConfluenceAttachmentAccepted(attachment) {
		return SourceDocument{}, false, nil
	}
	if attachment.Extensions.FileSize > c.attachmentThreshold {
		return SourceDocument{}, false, nil
	}
	rawURL := attachment.Links.Download
	if rawURL == "" {
		return SourceDocument{}, false, nil
	}
	blob, err := c.download(ctx, c.documentURL(rawURL))
	if err != nil {
		return SourceDocument{}, false, err
	}
	if len(blob) == 0 {
		return SourceDocument{}, false, nil
	}
	updatedAt := parseConfluenceTime(attachment.Version.When)
	if updatedAt.IsZero() {
		updatedAt = parseConfluenceTime(page.Version.When)
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	metadata := map[string]any{"parent_page_id": c.documentURL(page.Links.WebUI)}
	if firstNonEmpty(attachment.Space.Name, page.Space.Name) != "" {
		metadata["space"] = firstNonEmpty(attachment.Space.Name, page.Space.Name)
	}
	if len(attachment.Metadata.Labels.Results) > 0 {
		labels := make([]string, 0, len(attachment.Metadata.Labels.Results))
		for _, label := range attachment.Metadata.Labels.Results {
			if label.Name != "" {
				labels = append(labels, label.Name)
			}
		}
		if len(labels) > 0 {
			metadata["labels"] = labels
		}
	}
	title := confluenceAttachmentTitle(attachment)
	return SourceDocument{
		SourceID:    c.documentURL(attachment.Links.WebUI),
		Extension:   confluenceAttachmentExtension(title),
		Blob:        blob,
		UpdatedAt:   updatedAt,
		SizeBytes:   int64(len(blob)),
		Metadata:    metadata,
		Fingerprint: contentFingerprint(blob),
	}, true, nil
}

func confluenceFileNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func confluenceAttachmentTitle(attachment confluenceContent) string {
	return firstNonEmpty(attachment.Title, confluenceFileNameFromURL(attachment.Links.Download), "attachment")
}

func (c *ConfluenceConnector) searchAll(ctx context.Context, cql, expand string) ([]confluenceContent, error) {
	cursor := newConfluenceSearchCursor(c, cql, expand)
	var out []confluenceContent
	for {
		item, ok, err := cursor.next(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		out = append(out, item)
	}
}

// confluenceSearchCursor iterates one Confluence content search, fetching pages
// on demand while guarding against repeated next links and unbounded pagination.
type confluenceSearchCursor struct {
	connector *ConfluenceConnector
	nextPath  string
	seen      map[string]struct{}
	pages     int
	results   []confluenceContent
	index     int
}

func newConfluenceSearchCursor(connector *ConfluenceConnector, cql, expand string) *confluenceSearchCursor {
	return &confluenceSearchCursor{
		connector: connector,
		nextPath:  confluenceCQLPath(cql, expand, connector.batchSize),
		seen:      map[string]struct{}{},
	}
}

func (cur *confluenceSearchCursor) next(ctx context.Context) (confluenceContent, bool, error) {
	for cur.index >= len(cur.results) {
		if cur.nextPath == "" {
			return confluenceContent{}, false, nil
		}
		if _, ok := cur.seen[cur.nextPath]; ok {
			return confluenceContent{}, false, fmt.Errorf("confluence pagination repeated the same next link")
		}
		cur.seen[cur.nextPath] = struct{}{}
		cur.pages++
		if cur.pages > maxConfluenceSearchPages {
			return confluenceContent{}, false, fmt.Errorf("confluence search exceeded %d pages", maxConfluenceSearchPages)
		}
		var page confluenceSearchResponse
		if err := cur.connector.getJSON(ctx, cur.nextPath, &page); err != nil {
			return confluenceContent{}, false, err
		}
		cur.results = page.Results
		cur.index = 0
		cur.nextPath = page.Links.Next
	}
	item := cur.results[cur.index]
	cur.index++
	return item, true, nil
}

func (c *ConfluenceConnector) doJSON(ctx context.Context, path string, out any) error {
	data, err := c.do(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode Confluence response: %w", err)
	}
	return nil
}

func (c *ConfluenceConnector) downloadURL(ctx context.Context, rawURL string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, rawURL)
}

func (c *ConfluenceConnector) do(ctx context.Context, method, rawURL string) ([]byte, error) {
	resolved, err := c.resolveURL(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, resolved, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.isCloud || c.username != "" {
		req.SetBasicAuth(c.username, c.accessToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxConfluenceResponseSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxConfluenceResponseSize {
		return nil, fmt.Errorf("Confluence response exceeds %d bytes", maxConfluenceResponseSize)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &confluenceStatusError{status: res.StatusCode, body: string(body)}
	}
	return body, nil
}

func (c *ConfluenceConnector) resolveURL(rawURL string) (string, error) {
	if c.isCloud && strings.HasPrefix(rawURL, "/rest/") && strings.HasSuffix(c.apiBase, "/wiki") {
		rawURL = "/wiki" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(strings.TrimRight(c.apiBase, "/") + "/")
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() {
		if !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) {
			return "", fmt.Errorf("confluence URL %q targets a different origin than the configured wiki base", rawURL)
		}
		return parsed.String(), nil
	}
	return base.ResolveReference(parsed).String(), nil
}

func (c *ConfluenceConnector) documentURL(contentURL string) string {
	return buildConfluenceDocumentID(c.wikiBase, contentURL, c.isCloud)
}

func (c *ConfluenceConnector) currentUserName(ctx context.Context, userID string) (string, error) {
	var user struct {
		DisplayName string `json:"displayName"`
	}
	field := "key"
	if c.isCloud {
		field = "accountId"
	}
	if err := c.getJSON(ctx, "rest/api/user?"+field+"="+url.QueryEscape(userID), &user); err != nil {
		return "", err
	}
	return user.DisplayName, nil
}

func (c *ConfluenceConnector) basePageCQL() string {
	if c.cqlQuery != "" {
		return c.cqlQuery
	}
	query := "type=page"
	switch c.indexMode {
	case "space":
		query += fmt.Sprintf(" and space='%s'", confluenceCQLQuote(c.space))
	case "page":
		if c.indexRecursively {
			query += fmt.Sprintf(" and (ancestor='%s' or id='%s')", confluenceCQLQuote(c.pageID), confluenceCQLQuote(c.pageID))
		} else {
			query += fmt.Sprintf(" and id='%s'", confluenceCQLQuote(c.pageID))
		}
	}
	return query
}

func (c *ConfluenceConnector) pageCQL(start *time.Time, end time.Time, fromBeginning bool) string {
	query := c.basePageCQL()
	if !fromBeginning && start != nil && !start.IsZero() {
		query += " and lastmodified >= '" + confluenceCQLTime(*start) + "'"
	}
	if !end.IsZero() {
		query += " and lastmodified <= '" + confluenceCQLTime(end) + "'"
	}
	return query + " order by lastmodified asc"
}

func (c *ConfluenceConnector) attachmentCQL(pageID string, start *time.Time, end time.Time, fromBeginning bool) string {
	query := fmt.Sprintf("type=attachment and container='%s'", confluenceCQLQuote(pageID))
	if !fromBeginning && start != nil && !start.IsZero() {
		query += " and lastmodified >= '" + confluenceCQLTime(*start) + "'"
	}
	if !end.IsZero() {
		query += " and lastmodified <= '" + confluenceCQLTime(end) + "'"
	}
	return query + " order by lastmodified asc"
}

type confluenceSyncSession struct {
	connector     *ConfluenceConnector
	batchSize     int
	windowStart   *time.Time
	windowEnd     time.Time
	fromBeginning bool

	pageCursor     *confluenceSearchCursor
	currentPage    confluenceContent
	hasCurrentPage bool
	pageDocPending bool
	attachCursor   *confluenceSearchCursor

	resumeSourceID  string
	resumeMatched   bool
	resumeUpdatedAt *time.Time
}

// NextBatch returns the next Confluence document batch, fetching pages,
// comments, and attachments incrementally so session memory stays bounded by
// batchSize.
func (s *confluenceSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	var checkpoint *SyncCheckpoint
	for len(documents) < s.batchSize {
		doc, err := s.nextDocument(ctx)
		if errors.Is(err, io.EOF) {
			if s.resumeSourceID != "" && !s.resumeMatched {
				return SyncBatch{}, fmt.Errorf("confluence sync resume checkpoint %q was not found in the source: %w", s.resumeSourceID, ErrSyncResumeInvalid)
			}
			if len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			break
		}
		if err != nil {
			return SyncBatch{}, err
		}
		if !s.includeResumed(*doc) {
			continue
		}
		documents = append(documents, *doc)
		checkpoint = confluenceSyncCheckpoint(*doc)
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close closes the Confluence sync session.
func (s *confluenceSyncSession) Close() error {
	return nil
}

// nextDocument produces the next document in page order: each page followed by
// its accepted attachments, all filtered by the configured window.
func (s *confluenceSyncSession) nextDocument(ctx context.Context) (*SourceDocument, error) {
	for {
		if !s.hasCurrentPage {
			page, ok, err := s.pageCursor.next(ctx)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, io.EOF
			}
			s.currentPage = page
			s.hasCurrentPage = true
			s.pageDocPending = true
			s.attachCursor = newConfluenceSearchCursor(s.connector, s.connector.attachmentCQL(page.ID.String(), s.windowStart, s.windowEnd, s.fromBeginning), strings.Join(confluenceAttachmentExpansionFields, ","))
		}

		if s.pageDocPending {
			s.pageDocPending = false
			doc, err := s.connector.pageDocument(ctx, s.currentPage)
			if err != nil {
				return nil, err
			}
			if s.fromBeginning || inConfluenceWindow(doc.UpdatedAt, s.windowStart, s.windowEnd) {
				doc.SemanticIdentifier = confluenceSemanticIdentifier(s.currentPage.Space.Name, s.currentPage.ancestorTitles(), s.currentPage.Title)
				return &doc, nil
			}
			continue
		}

		attachment, ok, err := s.attachCursor.next(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			s.hasCurrentPage = false
			continue
		}
		doc, accepted, err := s.connector.attachmentDocument(ctx, s.currentPage, attachment)
		if err != nil {
			return nil, err
		}
		if !accepted {
			continue
		}
		if s.fromBeginning || inConfluenceWindow(doc.UpdatedAt, s.windowStart, s.windowEnd) {
			doc.SemanticIdentifier = confluenceSemanticIdentifier(firstNonEmpty(s.currentPage.Space.Name, attachment.Space.Name), s.currentPage.ancestorTitles(), s.currentPage.Title+" / "+confluenceAttachmentTitle(attachment))
			return &doc, nil
		}
	}
}

func (s *confluenceSyncSession) includeResumed(doc SourceDocument) bool {
	if s.resumeMatched || s.resumeSourceID == "" {
		return true
	}
	if doc.SourceID == s.resumeSourceID {
		s.resumeMatched = true
		return false
	}
	return false
}

func (s *confluenceSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("confluence sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	s.resumeSourceID = sourceID
	s.resumeUpdatedAt = checkpoint.UpdatedAt
	return nil
}

func confluenceSyncCheckpoint(doc SourceDocument) *SyncCheckpoint {
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{Cursor: doc.SourceID, SourceID: doc.SourceID, UpdatedAt: &updatedAt}
}

type confluencePruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

// NextBatch returns the next Confluence prune snapshot batch.
func (s *confluencePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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

// Close closes the Confluence prune session.
func (s *confluencePruneSession) Close() error {
	return nil
}

type confluenceSearchResponse struct {
	Results []confluenceContent `json:"results"`
	Links   struct {
		Next string `json:"next"`
	} `json:"_links"`
}

type confluenceSpace struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type confluenceContent struct {
	ID    confluenceString `json:"id"`
	Title string           `json:"title"`
	Type  string           `json:"type"`
	Body  struct {
		Storage confluenceBody `json:"storage"`
		View    confluenceBody `json:"view"`
	} `json:"body"`
	Space     confluenceSpace `json:"space"`
	Ancestors []struct {
		Title string `json:"title"`
	} `json:"ancestors"`
	Version struct {
		When string `json:"when"`
		By   struct {
			DisplayName string `json:"displayName"`
			Email       string `json:"email"`
		} `json:"by"`
	} `json:"version"`
	Metadata struct {
		MediaType string `json:"mediaType"`
		Labels    struct {
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
		} `json:"labels"`
	} `json:"metadata"`
	Extensions struct {
		FileSize int64 `json:"fileSize"`
	} `json:"extensions"`
	Links struct {
		WebUI    string `json:"webui"`
		Download string `json:"download"`
	} `json:"_links"`
}

type confluenceBody struct {
	Value string `json:"value"`
}

type confluenceString string

func (s *confluenceString) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = confluenceString(text)
		return nil
	}

	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		*s = confluenceString(number.String())
		return nil
	}

	return fmt.Errorf("Confluence string field must be string or number")
}

func (s confluenceString) String() string {
	return string(s)
}

func (c confluenceContent) bodyHTML() string {
	return firstNonEmpty(c.Body.Storage.Value, c.Body.View.Value)
}

func (c confluenceContent) ancestorTitles() []string {
	out := make([]string, 0, len(c.Ancestors))
	for _, ancestor := range c.Ancestors {
		if ancestor.Title != "" {
			out = append(out, ancestor.Title)
		}
	}
	return out
}

func confluenceAPIBase(wikiBase string, isCloud bool) string {
	base := strings.TrimRight(wikiBase, "/")
	if isCloud && base != "" && !strings.HasSuffix(base, "/wiki") {
		base += "/wiki"
	}
	return base
}

func buildConfluenceDocumentID(baseURL, contentURL string, isCloud bool) string {
	finalBase := strings.TrimRight(baseURL, "/") + "/"
	if isCloud && !strings.HasSuffix(finalBase, "/wiki/") {
		finalBase += "wiki/"
	}
	base, err := url.Parse(finalBase)
	if err != nil {
		return strings.TrimRight(finalBase, "/") + "/" + strings.TrimLeft(contentURL, "/")
	}
	ref, err := url.Parse(strings.TrimLeft(contentURL, "/"))
	if err != nil {
		return strings.TrimRight(finalBase, "/") + "/" + strings.TrimLeft(contentURL, "/")
	}
	return base.ResolveReference(ref).String()
}

func confluenceCQLPath(cql, expand string, limit int) string {
	values := url.Values{}
	values.Set("cql", cql)
	if expand != "" {
		values.Set("expand", expand)
	}
	values.Set("limit", fmt.Sprint(limit))
	return "rest/api/content/search?" + values.Encode()
}

func confluenceCQLQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, "'", "\\'")
}

func confluenceCQLTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04")
}

func inConfluenceWindow(updatedAt time.Time, start *time.Time, end time.Time) bool {
	if !end.IsZero() && updatedAt.After(end) {
		return false
	}
	if start != nil && !updatedAt.After(*start) {
		return false
	}
	return true
}

func parseConfluenceTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func confluenceSemanticIdentifier(space string, ancestors []string, title string) string {
	title = firstNonEmpty(title, "Untitled")
	parts := make([]string, 0, len(ancestors)+2)
	if space != "" {
		parts = append(parts, space)
	}
	parts = append(parts, ancestors...)
	parts = append(parts, title)
	return strings.Join(parts, " / ")
}

func confluenceHTMLText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	root, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return html.UnescapeString(raw)
	}
	var buffer bytes.Buffer
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			switch strings.ToLower(node.Data) {
			case "script", "style":
				return
			case "br", "p", "div", "tr", "li", "table", "h1", "h2", "h3", "h4", "h5", "h6":
				buffer.WriteByte('\n')
			}
		}
		if node.Type == xhtml.TextNode {
			text := strings.TrimSpace(html.UnescapeString(node.Data))
			if text != "" {
				if buffer.Len() > 0 {
					buffer.WriteByte(' ')
				}
				buffer.WriteString(text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == xhtml.ElementNode {
			switch strings.ToLower(node.Data) {
			case "p", "div", "tr", "li", "table", "h1", "h2", "h3", "h4", "h5", "h6":
				buffer.WriteByte('\n')
			}
		}
	}
	walk(root)
	text := strings.ReplaceAll(buffer.String(), "\r\n", "\n")
	text = confluenceWhitespaceRE.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(text, " \n", "\n")
	text = strings.ReplaceAll(text, "\n ", "\n")
	text = confluenceNewlineRE.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func metadataOrNil(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func isConfluenceAttachmentAccepted(attachment confluenceContent) bool {
	mediaType := strings.ToLower(attachment.Metadata.MediaType)
	if strings.HasPrefix(mediaType, "image/") {
		switch mediaType {
		case "image/jpeg", "image/jpg", "image/png", "image/gif", "image/bmp", "image/tiff", "image/webp":
			return true
		default:
			return false
		}
	}
	ext := confluenceAttachmentExtension(attachment.Title)
	if _, ok := webdavTextExtensions[ext]; ok {
		return true
	}
	if _, ok := webdavDocumentExtensions[ext]; ok {
		return true
	}
	return false
}

func confluenceAttachmentExtension(title string) string {
	ext := strings.ToLower(path.Ext(title))
	if ext == "" {
		return ".unknown"
	}
	return ext
}

func confluenceAttachmentThreshold() int64 {
	if raw := strings.TrimSpace(os.Getenv("CONFLUENCE_CONNECTOR_ATTACHMENT_SIZE_THRESHOLD")); raw != "" {
		if parsed := configInt(raw, defaultConfluenceAttachmentThreshold); parsed > 0 {
			return int64(parsed)
		}
	}
	return defaultConfluenceAttachmentThreshold
}

func confluenceValidationError(err error) error {
	var statusErr *confluenceStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.status {
		case http.StatusUnauthorized:
			return fmt.Errorf("invalid or expired Confluence credentials")
		case http.StatusForbidden:
			return fmt.Errorf("insufficient permissions to access Confluence resources")
		}
	}
	return err
}

type confluenceStatusError struct {
	status int
	body   string
}

func (e *confluenceStatusError) Error() string {
	return fmt.Sprintf("Confluence request failed with status %d: %s", e.status, strings.TrimSpace(e.body))
}

var confluencePageExpansionFields = []string{
	"body.storage.value",
	"body.view.value",
	"space",
	"ancestors",
	"version",
	"metadata.labels",
}

var confluenceAttachmentExpansionFields = []string{
	"metadata",
	"metadata.labels",
	"extensions",
	"version",
	"space",
}
