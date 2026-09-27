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
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultNotionBatchSize    = 32
	defaultNotionFileMaxBytes = 100 * 1024 * 1024
	notionAPIBaseURL          = "https://api.notion.com/v1"
	notionVersion             = "2022-06-28"
)

// NotionConnector reads pages and attachments from Notion.
type NotionConnector struct {
	rootPageID      string
	integrationKey  string
	batchSize       int
	fileMaxBytes    int64
	recursiveLookup bool
	httpClient      *http.Client
	indexedPages    map[string]bool
	pagePathCache   map[string]string

	searchPages      func(ctx context.Context, request notionSearchRequest) (notionSearchResponse, error)
	fetchPage        func(ctx context.Context, pageID string) (notionPage, error)
	fetchDatabase    func(ctx context.Context, databaseID string) (notionPage, error)
	queryDatabase    func(ctx context.Context, databaseID, cursor string) (notionDatabaseQueryResponse, error)
	fetchChildBlocks func(ctx context.Context, blockID, cursor string) (notionBlockPage, error)
	downloadFile     func(ctx context.Context, rawURL string) ([]byte, error)
}

// NewNotionConnector creates a Notion connector from the connector config map.
func NewNotionConnector(config map[string]any) (*NotionConnector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultNotionBatchSize)
	fileMaxBytes := int64(configInt(firstNonEmpty(stringConfig(config["file_max_bytes"]), stringConfig(config["max_file_size"]), stringConfig(config["max_attachment_size"])), defaultNotionFileMaxBytes))
	rootPageID := strings.TrimSpace(stringConfig(config["root_page_id"]))
	return &NotionConnector{
		rootPageID:      rootPageID,
		integrationKey:  strings.TrimSpace(stringConfig(credentials["notion_integration_token"])),
		batchSize:       batchSize,
		fileMaxBytes:    fileMaxBytes,
		recursiveLookup: true,
		httpClient:      &http.Client{Timeout: 60 * time.Second},
		indexedPages:    map[string]bool{},
		pagePathCache:   map[string]string{},
	}, nil
}

// Validate validates Notion connector settings and credentials.
func (c *NotionConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("notion connector is nil")
	}
	if c.integrationKey == "" {
		return fmt.Errorf("Notion credentials not loaded.")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if c.rootPageID != "" {
		_, err := c.getPage(ctx, c.rootPageID)
		return err
	}
	_, err := c.search(ctx, notionSearchRequest{
		Filter:   map[string]any{"property": "object", "value": "page"},
		PageSize: 1,
	})
	return err
}

