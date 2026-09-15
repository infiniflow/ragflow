package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func withMoodleTestHooks(t *testing.T) {
	t.Helper()
	origLoopback := restAPISSRFAllowLoopback
	origTries := moodleRetryTries
	origBaseDelay := moodleRetryBaseDelay
	origBackoff := moodleRetryBackoff
	restAPISSRFAllowLoopback = true
	moodleRetryTries = 2
	moodleRetryBaseDelay = time.Millisecond
	moodleRetryBackoff = 2
	t.Cleanup(func() {
		restAPISSRFAllowLoopback = origLoopback
		moodleRetryTries = origTries
		moodleRetryBaseDelay = origBaseDelay
		moodleRetryBackoff = origBackoff
	})
}

func mustMoodleConnector(t *testing.T, moodleURL string) *MoodleConnector {
	t.Helper()
	connector, err := NewMoodleConnector(map[string]any{
		"moodle_url": moodleURL,
		"batch_size": 2,
		"credentials": map[string]any{
			"moodle_token": "testtoken",
		},
	})
	if err != nil {
		t.Fatalf("NewMoodleConnector: %v", err)
	}
	return connector
}

type moodleTestFixtures struct {
	courses          string
	contents         map[string]string
	forumDiscussions map[string]string
	files            map[string]string
	siteInfoBody     string
	courseListCount  *atomic.Int64
	forumCallCount   *atomic.Int64
}

func newTestMoodleServer(t *testing.T, buildFixtures func(serverURL string) moodleTestFixtures) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	fixtures := buildFixtures(server.URL)
	siteInfoBody := fixtures.siteInfoBody
	if siteInfoBody == "" {
		siteInfoBody = `{"sitename":"Test Moodle","siteurl":"https://example.com"}`
	}
	mux.HandleFunc("/webservice/rest/server.php", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		switch r.Form.Get("wsfunction") {
		case "core_webservice_get_site_info":
			w.Write([]byte(siteInfoBody))
		case "core_course_get_courses":
			if fixtures.courseListCount != nil {
				fixtures.courseListCount.Add(1)
			}
			w.Write([]byte(fixtures.courses))
		case "core_course_get_contents":
			body, ok := fixtures.contents[r.Form.Get("courseid")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(body))
		case "mod_forum_get_forum_discussions":
			if fixtures.forumCallCount != nil {
				fixtures.forumCallCount.Add(1)
			}
			body, ok := fixtures.forumDiscussions[r.Form.Get("forumid")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(body))
		default:
			http.Error(w, "unknown function", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/pluginfile.php/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "testtoken" {
			http.Error(w, "missing token", http.StatusForbidden)
			return
		}
		body, ok := fixtures.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	})
	return server
}

