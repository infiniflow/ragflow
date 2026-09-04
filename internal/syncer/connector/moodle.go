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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
)

const (
	defaultMoodleBatchSize = 2
	moodleRequestTimeout   = 60 * time.Second
	moodleMaxRedirects     = 10
	moodleRESTPath         = "/webservice/rest/server.php"
)

// Retry/backoff knobs for Moodle web service calls. They are package
// variables so tests can shrink the delays.
var (
	moodleRetryTries     = 3
	moodleRetryBaseDelay = time.Second
	moodleRetryBackoff   = 2
)
var (
	// moodleMaxResponseSize caps REST JSON responses. Package variable so
	// tests can shrink it.
	moodleMaxResponseSize int64 = 32 * 1024 * 1024
	// moodleMaxDownloadSize caps file downloads. Package variable so
	// tests can shrink it.
	moodleMaxDownloadSize int64 = 100 * 1024 * 1024
)

// MoodleConnector reads course content from a Moodle LMS through its REST web
// service API.
type MoodleConnector struct {
	moodleURL string
	token     string
	batchSize int

	// restCall is a test hook that replaces the live web service transport.
	restCall func(ctx context.Context, function string, params map[string]any, out any) error
}

// NewMoodleConnector creates a Moodle connector from connector config.
func NewMoodleConnector(config map[string]any) (*MoodleConnector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultMoodleBatchSize)
	return &MoodleConnector{
		moodleURL: strings.TrimRight(strings.TrimSpace(stringConfig(config["moodle_url"])), "/"),
		token:     strings.TrimSpace(stringConfig(credentials["moodle_token"])),
		batchSize: batchSize,
	}, nil
}

// Validate validates Moodle connector settings, credentials, and connectivity.
func (c *MoodleConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("Moodle connector is nil")
	}
	if c.moodleURL == "" {
		return fmt.Errorf("No Moodle URL was provided in connector settings.")
	}
	if c.token == "" {
		return &ConnectorMissingCredentialError{Message: "Moodle API token is required"}
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if err := validateMoodleURLForSSRF(c.moodleURL); err != nil {
		return err
	}
	var siteInfo moodleSiteInfo
	if err := c.callREST(ctx, "core_webservice_get_site_info", nil, &siteInfo); err != nil {
		return err
	}
	if siteInfo.SiteName == "" {
		return &ConnectorValidationError{Message: "Invalid Moodle API response"}
	}
	return nil
}

// ValidateConnectorSetting validates Moodle settings from an unsaved config.
func (c *MoodleConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Moodle sync session.
func (c *MoodleConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	session := &moodleSyncSession{
		connector: c,
		request:   request,
		batchSize: c.batchSize,
	}
	if request.Resume != nil {
		if err := session.applyResume(request.Resume); err != nil {
			return nil, err
		}
	}
	return session, nil
}

// OpenPrune opens one complete Moodle prune snapshot session.
func (c *MoodleConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	courses, err := c.getCourses(ctx)
	if err != nil {
		return nil, err
	}
	var documents []SlimDocument
	for _, course := range courses {
		sections, err := c.getCourseContents(ctx, course.ID)
		if err != nil {
			return nil, err
		}
		for _, section := range sections {
			for _, module := range section.Modules {
				slimID := moodleSlimDocIDForModule(module)
				if slimID == "" {
					continue
				}
				documents = append(documents, SlimDocument{SourceID: slimID})
			}
		}
	}
	return &moodlePruneSession{documents: documents, batchSize: c.batchSize}, nil
}

// ---------------------------------------------------------------------------
// Moodle REST web service client
// ---------------------------------------------------------------------------

type moodleSiteInfo struct {
	SiteName string `json:"sitename"`
}

type moodleCourse struct {
	ID        int64  `json:"id"`
	FullName  string `json:"fullname"`
	ShortName string `json:"shortname"`
}

type moodleSection struct {
	ID      int64          `json:"id"`
	Name    string         `json:"name"`
	Section *int64         `json:"section"`
	Modules []moodleModule `json:"modules"`
}

type moodleModule struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	ModName      string          `json:"modname"`
	Instance     *int64          `json:"instance"`
	Description  string          `json:"description"`
	Visible      *int64          `json:"visible"`
	GroupMode    *int64          `json:"groupmode"`
	TimeCreated  *int64          `json:"timecreated"`
	TimeModified *int64          `json:"timemodified"`
	Added        *int64          `json:"added"`
	Contents     []moodleContent `json:"contents"`
}