// ValidateConnectorSetting validates Notion settings from an unsaved config.
func (c *NotionConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Notion sync session.
func (c *NotionConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	c.indexedPages = map[string]bool{}
	c.pagePathCache = map[string]string{}
	resumeSourceID, err := notionResumeSourceID(request.Resume)
	if err != nil {
		return nil, err
	}
	session := &notionSyncSession{
		connector:      c,
		request:        request,
		batchSize:      c.batchSize,
		resumeSourceID: resumeSourceID,
		searchRequest: notionSearchRequest{
			Filter:   map[string]any{"property": "object", "value": "page"},
			PageSize: 100,
		},
	}
	if !request.FromBeginning {
		session.searchRequest.Sort = map[string]any{"timestamp": "last_edited_time", "direction": "descending"}
	}
	return session, nil
}

// OpenPrune opens one complete Notion prune snapshot session.
func (c *NotionConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	c.indexedPages = map[string]bool{}
	documents, err := c.loadSlimDocuments(ctx)
	if err != nil {
		return nil, err
	}
	return &notionPruneSession{documents: documents, batchSize: c.batchSize}, nil
}

func (c *NotionConnector) loadSlimDocuments(ctx context.Context) ([]SlimDocument, error) {
	seen := map[string]bool{}
	if c.recursiveLookup && c.rootPageID != "" {
		page, err := c.getPage(ctx, c.rootPageID)
		if err != nil {
			return nil, err
		}
		return c.readSlimPages(ctx, []notionPage{page}, seen)
	}

	var documents []SlimDocument
	searchRequest := notionSearchRequest{
		Filter:   map[string]any{"property": "object", "value": "page"},
		PageSize: 100,
	}
	for {
		response, err := c.search(ctx, searchRequest)
		if err != nil {
			return nil, err
		}
		pageDocs, err := c.readSlimPages(ctx, response.Results, seen)
		if err != nil {
			return nil, err
		}
		documents = append(documents, pageDocs...)
		if !response.HasMore {
			break
		}
		searchRequest.StartCursor = response.NextCursor
	}
	return documents, nil
}

func (c *NotionConnector) readPage(ctx context.Context, page notionPage, request SyncRequest) ([]SourceDocument, []string, error) {
	if c.indexedPages[page.ID] {
		return nil, nil, nil
	}
	updatedAt := parseNotionTime(page.LastEditedTime)
	inWindow := request.FromBeginning || includeNotionUpdatedAt(updatedAt, request)
	pagePath := c.buildPagePath(ctx, page, map[string]bool{})
	blocks, children, attachments, err := c.readBlocks(ctx, page.ID, page.LastEditedTime, pagePath)
	if err != nil {
		return nil, nil, err
	}
	c.indexedPages[page.ID] = true

	if !inWindow {
		return nil, children, nil
	}

	title := c.pageTitle(page)
	if title == "" {
		title = "Untitled Page with ID " + page.ID
	}
	semanticIdentifier := pagePath
	if semanticIdentifier == "" {
		semanticIdentifier = title
	}
	semanticIdentifier += "_" + page.ID

	text := title
	if len(blocks) > 0 {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			parts = append(parts, block.Prefix+block.Text)
		}
		text = strings.Join(parts, "\n")
	} else if len(page.Properties) > 0 {
		text += "\n\n" + notionPropertiesToText(page.Properties)
	}
	blob := []byte(text)
	documents := []SourceDocument{{
		SourceID:           page.ID,
		SemanticIdentifier: semanticIdentifier,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           map[string]any{"url": page.URL},
		Fingerprint:        stableFingerprint(map[string]any{"id": page.ID, "last_edited_time": page.LastEditedTime}),
	}}
	documents = append(documents, attachments...)
	return documents, children, nil
}

func (c *NotionConnector) readBlocks(ctx context.Context, blockID, pageLastEditedTime, pagePath string) ([]notionTextBlock, []string, []SourceDocument, error) {
	var blocks []notionTextBlock
	var childPageIDs []string
	var attachments []SourceDocument
	cursor := ""
	for {
		response, err := c.children(ctx, blockID, cursor)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, block := range response.Results {
			blockObject := block.TypedObject()
			switch block.Type {
			case "ai_block", "unsupported", "external_object_instance_page":
				continue
			case "equation":
				if expression := stringConfig(blockObject["expression"]); expression != "" {
					blocks = append(blocks, notionTextBlock{ID: block.ID, Text: expression, Prefix: "\n"})
				}
				continue
			case "file", "image", "pdf", "video", "audio":
				doc, label, err := c.attachmentDocument(ctx, block, blockObject, pageLastEditedTime, pagePath)
				if err != nil {
					return nil, nil, nil, err
				}
				if doc != nil {
					attachments = append(attachments, *doc)
				}
				if label != "" {
					blocks = append(blocks, notionTextBlock{ID: block.ID, Text: notionBlockLabel(block.Type) + ": " + label, Prefix: "\n"})
				}
			case "child_database":
				databaseBlocks, databaseChildren, err := c.readPagesFromDatabase(ctx, block.ID)
				if err != nil {
					return nil, nil, nil, err
				}
				blocks = append(blocks, databaseBlocks...)
				if c.recursiveLookup {
					childPageIDs = append(childPageIDs, databaseChildren...)
				}
			default:
				text := extractNotionRichText(blockObject["rich_text"])
				if text != "" || block.Type == "bulleted_list_item" || block.Type == "numbered_list_item" || block.Type == "to_do" {
					blocks = append(blocks, notionTextBlock{ID: block.ID, Text: formatNotionBlockText(block.Type, text, blockObject), Prefix: "\n"})
				}
			}

			if block.HasChildren {
				if block.Type == "child_page" {
					childPageIDs = append(childPageIDs, block.ID)
				} else if block.Type != "child_database" {
					nestedBlocks, nestedChildren, nestedAttachments, err := c.readBlocks(ctx, block.ID, pageLastEditedTime, pagePath)
					if err != nil {
						return nil, nil, nil, err
					}
					blocks = append(blocks, nestedBlocks...)
					childPageIDs = append(childPageIDs, nestedChildren...)
					attachments = append(attachments, nestedAttachments...)
				}
			}
		}
		if !response.HasMore {
			break
		}
		cursor = response.NextCursor
	}
	return blocks, childPageIDs, attachments, nil
}