func fullSyncMoodleFixtures(serverURL string) moodleTestFixtures {
	return moodleTestFixtures{
		courses: `[{"id":1,"fullname":"Course One","shortname":"c1"},{"id":2,"fullname":"Course Two","shortname":"c2"}]`,
		contents: map[string]string{
			"1": `[{"id":11,"name":"Week 1","section":0,"modules":[` +
				`{"id":100,"name":"Guide","modname":"resource","instance":7,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[{"type":"file","filename":"guide.pdf","filepath":"/","filesize":123,"fileurl":"` + serverURL + `/pluginfile.php/guides/guide.pdf","mimetype":"application/pdf","timemodified":1700000000}]},` +
				`{"id":101,"name":"About","modname":"page","instance":8,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[{"type":"file","filename":"about.html","filepath":"/","filesize":456,"fileurl":"` + serverURL + `/pluginfile.php/pages/about.html","mimetype":"text/html","timemodified":1700000000}]},` +
				`{"id":102,"name":"Announcement Label","modname":"label","instance":9,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[]},` +
				`{"id":103,"name":"External Link","modname":"url","instance":10,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[]}` +
				`]}]`,
			"2": `[{"id":12,"name":"Week 2","section":1,"modules":[` +
				`{"id":200,"name":"Announcements","modname":"forum","instance":50,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[]},` +
				`{"id":201,"name":"Assignment 1","modname":"assign","instance":51,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"added":1600000000,"description":"<p>Write a <em>report</em>.</p>","contents":[]},` +
				`{"id":202,"name":"Quiz 1","modname":"quiz","instance":52,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"added":1600000000,"description":"<p>Answer the questions.</p>","contents":[]},` +
				`{"id":203,"name":"Book Title","modname":"book","instance":53,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[` +
				`{"type":"file","filename":"index.html","filepath":"/chapter1/","filesize":100,"fileurl":"` + serverURL + `/pluginfile.php/books/chapter1/index.html","chapterid":1001,"title":"Chapter 1","timecreated":1000000000,"timemodified":1700000000},` +
				`{"type":"file","filename":"index.html","filepath":"/chapter2/","filesize":100,"fileurl":"` + serverURL + `/pluginfile.php/books/chapter2/index.html","chapterid":1002,"title":"Chapter 2","timecreated":1000000000,"timemodified":1700000000},` +
				`{"type":"file","filename":"cover.png","filepath":"/","filesize":50,"fileurl":"` + serverURL + `/pluginfile.php/books/cover.png"}` +
				`]}` +
				`]}]`,
		},
		forumDiscussions: map[string]string{
			"50": `{"discussions":[{"id":10,"name":"Intro","message":"<p>Hello <strong>world</strong></p>","userid":5,"userfullname":"Alice","timecreated":100,"timemodified":200}]}`,
		},
		files: map[string]string{
			"/pluginfile.php/guides/guide.pdf":          "%PDF-1.4 fake",
			"/pluginfile.php/pages/about.html":          "<html><body><h1>About</h1><p>Hello</p></body></html>",
			"/pluginfile.php/books/chapter1/index.html": "<h1>Chapter 1</h1><p>First</p>",
			"/pluginfile.php/books/chapter2/index.html": "<h1>Chapter 2</h1><p>Second</p>",
		},
	}
}

func collectMoodleDocuments(t *testing.T, session SyncSession) []SourceDocument {
	t.Helper()
	var documents []SourceDocument
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			return documents
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		documents = append(documents, batch.Documents...)
	}
}

func TestMoodleConnectorValidate(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestMoodleConnectorValidateMissingURLAndToken(t *testing.T) {
	withMoodleTestHooks(t)
	connector, err := NewMoodleConnector(map[string]any{"credentials": map[string]any{}})
	if err != nil {
		t.Fatalf("NewMoodleConnector: %v", err)
	}
	if err := connector.Validate(context.Background()); err == nil {
		t.Fatalf("expected missing URL validation error")
	}
	connector.moodleURL = "https://example.com"
	err = connector.Validate(context.Background())
	if err == nil {
		t.Fatalf("expected missing token validation error")
	}
	var missingCred *ConnectorMissingCredentialError
	if !errors.As(err, &missingCred) {
		t.Fatalf("missing token error type = %T, want ConnectorMissingCredentialError", err)
	}
}

func TestMoodleConnectorValidateInvalidToken(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, func(serverURL string) moodleTestFixtures {
		fixtures := fullSyncMoodleFixtures(serverURL)
		fixtures.siteInfoBody = `{"exception":"moodle_exception","errorcode":"invalidtoken","message":"Invalid token - token has expired or is invalid"}`
		return fixtures
	})
	connector := mustMoodleConnector(t, server.URL)
	err := connector.Validate(context.Background())
	if err == nil {
		t.Fatalf("expected invalid token validation error")
	}
	var missingCred *ConnectorMissingCredentialError
	if !errors.As(err, &missingCred) {
		t.Fatalf("invalid token error type = %T, want ConnectorMissingCredentialError", err)
	}
}