type moodleContent struct {
	Filename     string `json:"filename"`
	FileURL      string `json:"fileurl"`
	FileSize     *int64 `json:"filesize"`
	MIMEType     string `json:"mimetype"`
	ChapterID    *int64 `json:"chapterid"`
	Title        string `json:"title"`
	TimeCreated  *int64 `json:"timecreated"`
	TimeModified *int64 `json:"timemodified"`
}

type moodleDiscussion struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Message      string `json:"message"`
	UserID       *int64 `json:"userid"`
	UserFullName string `json:"userfullname"`
	TimeCreated  *int64 `json:"timecreated"`
	TimeModified *int64 `json:"timemodified"`
}

func (c *MoodleConnector) callREST(ctx context.Context, function string, params map[string]any, out any) error {
	if c.restCall != nil {
		return c.restCall(ctx, function, params, out)
	}
	endpoint := c.moodleURL + moodleRESTPath
	form := url.Values{}
	form.Set("wstoken", c.token)
	form.Set("wsfunction", function)
	form.Set("moodlewsrestformat", "json")
	for key, value := range params {
		switch typed := value.(type) {
		case int64:
			form.Set(key, strconv.FormatInt(typed, 10))
		case int:
			form.Set(key, strconv.Itoa(typed))
		case string:
			form.Set(key, typed)
		}
	}
	resp, err := c.moodleHTTPDo(ctx, http.MethodPost, endpoint, []byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, moodleMaxResponseSize+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > moodleMaxResponseSize {
		return fmt.Errorf("Moodle REST response exceeds maximum size of %d bytes", moodleMaxResponseSize)
	}
	if resp.StatusCode != http.StatusOK {
		return moodleRESTError(body, resp.StatusCode)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err == nil {
		if _, ok := probe["exception"]; ok {
			return moodleRESTError(body, resp.StatusCode)
		}
	}
	return json.Unmarshal(body, out)
}

func moodleRESTError(body []byte, statusCode int) error {
	var apiError struct {
		ErrorCode string `json:"errorcode"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Message != "" {
		return classifyMoodleAPIError(apiError.ErrorCode, apiError.Message)
	}
	return fmt.Errorf("Moodle web service request failed with status %d", statusCode)
}

func classifyMoodleAPIError(errorCode, message string) error {
	combined := strings.ToLower(errorCode + " " + message)
	switch {
	case strings.Contains(combined, "invalidtoken"):
		return &ConnectorMissingCredentialError{Message: "Moodle token is invalid or expired"}
	case strings.Contains(combined, "accessexception"):
		return &ConnectorValidationError{Message: "Insufficient permissions. Ensure web services are enabled and permissions are correct."}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("Moodle validation error: %s", message)}
	}
}

func retryMoodle(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < moodleRetryTries; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt+1 >= moodleRetryTries {
			break
		}
		delay := moodleRetryBaseDelay
		for i := 0; i < attempt; i++ {
			delay *= time.Duration(moodleRetryBackoff)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (c *MoodleConnector) getCourses(ctx context.Context) ([]moodleCourse, error) {
	var courses []moodleCourse
	if err := retryMoodle(ctx, func() error {
		return c.callREST(ctx, "core_course_get_courses", nil, &courses)
	}); err != nil {
		return nil, err
	}
	sort.SliceStable(courses, func(i, j int) bool {
		return courses[i].ID < courses[j].ID
	})
	return courses, nil
}

func (c *MoodleConnector) getCourseContents(ctx context.Context, courseID int64) ([]moodleSection, error) {
	var sections []moodleSection
	if err := retryMoodle(ctx, func() error {
		return c.callREST(ctx, "core_course_get_contents", map[string]any{"courseid": courseID}, &sections)
	}); err != nil {
		return nil, fmt.Errorf("Moodle course %d contents failed: %w", courseID, err)
	}
	return sections, nil
}

func (c *MoodleConnector) getForumDiscussions(ctx context.Context, forumID int64) ([]moodleDiscussion, error) {
	var result struct {
		Discussions []moodleDiscussion `json:"discussions"`
	}
	if err := c.callREST(ctx, "mod_forum_get_forum_discussions", map[string]any{"forumid": forumID}, &result); err != nil {
		return nil, err
	}
	return result.Discussions, nil
}

func (c *MoodleConnector) downloadFile(ctx context.Context, fileURL string) ([]byte, error) {
	downloadURL := addMoodleToken(fileURL, c.token)
	resp, err := c.moodleHTTPDo(ctx, http.MethodGet, downloadURL, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Moodle file download failed with status %d for %s", resp.StatusCode, moodleRedactedURL(fileURL))
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, moodleMaxDownloadSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(blob)) > moodleMaxDownloadSize {
		return nil, fmt.Errorf("Moodle file download exceeds maximum size of %d bytes", moodleMaxDownloadSize)
	}
	return blob, nil
}

func addMoodleToken(fileURL, token string) string {
	if token == "" {
		return fileURL
	}
	if strings.Contains(strings.ToLower(fileURL), "token=") {
		return fileURL
	}
	delimiter := "?"
	if strings.Contains(fileURL, "?") {
		delimiter = "&"
	}
	return fileURL + delimiter + "token=" + token
}

func moodleHTMLToMarkdown(html string) (string, error) {
	converter := md.NewConverter("", true, &md.Options{EmDelimiter: "*"})
	out, err := converter.ConvertString(html)
	if err != nil {
		return "", fmt.Errorf("Moodle HTML to Markdown conversion failed: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Document building
// ---------------------------------------------------------------------------

func (c *MoodleConnector) courseDocuments(ctx context.Context, request SyncRequest, course moodleCourse, sections []moodleSection) ([]SourceDocument, error) {
	var documents []SourceDocument
	for _, section := range sections {
		for _, module := range section.Modules {
			docs, err := c.moduleDocuments(ctx, request, course, section, module)
			if err != nil {
				return nil, err
			}
			documents = append(documents, docs...)
		}
	}
	return documents, nil
}

func (c *MoodleConnector) moduleDocuments(ctx context.Context, request SyncRequest, course moodleCourse, section moodleSection, module moodleModule) ([]SourceDocument, error) {
	var document *SourceDocument
	var err error
	switch module.ModName {
	case "label", "url":
		return nil, nil
	case "resource":
		document, err = c.resourceDocument(ctx, request, course, section, module)
	case "page":
		document, err = c.pageDocument(ctx, request, course, section, module)
	case "forum":
		document, err = c.forumDocument(ctx, request, course, section, module)
	case "assign", "quiz":
		document, err = c.activityDocument(ctx, request, course, section, module)
	case "book":
		document, err = c.bookDocument(ctx, request, course, section, module)
	}
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, nil
	}
	return []SourceDocument{*document}, nil
}

func (c *MoodleConnector) resourceDocument(ctx context.Context, request SyncRequest, course moodleCourse, section moodleSection, module moodleModule) (*SourceDocument, error) {
	if len(module.Contents) == 0 || module.Contents[0].FileURL == "" {
		return nil, nil
	}
	fileInfo := module.Contents[0]
	fileName := path.Base(fileInfo.Filename)
	ts := moodleMaxTimestamp(module.TimeCreated, module.TimeModified, fileInfo.TimeModified)
	if !includeMoodleModule(request, ts) {
		return nil, nil
	}
	blob, err := c.downloadFile(ctx, fileInfo.FileURL)
	if err != nil {
		return nil, err
	}
	extension := filepath.Ext(fileName)
	if extension == "" {
		extension = ".bin"
	}
	fileSize := int64(len(blob))
	if fileInfo.FileSize != nil {
		fileSize = *fileInfo.FileSize
	}
	return &SourceDocument{
		SourceID:           fmt.Sprintf("moodle_resource_%d", module.ID),
		SemanticIdentifier: fmt.Sprintf("%s / %s / %s", course.FullName, section.Name, fileName),
		Extension:          extension,
		Blob:               blob,
		UpdatedAt:          moodleTime(ts),
		SizeBytes:          int64(len(blob)),
		Metadata:           c.fileModuleMetadata(course, section, module, fileInfo, fileSize, fileName),
	}, nil
}

func (c *MoodleConnector) pageDocument(ctx context.Context, request SyncRequest, course moodleCourse, section moodleSection, module moodleModule) (*SourceDocument, error) {
	if len(module.Contents) == 0 || module.Contents[0].FileURL == "" {
		return nil, nil
	}
	fileInfo := module.Contents[0]
	fileName := path.Base(fileInfo.Filename)
	ts := moodleMaxTimestamp(module.TimeCreated, module.TimeModified, fileInfo.TimeModified)
	if !includeMoodleModule(request, ts) {
		return nil, nil
	}
	blob, err := c.downloadFile(ctx, fileInfo.FileURL)
	if err != nil {
		return nil, err
	}
	extension := filepath.Ext(fileName)
	if extension == "" {
		extension = ".html"
	}
	fileSize := int64(len(blob))
	if fileInfo.FileSize != nil {
		fileSize = *fileInfo.FileSize
	}
	return &SourceDocument{
		SourceID:           fmt.Sprintf("moodle_page_%d", module.ID),
		SemanticIdentifier: fmt.Sprintf("%s / %s / %s", course.FullName, section.Name, module.Name),
		Extension:          extension,
		Blob:               blob,
		UpdatedAt:          moodleTime(ts),
		SizeBytes:          int64(len(blob)),
		Metadata:           c.fileModuleMetadata(course, section, module, fileInfo, fileSize, fileName),
	}, nil
}

func (c *MoodleConnector) forumDocument(ctx context.Context, request SyncRequest, course moodleCourse, section moodleSection, module moodleModule) (*SourceDocument, error) {
	if module.Instance == nil {
		return nil, nil
	}
	if !includeMoodleModule(request, module.windowTimestamp()) {
		return nil, nil
	}
	discussions, err := c.getForumDiscussions(ctx, *module.Instance)
	if err != nil {
		return nil, err
	}
	if len(discussions) == 0 {
		return nil, nil
	}
	markdown := []string{"# " + module.Name + "\n"}
	latest := moodleMaxTimestamp(module.TimeCreated, module.TimeModified)
	discussionMetadata := make([]map[string]any, 0, len(discussions))
	for _, discussion := range discussions {
		body, err := moodleHTMLToMarkdown(discussion.Message)
		if err != nil {
			return nil, err
		}
		markdown = append(markdown, fmt.Sprintf("## %s\n\n%s\n\n---\n", discussion.Name, body))
		latest = maxInt64(latest, derefInt64(discussion.TimeModified))
		discussionMetadata = append(discussionMetadata, map[string]any{
			"id":            discussion.ID,
			"name":          discussion.Name,
			"user_id":       discussion.UserID,
			"user_fullname": discussion.UserFullName,
			"time_created":  discussion.TimeCreated,
			"time_modified": discussion.TimeModified,
		})
	}
	metadata := moodleBaseMetadata(c, course, section, module)
	metadata["forum_id"] = module.Instance
	metadata["discussion_count"] = len(discussions)
	metadata["discussions"] = discussionMetadata
	blob := []byte(strings.Join(markdown, "\n"))
	return &SourceDocument{
		SourceID:           fmt.Sprintf("moodle_forum_%d", module.ID),
		SemanticIdentifier: fmt.Sprintf("%s / %s / %s", course.FullName, section.Name, module.Name),
		Extension:          ".md",
		Blob:               blob,
		UpdatedAt:          moodleTime(latest),
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
	}, nil
}

func (c *MoodleConnector) activityDocument(ctx context.Context, request SyncRequest, course moodleCourse, section moodleSection, module moodleModule) (*SourceDocument, error) {
	if module.Description == "" {
		return nil, nil
	}
	if !includeMoodleModule(request, module.windowTimestamp()) {
		return nil, nil
	}
	body, err := moodleHTMLToMarkdown(module.Description)
	if err != nil {
		return nil, err
	}
	markdown := fmt.Sprintf("# %s\n\n**Type:** %s\n\n%s", module.Name, capitalizeMoodleType(module.ModName), body)
	ts := moodleMaxTimestamp(module.TimeCreated, module.TimeModified, module.Added)
	metadata := moodleBaseMetadata(c, course, section, module)
	metadata["activity_type"] = module.ModName
	metadata["activity_instance"] = module.Instance
	metadata["description"] = module.Description
	metadata["added"] = module.Added
	blob := []byte(markdown)
	return &SourceDocument{
		SourceID:           fmt.Sprintf("moodle_%s_%d", module.ModName, module.ID),
		SemanticIdentifier: fmt.Sprintf("%s / %s / %s", course.FullName, section.Name, module.Name),
		Extension:          ".md",
		Blob:               blob,
		UpdatedAt:          moodleTime(ts),
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
	}, nil
}

func (c *MoodleConnector) bookDocument(ctx context.Context, request SyncRequest, course moodleCourse, section moodleSection, module moodleModule) (*SourceDocument, error) {
	if len(module.Contents) == 0 {
		return nil, nil
	}
	var chapters []moodleContent
	for _, content := range module.Contents {
		if content.FileURL != "" && path.Base(content.Filename) == "index.html" {
			chapters = append(chapters, content)
		}
	}
	if len(chapters) == 0 {
		return nil, nil
	}
	latest := moodleMaxTimestamp(module.TimeCreated, module.TimeModified)
	for _, content := range module.Contents {
		latest = maxInt64(latest, derefInt64(content.TimeCreated))
		latest = maxInt64(latest, derefInt64(content.TimeModified))
	}
	if !includeMoodleModule(request, latest) {
		return nil, nil
	}
	markdownParts := []string{"# " + module.Name + "\n"}
	chapterMetadata := make([]map[string]any, 0, len(chapters))
	for _, chapter := range chapters {
		blob, err := c.downloadFile(ctx, chapter.FileURL)
		if err != nil {
			return nil, err
		}
		body, err := moodleHTMLToMarkdown(string(blob))
		if err != nil {
			return nil, err
		}
		markdownParts = append(markdownParts, body+"\n\n---\n")
		chapterMetadata = append(chapterMetadata, map[string]any{
			"chapter_id":    chapter.ChapterID,
			"title":         chapter.Title,
			"filename":      chapter.Filename,
			"fileurl":       moodleRedactedURL(chapter.FileURL),
			"time_created":  chapter.TimeCreated,
			"time_modified": chapter.TimeModified,
			"size":          chapter.FileSize,
		})
	}
	metadata := moodleBaseMetadata(c, course, section, module)
	metadata["book_id"] = module.Instance
	metadata["chapter_count"] = len(chapters)
	metadata["chapters"] = chapterMetadata
	blob := []byte(strings.Join(markdownParts, "\n"))
	return &SourceDocument{
		SourceID:           fmt.Sprintf("moodle_book_%d", module.ID),
		SemanticIdentifier: fmt.Sprintf("%s / %s / %s", course.FullName, section.Name, module.Name),
		Extension:          ".md",
		Blob:               blob,
		UpdatedAt:          moodleTime(latest),
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
	}, nil
}

func (c *MoodleConnector) fileModuleMetadata(course moodleCourse, section moodleSection, module moodleModule, fileInfo moodleContent, fileSize int64, fileName string) map[string]any {
	metadata := moodleBaseMetadata(c, course, section, module)
	metadata["module_instance"] = module.Instance
	metadata["file_url"] = moodleRedactedURL(fileInfo.FileURL)
	metadata["file_name"] = fileName
	metadata["file_size"] = fileSize
	metadata["file_type"] = fileInfo.MIMEType
	return metadata
}

func moodleBaseMetadata(c *MoodleConnector, course moodleCourse, section moodleSection, module moodleModule) map[string]any {
	return map[string]any{
		"moodle_url":       c.moodleURL,
		"course_id":        course.ID,
		"course_name":      course.FullName,
		"course_shortname": course.ShortName,
		"section_id":       section.ID,
		"section_name":     section.Name,
		"section_number":   section.Section,
		"module_id":        module.ID,
		"module_name":      module.Name,
		"module_type":      module.ModName,
		"time_created":     module.TimeCreated,
		"time_modified":    module.TimeModified,
		"visible":          module.Visible,
		"groupmode":        module.GroupMode,
	}
}

// ---------------------------------------------------------------------------
// Sync session with checkpoint resume
// ---------------------------------------------------------------------------

type moodleSyncSession struct {
	connector           *MoodleConnector
	request             SyncRequest
	batchSize           int
	resumeAfterCourseID int64
	resumeCourseSet     bool
	resumeCourseChecked bool

	coursesLoaded     bool
	courses           []moodleCourse
	courseIndex       int
	pending           []SourceDocument
	pendingCheckpoint *SyncCheckpoint
	done              bool
}

// NextBatch returns the next Moodle document batch.
func (s *moodleSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	for {
		if s.done {
			return SyncBatch{}, io.EOF
		}
		if len(s.pending) == 0 {
			if !s.coursesLoaded {
				courses, err := s.connector.getCourses(ctx)
				if err != nil {
					return SyncBatch{}, err
				}
				s.courses = courses
				s.coursesLoaded = true
				if s.resumeCourseSet && !s.resumeCourseChecked {
					found := false
					for _, course := range s.courses {
						if course.ID == s.resumeAfterCourseID {
							found = true
							break
						}
					}
					if !found {
						return SyncBatch{}, fmt.Errorf("moodle resume course %d was not found in the current listing: %w", s.resumeAfterCourseID, ErrSyncResumeInvalid)
					}
					s.resumeCourseChecked = true
				}
			}
			if s.courseIndex >= len(s.courses) {
				s.done = true
				return SyncBatch{}, io.EOF
			}
			course := s.courses[s.courseIndex]
			s.courseIndex++
			if course.ID <= s.resumeAfterCourseID {
				continue
			}
			sections, err := s.connector.getCourseContents(ctx, course.ID)
			if err != nil {
				return SyncBatch{}, err
			}
			documents, err := s.connector.courseDocuments(ctx, s.request, course, sections)
			if err != nil {
				return SyncBatch{}, err
			}
			if len(documents) == 0 {
				continue
			}
			s.pending = documents
			updatedAt := documents[len(documents)-1].UpdatedAt
			checkpoint := &SyncCheckpoint{
				Cursor:    fmt.Sprintf("moodle_course_%d", course.ID),
				SourceID:  fmt.Sprintf("moodle_course_%d", course.ID),
				UpdatedAt: &updatedAt,
			}
			// The checkpoint only advances once the whole course is flushed so
			// an interrupted run re-processes a partially committed course.
			s.pendingCheckpoint = checkpoint
		}
		n := s.batchSize
		if n > len(s.pending) {
			n = len(s.pending)
		}
		chunk := s.pending[:n]
		s.pending = s.pending[n:]
		var checkpoint *SyncCheckpoint
		if len(s.pending) == 0 && s.pendingCheckpoint != nil {
			checkpoint = s.pendingCheckpoint
			s.pendingCheckpoint = nil
		}
		return SyncBatch{Documents: chunk, Checkpoint: checkpoint}, nil
	}
}

// Close closes the Moodle sync session.
func (s *moodleSyncSession) Close() error {
	return nil
}

func (s *moodleSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	const prefix = "moodle_course_"
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("moodle sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	courseID, err := strconv.ParseInt(strings.TrimPrefix(sourceID, prefix), 10, 64)
	if err != nil {
		return fmt.Errorf("moodle sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	s.resumeAfterCourseID = courseID
	s.resumeCourseSet = true
	return nil
}

type moodlePruneSession struct {
	documents  []SlimDocument
	batchSize  int
	batchIndex int
}

// NextBatch returns the next Moodle prune snapshot batch.
func (s *moodlePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.batchIndex >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	end := s.batchIndex + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	documents := s.documents[s.batchIndex:end]
	s.batchIndex = end
	return PruneBatch{Documents: documents}, nil
}

// Close closes the Moodle prune session.
func (s *moodlePruneSession) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func includeMoodleModule(request SyncRequest, ts int64) bool {
	if request.FromBeginning {
		return true
	}
	if ts <= 0 {
		return false
	}
	updatedAt := time.Unix(ts, 0).UTC()
	return !beforeOrAtWindowStart(updatedAt, request.WindowStart) && !afterWindowEnd(updatedAt, request.WindowEnd)
}

func moodleSlimDocIDForModule(module moodleModule) string {
	switch module.ModName {
	case "label", "url":
		return ""
	case "resource":
		return fmt.Sprintf("moodle_resource_%d", module.ID)
	case "forum":
		return fmt.Sprintf("moodle_forum_%d", module.ID)
	case "page":
		return fmt.Sprintf("moodle_page_%d", module.ID)
	case "book":
		return fmt.Sprintf("moodle_book_%d", module.ID)
	case "assign", "quiz":
		return fmt.Sprintf("moodle_%s_%d", module.ModName, module.ID)
	default:
		return ""
	}
}

func (m moodleModule) windowTimestamp() int64 {
	ts := moodleMaxTimestamp(m.TimeCreated, m.TimeModified)
	for _, content := range m.Contents {
		ts = maxInt64(ts, derefInt64(content.TimeModified))
	}
	return ts
}

func moodleMaxTimestamp(values ...*int64) int64 {
	var latest int64
	for _, value := range values {
		if value != nil && *value > latest {
			latest = *value
		}
	}
	return latest
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func moodleTime(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	return time.Unix(ts, 0).UTC()
}

func capitalizeMoodleType(mtype string) string {
	if mtype == "" {
		return mtype
	}
	return strings.ToUpper(mtype[:1]) + strings.ToLower(mtype[1:])
}

// ---------------------------------------------------------------------------
// SSRF protection
// ---------------------------------------------------------------------------

func validateMoodleURLForSSRF(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &ConnectorValidationError{Message: "Moodle URL must include a hostname."}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &ConnectorValidationError{Message: fmt.Sprintf("Unsupported URL scheme for Moodle connector: %q. Only http/https are allowed.", parsed.Scheme)}
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return &ConnectorValidationError{Message: "Moodle URL must include a hostname."}
	}
	if strings.EqualFold(hostname, "localhost") {
		return &ConnectorValidationError{Message: fmt.Sprintf("Moodle URL hostname %q is not allowed (localhost is blocked).", hostname)}
	}
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		// Resolution failure is not an SSRF condition by itself; the
		// per-request check surfaces it if it matters.
		return nil
	}
	if restAPISSRFAllowLoopback {
		allLoopback := true
		for _, addr := range addrs {
			if !addr.IsLoopback() {
				allLoopback = false
				break
			}
		}
		if allLoopback {
			return nil
		}
		// Not all loopback — fall through to normal validation.
	}
	for _, addr := range addrs {
		if !restAPIIPIsGlobal(restAPIEffectiveIP(addr)) {
			return &ConnectorValidationError{Message: fmt.Sprintf(
				"Moodle URL %q resolves to disallowed address %s (localhost, private, link-local, reserved, or multicast addresses are blocked).",
				rawURL, addr)}
		}
	}
	return nil
}

// moodleAssertURLSafe validates a per-request URL for SSRF, HTTPS, and
// same-origin policy. It returns the hostname and first validated IP so the
// caller can pin DNS for the actual dial.
func moodleAssertURLSafe(ctx context.Context, rawURL, originURL string) (string, net.IP, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("Moodle URL is missing a host.")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, fmt.Errorf("Disallowed URL scheme: %q. Only [http https] are allowed.", parsed.Scheme)
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", nil, fmt.Errorf("Moodle URL is missing a host.")
	}
	// Reject cross-origin targets: every request must go to the configured
	// Moodle host.
	if originURL != "" {
		if origin, err := url.Parse(originURL); err == nil && origin.Hostname() != "" {
			if !strings.EqualFold(hostname, origin.Hostname()) {
				return "", nil, fmt.Errorf("Moodle URL host %q does not match configured origin %q.", hostname, origin.Hostname())
			}
		}
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return "", nil, fmt.Errorf("Could not resolve hostname %q: %w", hostname, err)
	}
	if len(addrs) == 0 {
		return "", nil, fmt.Errorf("Hostname %q resolved to no addresses.", hostname)
	}
	if restAPISSRFAllowLoopback {
		allLoopback := true
		for _, addr := range addrs {
			if !addr.IP.IsLoopback() {
				allLoopback = false
				break
			}
		}
		if allLoopback {
			return hostname, addrs[0].IP, nil
		}
		// Not all loopback — fall through to normal validation.
	}
	var first net.IP
	for _, addr := range addrs {
		if !restAPIIPIsGlobal(restAPIEffectiveIP(addr.IP)) {
			return "", nil, fmt.Errorf("Moodle URL resolves to a non-public address (%s), which is not allowed.", addr.IP)
		}
		if first == nil {
			first = addr.IP
		}
	}
	if first == nil {
		return "", nil, fmt.Errorf("Hostname %q resolved to no addresses.", hostname)
	}
	return hostname, first, nil
}

// moodleHTTPDo sends an HTTP request with DNS-pinned transport and manual
// redirect handling. Each hop is independently validated for SSRF, HTTPS, and
// same-origin policy, preventing DNS rebinding and redirect-based bypasses.
func (c *MoodleConnector) moodleHTTPDo(ctx context.Context, method, rawURL string, body []byte, headers map[string]string) (*http.Response, error) {
	currentURL := rawURL
	currentMethod := method
	currentBody := body
	for hop := 0; hop <= moodleMaxRedirects; hop++ {
		hostname, pinIP, err := moodleAssertURLSafe(ctx, currentURL, c.moodleURL)
		if err != nil {
			return nil, err
		}
		transport := newRestAPIPinnedTransport(hostname, pinIP)
		client := &http.Client{
			Transport: transport,
			Timeout:   moodleRequestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		var bodyReader io.Reader
		if currentBody != nil {
			bodyReader = bytes.NewReader(currentBody)
		}
		req, err := http.NewRequestWithContext(ctx, currentMethod, currentURL, bodyReader)
		if err != nil {
			transport.CloseIdleConnections()
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			transport.CloseIdleConnections()
			return nil, err
		}
		if !restAPIIsRedirect(resp.StatusCode) {
			resp.Body = &restAPICloseIdleBody{body: resp.Body, transport: transport}
			return resp, nil
		}
		location := resp.Header.Get("Location")
		resp.Body.Close()
		transport.CloseIdleConnections()
		if location == "" {
			return nil, fmt.Errorf("Moodle redirect with empty Location header")
		}
		nextURL, err := restAPIResolveURL(currentURL, location)
		if err != nil {
			return nil, err
		}
		currentURL = nextURL
		if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
			currentMethod = http.MethodGet
			currentBody = nil
		}
	}
	return nil, fmt.Errorf("Moodle request stopped after %d redirects", moodleMaxRedirects)
}

// moodleRedactedURL strips query and fragment from a URL so that tokens or
// other sensitive query parameters are not persisted in document metadata.
func moodleRedactedURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