func (c *NotionConnector) readPagesFromDatabase(ctx context.Context, databaseID string) ([]notionTextBlock, []string, error) {
	var blocks []notionTextBlock
	var childPageIDs []string
	cursor := ""
	for {
		response, err := c.databaseQuery(ctx, databaseID, cursor)
		if err != nil {
			return nil, nil, err
		}
		for _, result := range response.Results {
			if text := notionPropertiesToText(result.Properties); text != "" {
				blocks = append(blocks, notionTextBlock{ID: result.ID, Text: text, Prefix: "\n"})
			}
			if c.recursiveLookup && result.Object == "page" {
				childPageIDs = append(childPageIDs, result.ID)
			}
		}
		if !response.HasMore {
			break
		}
		cursor = response.NextCursor
	}
	return blocks, childPageIDs, nil
}

func (c *NotionConnector) readSlimPages(ctx context.Context, pages []notionPage, seen map[string]bool) ([]SlimDocument, error) {
	var documents []SlimDocument
	var childPageIDs []string
	for _, page := range pages {
		if seen[page.ID] {
			continue
		}
		children, attachments, err := c.readSlimBlocks(ctx, page.ID)
		if err != nil {
			return nil, err
		}
		childPageIDs = append(childPageIDs, children...)
		seen[page.ID] = true
		documents = append(documents, SlimDocument{SourceID: page.ID})
		for _, attachmentID := range attachments {
			documents = append(documents, SlimDocument{SourceID: attachmentID})
		}
	}
	if c.recursiveLookup && len(childPageIDs) > 0 {
		childPages := make([]notionPage, 0, len(childPageIDs))
		for _, pageID := range childPageIDs {
			if seen[pageID] {
				continue
			}
			page, err := c.getPage(ctx, pageID)
			if err != nil {
				return nil, err
			}
			childPages = append(childPages, page)
		}
		childDocs, err := c.readSlimPages(ctx, childPages, seen)
		if err != nil {
			return nil, err
		}
		documents = append(documents, childDocs...)
	}
	return documents, nil
}

func (c *NotionConnector) readSlimBlocks(ctx context.Context, blockID string) ([]string, []string, error) {
	var childPageIDs []string
	var attachmentIDs []string
	cursor := ""
	for {
		response, err := c.children(ctx, blockID, cursor)
		if err != nil {
			return nil, nil, err
		}
		for _, block := range response.Results {
			if block.Type == "file" || block.Type == "image" || block.Type == "pdf" || block.Type == "video" || block.Type == "audio" {
				attachmentIDs = append(attachmentIDs, block.ID)
			}
			if block.HasChildren {
				if block.Type == "child_page" {
					childPageIDs = append(childPageIDs, block.ID)
				} else {
					nestedChildren, nestedAttachments, err := c.readSlimBlocks(ctx, block.ID)
					if err != nil {
						return nil, nil, err
					}
					childPageIDs = append(childPageIDs, nestedChildren...)
					attachmentIDs = append(attachmentIDs, nestedAttachments...)
				}
			}
			if block.Type == "child_database" && c.recursiveLookup {
				_, databaseChildren, err := c.readPagesFromDatabase(ctx, block.ID)
				if err != nil {
					return nil, nil, err
				}
				childPageIDs = append(childPageIDs, databaseChildren...)
			}
		}
		if !response.HasMore {
			break
		}
		cursor = response.NextCursor
	}
	return childPageIDs, attachmentIDs, nil
}