func TestMoodleConnectorOpenSyncBuildsAllModuleTypes(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := collectMoodleDocuments(t, session)
	if len(documents) != 6 {
		t.Fatalf("documents len = %d, want 6", len(documents))
	}
	byID := map[string]SourceDocument{}
	for _, document := range documents {
		byID[document.SourceID] = document
	}
	for _, sourceID := range []string{
		"moodle_resource_100", "moodle_page_101", "moodle_forum_200",
		"moodle_assign_201", "moodle_quiz_202", "moodle_book_203",
	} {
		if _, ok := byID[sourceID]; !ok {
			t.Fatalf("missing document %q", sourceID)
		}
	}
	resource := byID["moodle_resource_100"]
	if resource.SemanticIdentifier != "Course One / Week 1 / guide.pdf" {
		t.Fatalf("resource semantic = %q", resource.SemanticIdentifier)
	}
	if resource.Extension != ".pdf" {
		t.Fatalf("resource extension = %q", resource.Extension)
	}
	if string(resource.Blob) != "%PDF-1.4 fake" {
		t.Fatalf("resource blob = %q", resource.Blob)
	}
	if resource.Metadata["file_name"] != "guide.pdf" || resource.Metadata["file_size"] != int64(123) {
		t.Fatalf("resource metadata = %+v", resource.Metadata)
	}
	if resource.Metadata["course_name"] != "Course One" || resource.Metadata["section_name"] != "Week 1" {
		t.Fatalf("resource metadata = %+v", resource.Metadata)
	}

	page := byID["moodle_page_101"]
	if page.Extension != ".html" || string(page.Blob) != "<html><body><h1>About</h1><p>Hello</p></body></html>" {
		t.Fatalf("page = %+v", page)
	}

	forum := byID["moodle_forum_200"]
	if forum.Extension != ".md" {
		t.Fatalf("forum extension = %q", forum.Extension)
	}
	forumBlob := string(forum.Blob)
	if !strings.Contains(forumBlob, "# Announcements") || !strings.Contains(forumBlob, "## Intro") || !strings.Contains(forumBlob, "Hello **world**") {
		t.Fatalf("forum blob = %q", forumBlob)
	}
	if forum.Metadata["discussion_count"] != 1 {
		t.Fatalf("forum metadata = %+v", forum.Metadata)
	}
	forumID, ok := forum.Metadata["forum_id"].(*int64)
	if !ok || forumID == nil || *forumID != 50 {
		t.Fatalf("forum metadata = %+v", forum.Metadata)
	}

	assign := byID["moodle_assign_201"]
	if !strings.Contains(string(assign.Blob), "**Type:** Assign") || !strings.Contains(string(assign.Blob), "Write a *report*.") {
		t.Fatalf("assign blob = %q", assign.Blob)
	}
	if assign.Metadata["description"] != "<p>Write a <em>report</em>.</p>" {
		t.Fatalf("assign metadata = %+v", assign.Metadata)
	}

	quiz := byID["moodle_quiz_202"]
	if !strings.Contains(string(quiz.Blob), "**Type:** Quiz") {
		t.Fatalf("quiz blob = %q", quiz.Blob)
	}

	book := byID["moodle_book_203"]
	bookBlob := string(book.Blob)
	if !strings.Contains(bookBlob, "# Book Title") || !strings.Contains(bookBlob, "# Chapter 1") || !strings.Contains(bookBlob, "# Chapter 2") {
		t.Fatalf("book blob = %q", bookBlob)
	}
	if book.Metadata["chapter_count"] != 2 {
		t.Fatalf("book metadata = %+v", book.Metadata)
	}

	if _, ok := byID["moodle_label_102"]; ok {
		t.Fatalf("label module must be skipped")
	}
	if _, ok := byID["moodle_url_103"]; ok {
		t.Fatalf("url module must be skipped")
	}
}

func TestMoodleConnectorOpenSyncWindowFilter(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, func(serverURL string) moodleTestFixtures {
		return moodleTestFixtures{
			courses: `[{"id":1,"fullname":"Course One","shortname":"c1"}]`,
			contents: map[string]string{
				"1": `[{"id":11,"name":"Week 1","section":0,"modules":[` +
					`{"id":100,"name":"Before","modname":"resource","instance":7,"visible":1,"groupmode":0,"timemodified":1704067200,"contents":[{"type":"file","filename":"before.pdf","filepath":"/","fileurl":"` + serverURL + `/pluginfile.php/before.pdf","timemodified":1704067200}]},` +
					`{"id":101,"name":"Inside","modname":"resource","instance":8,"visible":1,"groupmode":0,"timemodified":1704153600,"contents":[{"type":"file","filename":"inside.pdf","filepath":"/","fileurl":"` + serverURL + `/pluginfile.php/inside.pdf","timemodified":1704153600}]},` +
					`{"id":102,"name":"After","modname":"resource","instance":9,"visible":1,"groupmode":0,"timemodified":1709251200,"contents":[{"type":"file","filename":"after.pdf","filepath":"/","fileurl":"` + serverURL + `/pluginfile.php/after.pdf","timemodified":1709251200}]}` +
					`]}]`,
			},
			files: map[string]string{
				"/pluginfile.php/before.pdf": "before",
				"/pluginfile.php/inside.pdf": "inside",
				"/pluginfile.php/after.pdf":  "after",
			},
		}
	})
	connector := mustMoodleConnector(t, server.URL)
	start := mustTime(t, "2024-01-01T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2024-02-01T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := collectMoodleDocuments(t, session)
	if len(documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(documents))
	}
	if documents[0].SourceID != "moodle_resource_101" {
		t.Fatalf("window filtered source id = %q, want moodle_resource_101", documents[0].SourceID)
	}
}