func (c *NotionConnector) attachmentDocument(ctx context.Context, block notionBlock, object map[string]any, pageLastEditedTime, pagePath string) (*SourceDocument, string, error) {
	urlValue, name, caption := notionFileMetadata(block.ID, object)
	if urlValue == "" {
		return nil, "", nil
	}
	data, err := c.file(ctx, urlValue)
	if err != nil {
		return nil, "", err
	}
	extension := filepath.Ext(name)
	if extension == "" {
		extension = filepath.Ext(path.Base(urlValue))
	}
	if extension == "" {
		extension = ".bin"
	}
	semanticIdentifier := name
	if caption != "" {
		semanticIdentifier += " (" + caption + ")"
	}
	if pagePath != "" {
		semanticIdentifier = pagePath + " / " + semanticIdentifier
	}
	updatedAt := parseNotionTime(pageLastEditedTime)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return &SourceDocument{
		SourceID:           block.ID,
		SemanticIdentifier: semanticIdentifier,
		Extension:          extension,
		Blob:               data,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(data)),
		Metadata:           map[string]any{"url": urlValue},
		Fingerprint:        contentFingerprint(data),
	}, semanticIdentifier, nil
}

func (c *NotionConnector) buildPagePath(ctx context.Context, page notionPage, visited map[string]bool) string {
	if cached := c.pagePathCache[page.ID]; cached != "" {
		return cached
	}
	if visited[page.ID] {
		return c.pageTitle(page)
	}
	visited[page.ID] = true
	currentTitle := c.pageTitle(page)
	if currentTitle == "" {
		currentTitle = "Untitled Page " + page.ID
	}
	parentID := page.ParentID()
	parentPath := ""
	if parentID != "" {
		if parent, err := c.getPage(ctx, parentID); err == nil {
			parentPath = c.buildPagePath(ctx, parent, visited)
		}
	}
	fullPath := currentTitle
	if parentPath != "" {
		fullPath = parentPath + " / " + currentTitle
	}
	c.pagePathCache[page.ID] = fullPath
	return fullPath
}

func (c *NotionConnector) pageTitle(page notionPage) string {
	if page.DatabaseName != "" {
		return page.DatabaseName
	}
	for _, rawProperty := range page.Properties {
		property, ok := rawProperty.(map[string]any)
		if !ok {
			continue
		}
		if stringConfig(property["type"]) != "title" {
			continue
		}
		return extractNotionRichText(property["title"])
	}
	return ""
}

func (c *NotionConnector) search(ctx context.Context, request notionSearchRequest) (notionSearchResponse, error) {
	if c.searchPages != nil {
		return c.searchPages(ctx, request)
	}
	var response notionSearchResponse
	err := c.doJSON(ctx, http.MethodPost, notionAPIBaseURL+"/search", request, &response)
	return response, err
}

func (c *NotionConnector) getPage(ctx context.Context, pageID string) (notionPage, error) {
	if c.fetchPage != nil {
		return c.fetchPage(ctx, pageID)
	}
	var page notionPage
	err := c.doJSON(ctx, http.MethodGet, notionAPIBaseURL+"/pages/"+pageID, nil, &page)
	if err == nil {
		return page, nil
	}
	if c.fetchDatabase != nil {
		return c.fetchDatabase(ctx, pageID)
	}
	var database notionDatabase
	if dbErr := c.doJSON(ctx, http.MethodGet, notionAPIBaseURL+"/databases/"+pageID, nil, &database); dbErr != nil {
		return notionPage{}, err
	}
	return database.toPage(), nil
}

func (c *NotionConnector) children(ctx context.Context, blockID, cursor string) (notionBlockPage, error) {
	if c.fetchChildBlocks != nil {
		return c.fetchChildBlocks(ctx, blockID, cursor)
	}
	endpoint := notionAPIBaseURL + "/blocks/" + blockID + "/children"
	if cursor != "" {
		endpoint += "?" + url.Values{"start_cursor": {cursor}}.Encode()
	}
	var response notionBlockPage
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response)
	return response, err
}

func (c *NotionConnector) databaseQuery(ctx context.Context, databaseID, cursor string) (notionDatabaseQueryResponse, error) {
	if c.queryDatabase != nil {
		return c.queryDatabase(ctx, databaseID, cursor)
	}
	body := map[string]any{}
	if cursor != "" {
		body["start_cursor"] = cursor
	}
	var response notionDatabaseQueryResponse
	err := c.doJSON(ctx, http.MethodPost, notionAPIBaseURL+"/databases/"+databaseID+"/query", body, &response)
	return response, err
}