func TestMoodleConnectorOpenSyncResumesFromCheckpoint(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	updatedAt := mustTime(t, "2023-11-14T22:13:20Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:    "moodle_course_1",
			SourceID:  "moodle_course_1",
			UpdatedAt: &updatedAt,
		},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := collectMoodleDocuments(t, session)
	if len(documents) != 4 {
		t.Fatalf("documents len = %d, want 4 (course 1 skipped)", len(documents))
	}
	for _, document := range documents {
		if strings.HasPrefix(document.SourceID, "moodle_resource_") || strings.HasPrefix(document.SourceID, "moodle_page_") {
			t.Fatalf("resume must skip course 1, got %q", document.SourceID)
		}
	}
}

func TestMoodleConnectorOpenSyncResumeRejectsMissingCourse(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   "moodle_course_999",
			SourceID: "moodle_course_999",
		},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestMoodleConnectorOpenSyncResumeRejectsMalformedCursor(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor: "moodle_course_not-a-number",
		},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestMoodleConnectorOpenSyncDeferredCoursesUntilNextBatch(t *testing.T) {
	withMoodleTestHooks(t)
	var courseListCount atomic.Int64
	server := newTestMoodleServer(t, func(serverURL string) moodleTestFixtures {
		fixtures := fullSyncMoodleFixtures(serverURL)
		fixtures.courseListCount = &courseListCount
		return fixtures
	})
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	if courseListCount.Load() != 0 {
		t.Fatalf("course list calls after OpenSync = %d, want 0", courseListCount.Load())
	}
	if _, err := session.NextBatch(context.Background()); err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if courseListCount.Load() != 1 {
		t.Fatalf("course list calls after NextBatch = %d, want 1", courseListCount.Load())
	}
}

func TestMoodleConnectorOpenPruneReturnsSlimSnapshot(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	expected := map[string]bool{
		"moodle_resource_100": false,
		"moodle_page_101":     false,
		"moodle_forum_200":    false,
		"moodle_assign_201":   false,
		"moodle_quiz_202":     false,
		"moodle_book_203":     false,
	}
	var total int
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		for _, document := range batch.Documents {
			total++
			if _, ok := expected[document.SourceID]; !ok {
				t.Fatalf("unexpected slim document %q", document.SourceID)
			}
			expected[document.SourceID] = true
		}
	}
	if total != 6 {
		t.Fatalf("slim documents total = %d, want 6", total)
	}
	for sourceID, seen := range expected {
		if !seen {
			t.Fatalf("missing slim document %q", sourceID)
		}
	}
}

func TestMoodleConnectorOpenSyncCheckpointAdvancesPerCourse(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	var checkpoints []string
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		if batch.Checkpoint != nil {
			checkpoints = append(checkpoints, batch.Checkpoint.Cursor)
		}
	}
	// Course 1 has one batch (2 docs) and course 2 has two batches (4 docs);
	// the checkpoint advances only on the last batch of each course.
	if len(checkpoints) != 2 || checkpoints[0] != "moodle_course_1" || checkpoints[1] != "moodle_course_2" {
		t.Fatalf("checkpoints = %v, want [moodle_course_1 moodle_course_2]", checkpoints)
	}
}

func TestMoodleHTMLToMarkdown(t *testing.T) {
	out, err := moodleHTMLToMarkdown("<p>Hello <strong>world</strong> and <em>you</em></p>")
	if err != nil {
		t.Fatalf("moodleHTMLToMarkdown: %v", err)
	}
	if !strings.Contains(out, "**world**") || !strings.Contains(out, "*you*") {
		t.Fatalf("markdown = %q", out)
	}
	out, err = moodleHTMLToMarkdown("<h1>Title</h1><p>Body</p>")
	if err != nil {
		t.Fatalf("moodleHTMLToMarkdown: %v", err)
	}
	if !strings.Contains(out, "# Title") || !strings.Contains(out, "Body") {
		t.Fatalf("markdown = %q", out)
	}
}

func TestAddMoodleToken(t *testing.T) {
	if got := addMoodleToken("https://example.com/file.pdf", "tok"); got != "https://example.com/file.pdf?token=tok" {
		t.Fatalf("plain url = %q", got)
	}
	if got := addMoodleToken("https://example.com/file.pdf?x=1", "tok"); got != "https://example.com/file.pdf?x=1&token=tok" {
		t.Fatalf("query url = %q", got)
	}
	if got := addMoodleToken("https://example.com/file.pdf?token=old", "tok"); got != "https://example.com/file.pdf?token=old" {
		t.Fatalf("existing token url = %q", got)
	}
}

func TestValidateMoodleURLForSSRF(t *testing.T) {
	withMoodleTestHooks(t)
	origLoopback := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = false
	defer func() { restAPISSRFAllowLoopback = origLoopback }()

	if err := validateMoodleURLForSSRF("ftp://example.com"); err == nil {
		t.Fatalf("expected scheme rejection")
	}
	if err := validateMoodleURLForSSRF("localhost"); err == nil {
		t.Fatalf("expected missing host rejection")
	}
	if err := validateMoodleURLForSSRF("https://localhost/moodle"); err == nil {
		t.Fatalf("expected localhost rejection")
	}
	if err := validateMoodleURLForSSRF("https://127.0.0.1/moodle"); err == nil {
		t.Fatalf("expected loopback rejection")
	}
}

func TestMoodleConnectorFileDownloadRequiresToken(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := collectMoodleDocuments(t, session)
	for _, document := range documents {
		if strings.HasPrefix(document.SourceID, "moodle_resource_") && string(document.Blob) != "%PDF-1.4 fake" {
			t.Fatalf("resource blob = %q", document.Blob)
		}
	}
}

func TestMoodleConnectorValidateConnectorSetting(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, fullSyncMoodleFixtures)
	connector := mustMoodleConnector(t, server.URL)
	if err := connector.ValidateConnectorSetting(context.Background(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
}

func TestMoodleRedactedURL(t *testing.T) {
	original := "https://moodle.example.com/pluginfile.php/guides/guide.pdf?token=secret&forcedownload=1"
	redacted := moodleRedactedURL(original)
	if strings.Contains(redacted, "token") || strings.Contains(redacted, "?") {
		t.Fatalf("redacted URL still contains query params: %q", redacted)
	}
	want := "https://moodle.example.com/pluginfile.php/guides/guide.pdf"
	if redacted != want {
		t.Fatalf("redacted URL = %q, want %q", redacted, want)
	}
}

func TestMoodleConnectorDownloadErrorFailsCourse(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, func(serverURL string) moodleTestFixtures {
		return moodleTestFixtures{
			courses: `[{"id":1,"fullname":"Course One","shortname":"c1"}]`,
			contents: map[string]string{
				"1": `[{"id":11,"name":"Week 1","section":0,"modules":[` +
					`{"id":100,"name":"Guide","modname":"resource","instance":7,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[{"type":"file","filename":"guide.pdf","filepath":"/","filesize":123,"fileurl":"` + serverURL + `/pluginfile.php/nonexistent/guide.pdf","mimetype":"application/pdf","timemodified":1700000000}]}` +
					`]}]`,
			},
			files: map[string]string{},
		}
	})
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	_, err = session.NextBatch(context.Background())
	if err == nil {
		t.Fatalf("expected NextBatch to fail when a download fails")
	}
}