func (c *NotionConnector) file(ctx context.Context, rawURL string) ([]byte, error) {
	if c.downloadFile != nil {
		return c.downloadFile(ctx, rawURL)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("notion file download failed with HTTP %d", response.StatusCode)
	}
	if response.ContentLength > c.fileMaxBytes {
		return nil, fmt.Errorf("notion file exceeds maximum size: %d > %d bytes", response.ContentLength, c.fileMaxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, c.fileMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.fileMaxBytes {
		return nil, fmt.Errorf("notion file exceeds maximum size: > %d bytes", c.fileMaxBytes)
	}
	return data, nil
}

func (c *NotionConnector) doJSON(ctx context.Context, method, url string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.integrationKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Notion-Version", notionVersion)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return notionHTTPError(response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

type notionSyncSession struct {
	connector      *NotionConnector
	request        SyncRequest
	batchSize      int
	pageQueue      []notionPage
	buffer         []SourceDocument
	rootFetched    bool
	searchRequest  notionSearchRequest
	searchDone     bool
	resumeSourceID string
	resumeMatched  bool
}

func (s *notionSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		if len(s.buffer) > 0 {
			n := min(s.batchSize-len(documents), len(s.buffer))
			documents = append(documents, s.buffer[:n]...)
			s.buffer = s.buffer[n:]
			continue
		}
		pageDocs, err := s.nextPageDocuments(ctx)
		if errors.Is(err, io.EOF) {
			if s.resumeSourceID != "" && !s.resumeMatched {
				return SyncBatch{}, fmt.Errorf("notion sync resume checkpoint %q was not found in the source: %w", s.resumeSourceID, ErrSyncResumeInvalid)
			}
			if len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			break
		}
		if err != nil {
			return SyncBatch{}, err
		}
		s.buffer = append(s.buffer, s.applyResume(pageDocs)...)
	}
	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	last := documents[len(documents)-1]
	updatedAt := last.UpdatedAt
	return SyncBatch{Documents: documents, Checkpoint: &SyncCheckpoint{Cursor: last.SourceID, SourceID: last.SourceID, UpdatedAt: &updatedAt}}, nil
}

func (s *notionSyncSession) Close() error { return nil }

func (s *notionSyncSession) nextPageDocuments(ctx context.Context) ([]SourceDocument, error) {
	for {
		if len(s.pageQueue) == 0 {
			if err := s.loadMorePages(ctx); err != nil {
				return nil, err
			}
			continue
		}
		page := s.pageQueue[0]
		s.pageQueue = s.pageQueue[1:]
		documents, children, err := s.connector.readPage(ctx, page, s.request)
		if err != nil {
			return nil, err
		}
		if s.connector.recursiveLookup {
			for _, childID := range children {
				if s.connector.indexedPages[childID] {
					continue
				}
				childPage, err := s.connector.getPage(ctx, childID)
				if err != nil {
					return nil, err
				}
				s.pageQueue = append(s.pageQueue, childPage)
			}
		}
		if len(documents) > 0 {
			return documents, nil
		}
	}
}

func (s *notionSyncSession) loadMorePages(ctx context.Context) error {
	if s.connector.recursiveLookup && s.connector.rootPageID != "" {
		if s.rootFetched {
			return io.EOF
		}
		page, err := s.connector.getPage(ctx, s.connector.rootPageID)
		if err != nil {
			return err
		}
		s.rootFetched = true
		s.pageQueue = append(s.pageQueue, page)
		return nil
	}
	for len(s.pageQueue) == 0 {
		if s.searchDone {
			return io.EOF
		}
		response, err := s.connector.search(ctx, s.searchRequest)
		if err != nil {
			return err
		}
		s.pageQueue = append(s.pageQueue, filterNotionPages(response.Results, s.request)...)
		if !response.HasMore || response.NextCursor == "" || notionSearchResultsOlderThanWindowStart(response.Results, s.request) {
			s.searchDone = true
			continue
		}
		s.searchRequest.StartCursor = response.NextCursor
	}
	return nil
}

func (s *notionSyncSession) applyResume(documents []SourceDocument) []SourceDocument {
	if s.resumeSourceID == "" || s.resumeMatched {
		return documents
	}
	for index, document := range documents {
		if document.SourceID == s.resumeSourceID {
			s.resumeMatched = true
			return documents[index+1:]
		}
	}
	return nil
}

type notionPruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

func (s *notionPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.index >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	documents := s.documents[s.index:end]
	s.index = end
	return PruneBatch{Documents: documents}, nil
}

func (s *notionPruneSession) Close() error { return nil }

type notionSearchRequest struct {
	Filter      map[string]any `json:"filter,omitempty"`
	Sort        map[string]any `json:"sort,omitempty"`
	PageSize    int            `json:"page_size,omitempty"`
	StartCursor string         `json:"start_cursor,omitempty"`
}

type notionSearchResponse struct {
	Results    []notionPage `json:"results"`
	NextCursor string       `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}

type notionDatabaseQueryResponse struct {
	Results    []notionDatabaseResult `json:"results"`
	NextCursor string                 `json:"next_cursor"`
	HasMore    bool                   `json:"has_more"`
}

type notionDatabaseResult struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	Properties map[string]any `json:"properties"`
}

type notionPage struct {
	ID             string         `json:"id"`
	Object         string         `json:"object"`
	CreatedTime    string         `json:"created_time"`
	LastEditedTime string         `json:"last_edited_time"`
	Archived       bool           `json:"archived"`
	Properties     map[string]any `json:"properties"`
	URL            string         `json:"url"`
	Parent         map[string]any `json:"parent"`
	DatabaseName   string         `json:"database_name"`
}

func (p notionPage) ParentID() string {
	parentType := stringConfig(p.Parent["type"])
	if parentType != "page_id" && parentType != "database_id" {
		return ""
	}
	return stringConfig(p.Parent[parentType])
}

type notionDatabase struct {
	ID             string           `json:"id"`
	LastEditedTime string           `json:"last_edited_time"`
	URL            string           `json:"url"`
	Parent         map[string]any   `json:"parent"`
	Title          []map[string]any `json:"title"`
}

func (d notionDatabase) toPage() notionPage {
	return notionPage{
		ID:             d.ID,
		Object:         "database",
		LastEditedTime: d.LastEditedTime,
		URL:            d.URL,
		Parent:         d.Parent,
		Properties:     map[string]any{},
		DatabaseName:   extractNotionRichText(d.Title),
	}
}

type notionBlockPage struct {
	Results    []notionBlock `json:"results"`
	NextCursor string        `json:"next_cursor"`
	HasMore    bool          `json:"has_more"`
}

type notionBlock struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	HasChildren bool           `json:"has_children"`
	Raw         map[string]any `json:"-"`
}

func (b *notionBlock) UnmarshalJSON(data []byte) error {
	type alias notionBlock
	var base alias
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*b = notionBlock(base)
	b.Raw = raw
	return nil
}

func (b notionBlock) TypedObject() map[string]any {
	if typed, ok := b.Raw[b.Type].(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

type notionTextBlock struct {
	ID     string
	Text   string
	Prefix string
}

func filterNotionPages(pages []notionPage, request SyncRequest) []notionPage {
	if request.FromBeginning {
		return pages
	}
	out := make([]notionPage, 0, len(pages))
	for _, page := range pages {
		if includeNotionUpdatedAt(parseNotionTime(page.LastEditedTime), request) {
			out = append(out, page)
		}
	}
	return out
}

func notionSearchResultsOlderThanWindowStart(pages []notionPage, request SyncRequest) bool {
	if request.FromBeginning || request.WindowStart == nil || len(pages) == 0 {
		return false
	}
	for _, page := range pages {
		updatedAt := parseNotionTime(page.LastEditedTime)
		if updatedAt.IsZero() || !beforeOrAtWindowStart(updatedAt, request.WindowStart) {
			return false
		}
	}
	return true
}

func notionResumeSourceID(checkpoint *SyncCheckpoint) (string, error) {
	if checkpoint == nil {
		return "", nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return "", fmt.Errorf("notion sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	return sourceID, nil
}

func includeNotionUpdatedAt(updatedAt time.Time, request SyncRequest) bool {
	if updatedAt.IsZero() {
		return true
	}
	return !beforeOrAtWindowStart(updatedAt, request.WindowStart) && !afterWindowEnd(updatedAt, request.WindowEnd)
}

func parseNotionTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func extractNotionRichText(value any) string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		}
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		richText, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := notionRichTextContent(richText)
		if href := stringConfig(richText["href"]); content != "" && href != "" {
			content += " (" + href + ")"
		}
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func notionRichTextContent(richText map[string]any) string {
	switch stringConfig(richText["type"]) {
	case "equation":
		if equation, ok := richText["equation"].(map[string]any); ok {
			return stringConfig(equation["expression"])
		}
	case "mention":
		if mention, ok := richText["mention"].(map[string]any); ok {
			mentionType := stringConfig(mention["type"])
			if mentionValue, ok := mention[mentionType].(map[string]any); ok {
				if mentionType == "date" {
					start := stringConfig(mentionValue["start"])
					end := stringConfig(mentionValue["end"])
					if start != "" && end != "" {
						return start + " - " + end
					}
					return start
				}
				return firstNonEmpty(stringConfig(mentionValue["id"]), stringConfig(mentionValue["url"]), fmt.Sprint(mentionValue))
			}
		}
	default:
		if text := stringConfig(richText["plain_text"]); text != "" {
			return text
		}
		if textObject, ok := richText["text"].(map[string]any); ok {
			return stringConfig(textObject["content"])
		}
	}
	return ""
}

func formatNotionBlockText(blockType, text string, object map[string]any) string {
	switch blockType {
	case "bulleted_list_item":
		return "- " + text
	case "numbered_list_item":
		return "1. " + text
	case "to_do":
		prefix := "[ ]"
		if checked, ok := object["checked"].(bool); ok && checked {
			prefix = "[x]"
		}
		if text == "" {
			return prefix
		}
		return prefix + " " + text
	default:
		return text
	}
}

func notionFileMetadata(blockID string, object map[string]any) (string, string, string) {
	sourceType := stringConfig(object["type"])
	source, _ := object[sourceType].(map[string]any)
	rawURL := stringConfig(source["url"])
	name := firstNonEmpty(stringConfig(object["name"]), stringConfig(source["name"]), notionURLBase(rawURL), "notion_file_"+blockID)
	if blockID != "" {
		extension := filepath.Ext(name)
		stem := strings.TrimSuffix(name, extension)
		if stem != "" {
			name = stem + "_" + blockID + extension
		}
	}
	caption := extractNotionRichText(object["caption"])
	return rawURL, name, caption
}

func notionPropertiesToText(properties map[string]any) string {
	lines := make([]string, 0, len(properties))
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := properties[key]
		property, ok := value.(map[string]any)
		if !ok {
			lines = append(lines, fmt.Sprintf("%s: %v", key, value))
			continue
		}
		if text := notionPropertyText(property); text != "" {
			lines = append(lines, key+": "+text)
		}
	}
	return strings.Join(lines, "\n")
}

func notionBlockLabel(blockType string) string {
	if blockType == "" {
		return ""
	}
	return strings.ToUpper(blockType[:1]) + blockType[1:]
}

func notionURLBase(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Path != "" {
		return path.Base(parsed.Path)
	}
	return path.Base(rawURL)
}

func notionPropertyText(property map[string]any) string {
	propertyType := stringConfig(property["type"])
	switch propertyType {
	case "title", "rich_text":
		return extractNotionRichText(property[propertyType])
	case "select":
		if selected, ok := property["select"].(map[string]any); ok {
			return stringConfig(selected["name"])
		}
	case "multi_select":
		items, _ := property["multi_select"].([]any)
		names := make([]string, 0, len(items))
		for _, item := range items {
			if option, ok := item.(map[string]any); ok {
				names = append(names, stringConfig(option["name"]))
			}
		}
		return strings.Join(names, ", ")
	case "date":
		if date, ok := property["date"].(map[string]any); ok {
			return firstNonEmpty(stringConfig(date["start"]), stringConfig(date["end"]))
		}
	case "number", "checkbox", "url", "email", "phone_number":
		return stringConfig(property[propertyType])
	}
	return ""
}

func notionHTTPError(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("Notion credential appears to be invalid or expired (HTTP 401).")
	case http.StatusForbidden:
		return fmt.Errorf("Your Notion token does not have sufficient permissions (HTTP 403).")
	case http.StatusNotFound:
		return fmt.Errorf("Notion resource not found or not shared with the integration (HTTP 404).")
	case http.StatusTooManyRequests:
		return fmt.Errorf("Validation failed due to Notion rate-limits being exceeded (HTTP 429).")
	default:
		return fmt.Errorf("unexpected Notion HTTP error (status=%d)", statusCode)
	}
}