func TestMoodleConnectorMetadataRedactsFileURL(t *testing.T) {
	withMoodleTestHooks(t)
	server := newTestMoodleServer(t, func(serverURL string) moodleTestFixtures {
		return moodleTestFixtures{
			courses: `[{"id":1,"fullname":"Course One","shortname":"c1"}]`,
			contents: map[string]string{
				"1": `[{"id":11,"name":"Week 1","section":0,"modules":[` +
					`{"id":100,"name":"Guide","modname":"resource","instance":7,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[{"type":"file","filename":"guide.pdf","filepath":"/","filesize":123,"fileurl":"` + serverURL + `/pluginfile.php/guides/guide.pdf?forcedownload=1","mimetype":"application/pdf","timemodified":1700000000}]}` +
					`]}]`,
			},
			files: map[string]string{
				"/pluginfile.php/guides/guide.pdf": "%PDF-1.4 fake",
			},
		}
	})
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	documents := collectMoodleDocuments(t, session)
	for _, doc := range documents {
		if doc.SourceID != "moodle_resource_100" {
			continue
		}
		fileURL, ok := doc.Metadata["file_url"].(string)
		if !ok {
			t.Fatalf("file_url not a string: %T", doc.Metadata["file_url"])
		}
		if strings.Contains(fileURL, "?") {
			t.Fatalf("file_url should be redacted (no query): %q", fileURL)
		}
	}
}

func TestMoodleAssertURLSafeRejectsCrossOrigin(t *testing.T) {
	origLoopback := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = false
	defer func() { restAPISSRFAllowLoopback = origLoopback }()

	_, _, err := moodleAssertURLSafe(context.Background(), "https://evil.com/api", "https://example.com")
	if err == nil {
		t.Fatalf("expected cross-origin rejection")
	}
}

func TestMoodleAssertURLSafeLoopbackAllAddresses(t *testing.T) {
	origLoopback := restAPISSRFAllowLoopback
	restAPISSRFAllowLoopback = true
	defer func() { restAPISSRFAllowLoopback = origLoopback }()

	// All loopback → allowed.
	_, _, err := moodleAssertURLSafe(context.Background(), "http://127.0.0.1/path", "http://127.0.0.1")
	if err != nil {
		t.Fatalf("expected loopback to be allowed: %v", err)
	}

	// Not all loopback (private address) → rejected even with loopback flag.
	_, _, err = moodleAssertURLSafe(context.Background(), "http://10.0.0.1/path", "http://10.0.0.1")
	if err == nil {
		t.Fatalf("expected non-loopback private address to be rejected even with loopback flag")
	}
}

func TestMoodleConnectorRESTResponseSizeCap(t *testing.T) {
	withMoodleTestHooks(t)
	origMax := moodleMaxResponseSize
	moodleMaxResponseSize = 10
	defer func() { moodleMaxResponseSize = origMax }()

	server := newTestMoodleServer(t, func(serverURL string) moodleTestFixtures {
		fixtures := fullSyncMoodleFixtures(serverURL)
		fixtures.siteInfoBody = `{"sitename":"Test Moodle with a very long name that exceeds the cap"}`
		return fixtures
	})
	connector := mustMoodleConnector(t, server.URL)
	err := connector.Validate(context.Background())
	if err == nil {
		t.Fatalf("expected oversize response error")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("expected oversize error, got: %v", err)
	}
}

func TestMoodleConnectorDownloadSizeCap(t *testing.T) {
	withMoodleTestHooks(t)
	origMax := moodleMaxDownloadSize
	moodleMaxDownloadSize = 10
	defer func() { moodleMaxDownloadSize = origMax }()

	server := newTestMoodleServer(t, func(serverURL string) moodleTestFixtures {
		return moodleTestFixtures{
			courses: `[{"id":1,"fullname":"Course One","shortname":"c1"}]`,
			contents: map[string]string{
				"1": `[{"id":11,"name":"Week 1","section":0,"modules":[` +
					`{"id":100,"name":"Guide","modname":"resource","instance":7,"visible":1,"groupmode":0,"timecreated":1000000000,"timemodified":1700000000,"contents":[{"type":"file","filename":"guide.pdf","filepath":"/","filesize":123,"fileurl":"` + serverURL + `/pluginfile.php/guides/guide.pdf","mimetype":"application/pdf","timemodified":1700000000}]}` +
					`]}]`,
			},
			files: map[string]string{
				"/pluginfile.php/guides/guide.pdf": "this content is longer than ten bytes",
			},
		}
	})
	connector := mustMoodleConnector(t, server.URL)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	_, err = session.NextBatch(context.Background())
	if err == nil {
		t.Fatalf("expected oversize download error")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("expected oversize error, got: %v", err)
	}
}
