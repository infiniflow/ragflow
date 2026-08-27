package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func withSlackTestHooks(t *testing.T) {
	t.Helper()
	origTries := slackRetryTries
	origBaseDelay := slackRetryBaseDelay
	origBackoff := slackRetryBackoff
	slackRetryTries = 2
	slackRetryBaseDelay = time.Millisecond
	slackRetryBackoff = 2
	t.Cleanup(func() {
		slackRetryTries = origTries
		slackRetryBaseDelay = origBaseDelay
		slackRetryBackoff = origBackoff
	})
}

func mustSlackConnector(t *testing.T, serverURL string, extra map[string]any) *SlackConnector {
	t.Helper()
	config := map[string]any{
		"batch_size": 2,
		"credentials": map[string]any{
			"slack_bot_token": "xoxb-test",
		},
	}
	for key, value := range extra {
		config[key] = value
	}
	connector, err := NewSlackConnector(config)
	if err != nil {
		t.Fatalf("NewSlackConnector: %v", err)
	}
	if serverURL != "" {
		connector.baseURL = strings.TrimRight(serverURL, "/") + "/api"
	}
	return connector
}

type slackTestFixtures struct {
	authError        string
	channelsJSON     string
	historyByChannel map[string]string
	repliesByKey     map[string]string
	usersByID        map[string]string

	listCalls     *atomic.Int64
	historyCalls  *atomic.Int64
	joinCalls     *atomic.Int64
	usersCalls    *atomic.Int64
	historyParams []url.Values
}

func newTestSlackServer(t *testing.T, fixtures *slackTestFixtures) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		if fixtures.authError != "" {
			writeSlackError(w, fixtures.authError)
			return
		}
		w.Write([]byte(`{"ok":true,"url":"https://example.slack.com/","team":"Test","user":"bot","team_id":"T1","user_id":"U1"}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		if fixtures.listCalls != nil {
			fixtures.listCalls.Add(1)
		}
		body := fixtures.channelsJSON
		if body == "" {
			body = `{"ok":true,"channels":[],"response_metadata":{"next_cursor":""}}`
		}
		w.Write([]byte(body))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		if fixtures.historyCalls != nil {
			fixtures.historyCalls.Add(1)
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if fixtures.historyParams != nil {
			params := url.Values{}
			for key, values := range r.Form {
				params[key] = append([]string{}, values...)
			}
			fixtures.historyParams = append(fixtures.historyParams, params)
		}
		body := fixtures.historyByChannel[r.Form.Get("channel")]
		if body == "" {
			body = `{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`
		}
		w.Write([]byte(body))
	})
	mux.HandleFunc("/api/conversations.replies", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		key := r.Form.Get("channel") + ":" + r.Form.Get("ts")
		body := fixtures.repliesByKey[key]
		if body == "" {
			body = `{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`
		}
		w.Write([]byte(body))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		if fixtures.joinCalls != nil {
			fixtures.joinCalls.Add(1)
		}
		w.Write([]byte(`{"ok":true,"channel":{"id":"C1"}}`))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		if fixtures.usersCalls != nil {
			fixtures.usersCalls.Add(1)
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		body := fixtures.usersByID[r.Form.Get("user")]
		if body == "" {
			body = `{"ok":false,"error":"user_not_found"}`
		}
		w.Write([]byte(body))
	})
	return server
}

func writeSlackError(w http.ResponseWriter, apiError string) {
	w.Write([]byte(`{"ok":false,"error":"` + apiError + `"}`))
}

func slackHistoryBody(messages ...string) string {
	return `{"ok":true,"messages":[` + strings.Join(messages, ",") + `],"has_more":false,"response_metadata":{"next_cursor":""}}`
}

const (
	slackChannelsTwo = `{"ok":true,"channels":[
		{"id":"C1","name":"general","is_archived":false,"is_member":true,"is_private":false},
		{"id":"C2","name":"random","is_archived":false,"is_member":false,"is_private":false}
	],"response_metadata":{"next_cursor":""}}`

	slackMsgPlain        = `{"type":"message","ts":"1700000000.000001","user":"U123","text":"Hello <!channel> from <@U456>"}`
	slackMsgThreadParent = `{"type":"message","ts":"1700000001.000001","user":"U123","text":"Thread root","thread_ts":"1700000001.000001"}`
	slackMsgBot          = `{"type":"message","ts":"1700000003.000001","user":"U123","text":"bot noise","bot_id":"B1","bot_profile":{"name":"Some Bot"}}`
	slackMsgJoin         = `{"type":"message","subtype":"channel_join","ts":"1700000004.000001","user":"U123","text":"<@U123> has joined the channel"}`
	slackMsgDanswerBot   = `{"type":"message","ts":"1700000005.000001","user":"U123","text":"danswer note","bot_id":"B2","bot_profile":{"name":"DanswerBot Testing"}}`
	slackMsgNormal       = `{"type":"message","ts":"1700000006.000001","user":"U123","text":"A normal message"}`
)

func slackThreadBody() string {
	return `{"ok":true,"messages":[
		{"type":"message","ts":"1700000001.000001","user":"U123","text":"Thread root","thread_ts":"1700000001.000001"},
		{"type":"message","ts":"1700000002.000002","user":"U456","text":"A reply"}
	],"has_more":false,"response_metadata":{"next_cursor":""}}`
}

func slackUserAlice() string {
	return `{"ok":true,"user":{"id":"U123","real_name":"Alice","profile":{"display_name":"Alice","real_name":"Alice"}}}`
}

func slackUserBob() string {
	return `{"ok":true,"user":{"id":"U456","real_name":"Bob","profile":{"display_name":"","real_name":"Bob"}}}`
}

func collectSlackDocuments(t *testing.T, session SyncSession) []SourceDocument {
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

func TestSlackConnectorValidate(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON: slackChannelsTwo,
		usersByID:    map[string]string{"USLACKBOT": slackUserAlice()},
		listCalls:    &atomic.Int64{},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if fixtures.listCalls == nil || fixtures.listCalls.Load() == 0 {
		t.Fatalf("conversations.list was not called during validation")
	}
}

func TestSlackConnectorValidateMissingToken(t *testing.T) {
	connector := mustSlackConnector(t, "", map[string]any{"credentials": map[string]any{}})
	err := connector.Validate(context.Background())
	if err == nil {
		t.Fatalf("expected missing credential error")
	}
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %T, want *ConnectorMissingCredentialError", err)
	}
}

func TestSlackConnectorValidateNil(t *testing.T) {
	var connector *SlackConnector
	if err := connector.Validate(context.Background()); err == nil {
		t.Fatalf("expected error for nil connector")
	}
}

func TestSlackConnectorValidateInvalidAuth(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{authError: "invalid_auth"}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	err := connector.Validate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Invalid Slack bot token") {
		t.Fatalf("Validate error = %v, want invalid bot token error", err)
	}
}

func TestSlackConnectorValidateMissingScope(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON: slackChannelsTwo,
		usersByID:    map[string]string{"USLACKBOT": `{"ok":false,"error":"missing_scope"}`},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	err := connector.Validate(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "scope") {
		t.Fatalf("Validate error = %v, want missing scope error", err)
	}
}

func TestSlackConnectorMissingScopeMessageNamesNeededScope(t *testing.T) {
	withSlackTestHooks(t)
	body := []byte(`{"ok":false,"error":"missing_scope","needed":"channels:read","provided":"channels:write"}`)
	err := classifySlackAPIError("conversations.list", "missing_scope", body)
	var valErr *ConnectorValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("error = %T, want *ConnectorValidationError", err)
	}
	if !strings.Contains(valErr.Message, "channels:read") {
		t.Fatalf("message = %q, want it to contain %q", valErr.Message, "channels:read")
	}
	if strings.Contains(valErr.Message, "channels:write") {
		t.Fatalf("message = %q, want it not to mention provided scopes", valErr.Message)
	}
}

func TestSlackConnectorMissingScopeMessageFallsBackWithoutNeeded(t *testing.T) {
	withSlackTestHooks(t)
	body := []byte(`{"ok":false,"error":"missing_scope"}`)
	err := classifySlackAPIError("users.info", "missing_scope", body)
	var valErr *ConnectorValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("error = %T, want *ConnectorValidationError", err)
	}
	if !strings.Contains(valErr.Message, "users.info") || !strings.Contains(valErr.Message, "missing_scope") {
		t.Fatalf("message = %q, want generic missing scope message", valErr.Message)
	}
}

func TestSlackConnectorValidateRateLimitedProceeds(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{authError: "ratelimited"}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate should proceed after rate limit, got %v", err)
	}
}

func TestSlackConnectorOpenSyncBuildsThreadAndMessageDocs(t *testing.T) {
	withSlackTestHooks(t)
	joinCalls := &atomic.Int64{}
	fixtures := &slackTestFixtures{
		channelsJSON: slackChannelsTwo,
		historyByChannel: map[string]string{
			"C1": slackHistoryBody(slackMsgThreadParent, slackMsgPlain),
		},
		repliesByKey: map[string]string{"C1:1700000001.000001": slackThreadBody()},
		usersByID:    map[string]string{"U123": slackUserAlice(), "U456": slackUserBob()},
		joinCalls:    joinCalls,
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(documents))
	}

	threadDoc := documents[0]
	if threadDoc.SourceID != "C1__1700000001.000001" {
		t.Fatalf("thread SourceID = %q", threadDoc.SourceID)
	}
	if threadDoc.SemanticIdentifier != "Alice in #general: Thread root" {
		t.Fatalf("thread semantic = %q", threadDoc.SemanticIdentifier)
	}
	if threadDoc.Extension != ".txt" {
		t.Fatalf("thread extension = %q", threadDoc.Extension)
	}
	if string(threadDoc.Blob) != "Thread root\n\nA reply" {
		t.Fatalf("thread blob = %q", string(threadDoc.Blob))
	}
	if threadDoc.UpdatedAt.Unix() != 1700000002 {
		t.Fatalf("thread updated_at = %v", threadDoc.UpdatedAt)
	}
	if threadDoc.Metadata["Channel"] != "general" {
		t.Fatalf("thread metadata = %v", threadDoc.Metadata)
	}

	plainDoc := documents[1]
	if plainDoc.SourceID != "C1__1700000000.000001" {
		t.Fatalf("plain SourceID = %q", plainDoc.SourceID)
	}
	if plainDoc.SemanticIdentifier != "Alice in #general: Hello @channel from @Bob" {
		t.Fatalf("plain semantic = %q", plainDoc.SemanticIdentifier)
	}
	if string(plainDoc.Blob) != "Hello @channel from @Bob" {
		t.Fatalf("plain blob = %q", string(plainDoc.Blob))
	}

	if joinCalls.Load() != 1 {
		t.Fatalf("conversations.join calls = %d, want 1 (C2 is not a member)", joinCalls.Load())
	}
}

func TestSlackConnectorOpenSyncWindowBounds(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": slackHistoryBody(slackMsgPlain)},
		historyParams:    []url.Values{},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	windowStart := time.Unix(1700000000, 0).UTC()
	windowEnd := time.Unix(1700000100, 0).UTC()
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &windowStart,
		WindowEnd:   windowEnd,
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	collectSlackDocuments(t, session)

	// Both C1 and C2 are enumerated; C1 is processed first.
	if len(fixtures.historyParams) != 2 {
		t.Fatalf("history calls = %d, want 2", len(fixtures.historyParams))
	}
	if got := fixtures.historyParams[0].Get("oldest"); got != "1700000000" {
		t.Fatalf("oldest = %q, want 1700000000", got)
	}
	if got := fixtures.historyParams[0].Get("latest"); got != "1700000100" {
		t.Fatalf("latest = %q, want 1700000100", got)
	}
}

func TestSlackConnectorOpenSyncFromBeginningHasNoWindow(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": slackHistoryBody(slackMsgPlain)},
		historyParams:    []url.Values{},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	collectSlackDocuments(t, session)

	if got := fixtures.historyParams[0].Get("oldest"); got != "" {
		t.Fatalf("oldest = %q, want empty for full sync", got)
	}
	if got := fixtures.historyParams[0].Get("latest"); got != "" {
		t.Fatalf("latest = %q, want empty for full sync", got)
	}
}

func TestSlackConnectorOpenSyncFiltersMessages(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": slackHistoryBody(slackMsgBot, slackMsgJoin, slackMsgDanswerBot, slackMsgNormal)},
		usersByID:        map[string]string{"U123": slackUserAlice()},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 2 {
		t.Fatalf("documents = %d, want 2 (Danswer bot and normal message)", len(documents))
	}
	if documents[0].SourceID != "C1__1700000005.000001" {
		t.Fatalf("first doc SourceID = %q", documents[0].SourceID)
	}
	if documents[1].SourceID != "C1__1700000006.000001" {
		t.Fatalf("second doc SourceID = %q", documents[1].SourceID)
	}
}

func TestSlackConnectorOpenSyncChannelSelection(t *testing.T) {
	withSlackTestHooks(t)
	historyCalls := &atomic.Int64{}
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": slackHistoryBody(slackMsgPlain)},
		usersByID:        map[string]string{"U123": slackUserAlice()},
		historyCalls:     historyCalls,
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, map[string]any{"channels": "general"})

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 1 || documents[0].SourceID != "C1__1700000000.000001" {
		t.Fatalf("documents = %+v", documents)
	}
	if historyCalls.Load() != 1 {
		t.Fatalf("history calls = %d, want 1", historyCalls.Load())
	}
}

func TestSlackConnectorOpenSyncChannelNotFound(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{channelsJSON: slackChannelsTwo}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, map[string]any{"channels": "missing"})

	_, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("OpenSync error = %v, want channel not found", err)
	}
}

func TestSlackConnectorOpenSyncRegexChannels(t *testing.T) {
	withSlackTestHooks(t)
	historyCalls := &atomic.Int64{}
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": slackHistoryBody(slackMsgPlain)},
		usersByID:        map[string]string{"U123": slackUserAlice()},
		historyCalls:     historyCalls,
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, map[string]any{
		"channels":              "gen.*",
		"channel_regex_enabled": true,
	})

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 1 || documents[0].SourceID != "C1__1700000000.000001" {
		t.Fatalf("documents = %+v", documents)
	}
}

func TestSlackConnectorOpenSyncResumesFromCheckpoint(t *testing.T) {
	withSlackTestHooks(t)
	historyCalls := &atomic.Int64{}
	fixtures := &slackTestFixtures{
		channelsJSON: `{"ok":true,"channels":[
			{"id":"C1","name":"general","is_member":true},
			{"id":"C2","name":"random","is_member":true},
			{"id":"C3","name":"dev","is_member":true}
		],"response_metadata":{"next_cursor":""}}`,
		historyByChannel: map[string]string{
			"C1": slackHistoryBody(slackMsgPlain),
			"C3": slackHistoryBody(slackMsgNormal),
		},
		usersByID:    map[string]string{"U123": slackUserAlice()},
		historyCalls: historyCalls,
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   "slack_channel_C2",
			SourceID: "slack_channel_C2",
		},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 1 || documents[0].SourceID != "C3__1700000006.000001" {
		t.Fatalf("documents = %+v, want only channel C3", documents)
	}
	if historyCalls.Load() != 1 {
		t.Fatalf("history calls = %d, want 1 (only C3)", historyCalls.Load())
	}
}

func TestSlackConnectorOpenSyncResumeRejectsMissingChannel(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON: `{"ok":true,"channels":[
			{"id":"C1","name":"general","is_member":true},
			{"id":"C2","name":"random","is_member":true}
		],"response_metadata":{"next_cursor":""}}`,
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   "slack_channel_C3",
			SourceID: "slack_channel_C3",
		},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestSlackConnectorOpenSyncCheckpointAdvancesPerChannel(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON: slackChannelsTwo,
		historyByChannel: map[string]string{
			"C1": slackHistoryBody(slackMsgPlain, slackMsgNormal),
			"C2": slackHistoryBody(slackMsgThreadParent),
		},
		repliesByKey: map[string]string{"C2:1700000001.000001": slackThreadBody()},
		usersByID:    map[string]string{"U123": slackUserAlice()},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, map[string]any{"batch_size": 1})

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	var checkpoints []string
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		if len(batch.Documents) == 0 {
			t.Fatalf("empty batch")
		}
		if batch.Checkpoint != nil {
			checkpoints = append(checkpoints, batch.Checkpoint.Cursor)
		}
	}
	// batch_size=1: C1 emits two batches (no checkpoint on the first), C2 emits
	// one; the checkpoint advances only on the last batch of each channel.
	if len(checkpoints) != 2 || checkpoints[0] != "slack_channel_C1" || checkpoints[1] != "slack_channel_C2" {
		t.Fatalf("checkpoints = %v, want [slack_channel_C1 slack_channel_C2]", checkpoints)
	}
}

func TestSlackConnectorOpenPruneSlimSnapshot(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": slackHistoryBody(slackMsgPlain, slackMsgBot)},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()

	var slim []SlimDocument
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		slim = append(slim, batch.Documents...)
	}
	if len(slim) != 1 || slim[0].SourceID != "C1__1700000000.000001" {
		t.Fatalf("slim documents = %+v", slim)
	}
}

func TestSlackConnectorOpenPruneThreadRootIdentity(t *testing.T) {
	withSlackTestHooks(t)
	history := slackHistoryBody(
		slackMsgThreadParent, // root: ts == thread_ts == 1700000001.000001
		`{"type":"message","ts":"1700000002.000002","user":"U456","text":"A reply","thread_ts":"1700000001.000001"}`,
		slackMsgPlain,
	)
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": history},
		repliesByKey:     map[string]string{"C1:1700000001.000001": slackThreadBody()},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()

	var slim []SlimDocument
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		slim = append(slim, batch.Documents...)
	}
	// The thread is emitted once under its root timestamp; the reply and plain
	// message must not leak their own timestamps.
	want := []string{"C1__1700000001.000001", "C1__1700000000.000001"}
	if len(slim) != len(want) {
		t.Fatalf("slim documents = %+v, want %v", slim, want)
	}
	for i, doc := range slim {
		if doc.SourceID != want[i] {
			t.Fatalf("slim[%d].SourceID = %q, want %q", i, doc.SourceID, want[i])
		}
	}
}

func TestSlackConnectorOpenPruneBotRootThread(t *testing.T) {
	withSlackTestHooks(t)
	history := slackHistoryBody(
		`{"type":"message","ts":"1700000001.000001","user":"U123","text":"bot root","thread_ts":"1700000001.000001","bot_id":"B1","bot_profile":{"name":"Some Bot"}}`,
		`{"type":"message","ts":"1700000002.000002","user":"U456","text":"A reply","thread_ts":"1700000001.000001"}`,
	)
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": history},
		repliesByKey: map[string]string{
			"C1:1700000001.000001": `{"ok":true,"messages":[
				{"type":"message","ts":"1700000001.000001","user":"U123","text":"bot root","thread_ts":"1700000001.000001","bot_id":"B1","bot_profile":{"name":"Some Bot"}},
				{"type":"message","ts":"1700000002.000002","user":"U456","text":"A reply","thread_ts":"1700000001.000001"}
			],"has_more":false,"response_metadata":{"next_cursor":""}}`,
		},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()

	var slim []SlimDocument
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		slim = append(slim, batch.Documents...)
	}
	// A thread with an accepted reply is pruned under the root timestamp even
	// when the root message itself is filtered out.
	if len(slim) != 1 || slim[0].SourceID != "C1__1700000001.000001" {
		t.Fatalf("slim documents = %+v, want [C1__1700000001.000001]", slim)
	}
}

func TestSlackConnectorOpenPrunePagesChannelsLazily(t *testing.T) {
	withSlackTestHooks(t)
	historyCalls := &atomic.Int64{}
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": slackHistoryBody(slackMsgPlain)},
		historyCalls:     historyCalls,
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()
	if historyCalls.Load() != 0 {
		t.Fatalf("history calls after OpenPrune = %d, want 0 (lazy paging)", historyCalls.Load())
	}

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if historyCalls.Load() == 0 {
		t.Fatalf("history calls after NextBatch = 0, want lazy channel fetch")
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "C1__1700000000.000001" {
		t.Fatalf("batch documents = %+v", batch.Documents)
	}
}

func TestSlackConnectorOpenPruneResolvesRepliesFromThreadAPI(t *testing.T) {
	withSlackTestHooks(t)
	// The bot root is not accepted, and history carries no replies. Only
	// conversations.replies reveals the accepted reply, so prune must resolve
	// the thread like sync does instead of trusting history alone.
	history := slackHistoryBody(
		`{"type":"message","ts":"1700000001.000001","user":"U123","text":"bot root","thread_ts":"1700000001.000001","bot_id":"B1","bot_profile":{"name":"Some Bot"}}`,
	)
	fixtures := &slackTestFixtures{
		channelsJSON:     slackChannelsTwo,
		historyByChannel: map[string]string{"C1": history},
		repliesByKey: map[string]string{
			"C1:1700000001.000001": `{"ok":true,"messages":[
				{"type":"message","ts":"1700000001.000001","user":"U123","text":"bot root","thread_ts":"1700000001.000001","bot_id":"B1","bot_profile":{"name":"Some Bot"}},
				{"type":"message","ts":"1700000002.000002","user":"U456","text":"A reply","thread_ts":"1700000001.000001"}
			],"has_more":false,"response_metadata":{"next_cursor":""}}`,
		},
	}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()

	var slim []SlimDocument
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		slim = append(slim, batch.Documents...)
	}
	if len(slim) != 1 || slim[0].SourceID != "C1__1700000001.000001" {
		t.Fatalf("slim documents = %+v, want [C1__1700000001.000001]", slim)
	}
}

func TestSlackConnectorOpenPrunePropagatesThreadResolutionError(t *testing.T) {
	withSlackTestHooks(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackChannelsTwo))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("channel") != "C1" {
			w.Write([]byte(`{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`))
			return
		}
		w.Write([]byte(slackHistoryBody(slackMsgThreadParent)))
	})
	mux.HandleFunc("/api/conversations.replies", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackUserAlice()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, nil)
	session, err := connector.OpenPrune(context.Background(), PruneRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()

	_, err = session.NextBatch(context.Background())
	if err == nil {
		t.Fatalf("NextBatch: want thread resolution error, got nil")
	}
}

func TestSlackConnectorTextCleaning(t *testing.T) {
	withSlackTestHooks(t)
	usersCalls := &atomic.Int64{}
	fixtures := &slackTestFixtures{usersByID: map[string]string{"U123": slackUserAlice()}, usersCalls: usersCalls}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	text := "Hi <@U123> in <#C1|general> <!channel> <!here> <!everyone> <!date|123|formatted> and <@U123> again"
	got, err := connector.indexClean(context.Background(), text)
	if err != nil {
		t.Fatalf("indexClean: %v", err)
	}
	want := "Hi @Alice in #general @channel @here @everyone 123|formatted and @Alice again"
	if got != want {
		t.Fatalf("indexClean = %q, want %q", got, want)
	}
	if usersCalls.Load() != 1 {
		t.Fatalf("users.info calls = %d, want 1 (cached)", usersCalls.Load())
	}
}

func TestSlackConnectorValidateConnectorSetting(t *testing.T) {
	withSlackTestHooks(t)
	fixtures := &slackTestFixtures{channelsJSON: slackChannelsTwo}
	server := newTestSlackServer(t, fixtures)
	connector := mustSlackConnector(t, server.URL, nil)

	if err := connector.ValidateConnectorSetting(context.Background(), map[string]any{}); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
}

func TestSlackConnectorHistoryPagination(t *testing.T) {
	withSlackTestHooks(t)
	var calls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackChannelsTwo))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("channel") != "C1" {
			w.Write([]byte(`{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`))
			return
		}
		if calls.Add(1) == 1 {
			w.Write([]byte(`{"ok":true,"messages":[` + slackMsgPlain + `],"has_more":true,"response_metadata":{"next_cursor":"nextpage"}}`))
			return
		}
		w.Write([]byte(`{"ok":true,"messages":[` + slackMsgNormal + `],"has_more":false,"response_metadata":{"next_cursor":""}}`))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackUserAlice()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, nil)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 2 {
		t.Fatalf("documents = %d, want 2 across two history pages", len(documents))
	}
	if calls.Load() != 2 {
		t.Fatalf("C1 history calls = %d, want 2", calls.Load())
	}
}

func TestSlackConnectorChannelListPagination(t *testing.T) {
	withSlackTestHooks(t)
	var calls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Write([]byte(`{"ok":true,"channels":[{"id":"C1","name":"general","is_member":true}],"response_metadata":{"next_cursor":"nextpage"}}`))
			return
		}
		w.Write([]byte(`{"ok":true,"channels":[{"id":"C2","name":"random","is_member":true}],"response_metadata":{"next_cursor":""}}`))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackUserAlice()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, nil)
	channels, err := connector.resolvedChannels(context.Background())
	if err != nil {
		t.Fatalf("resolvedChannels: %v", err)
	}
	if len(channels) != 2 || channels[0].ID != "C1" || channels[1].ID != "C2" {
		t.Fatalf("channels = %+v", channels)
	}
}

func TestSlackConnectorRetriesTransientHistory(t *testing.T) {
	withSlackTestHooks(t)
	var calls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackChannelsTwo))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("channel") != "C1" {
			w.Write([]byte(`{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`))
			return
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"ok":false,"error":"ratelimited"}`))
			return
		}
		w.Write([]byte(`{"ok":true,"messages":[` + slackMsgPlain + `],"has_more":false,"response_metadata":{"next_cursor":""}}`))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackUserAlice()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, nil)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 1 {
		t.Fatalf("documents = %d, want 1 after transient retry", len(documents))
	}
	if calls.Load() != 2 {
		t.Fatalf("C1 history calls = %d, want 2 (429 then success)", calls.Load())
	}
}

func TestSlackConnectorOpenSyncSkipsUnjoinableChannel(t *testing.T) {
	withSlackTestHooks(t)
	joinCalls := &atomic.Int64{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackChannelsTwo))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		joinCalls.Add(1)
		if r.FormValue("channel") == "C2" {
			w.Write([]byte(`{"ok":false,"error":"is_archived"}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("channel") != "C1" {
			w.Write([]byte(`{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`))
			return
		}
		w.Write([]byte(slackHistoryBody(slackMsgPlain)))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackUserAlice()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, nil)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	documents := collectSlackDocuments(t, session)
	if len(documents) != 1 || documents[0].SourceID != "C1__1700000000.000001" {
		t.Fatalf("documents = %+v, want only C1", documents)
	}
	if joinCalls.Load() != 1 {
		t.Fatalf("join calls = %d, want 1 (only C2)", joinCalls.Load())
	}
}

func TestSlackConnectorOpenPruneSkipsUnjoinableChannel(t *testing.T) {
	withSlackTestHooks(t)
	joinCalls := &atomic.Int64{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackChannelsTwo))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		joinCalls.Add(1)
		if r.FormValue("channel") == "C2" {
			w.Write([]byte(`{"ok":false,"error":"method_not_supported_for_channel_type"}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("channel") != "C1" {
			w.Write([]byte(`{"ok":true,"messages":[],"has_more":false,"response_metadata":{"next_cursor":""}}`))
			return
		}
		w.Write([]byte(slackHistoryBody(slackMsgPlain)))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackUserAlice()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, nil)
	session, err := connector.OpenPrune(context.Background(), PruneRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()
	var slim []SlimDocument
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		slim = append(slim, batch.Documents...)
	}
	if len(slim) != 1 || slim[0].SourceID != "C1__1700000000.000001" {
		t.Fatalf("slim documents = %+v, want only C1", slim)
	}
}

func TestSlackConnectorJoinPropagatesUnrelatedErrors(t *testing.T) {
	withSlackTestHooks(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth.test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackChannelsTwo))
	})
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
	})
	mux.HandleFunc("/api/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackHistoryBody(slackMsgPlain)))
	})
	mux.HandleFunc("/api/users.info", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(slackUserAlice()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, nil)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	// C2 is not a member, so join fails with invalid_auth, which is unrelated
	// to channel availability and must abort the run.
	var gotErr error
	for {
		_, nextErr := session.NextBatch(context.Background())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			gotErr = nextErr
			break
		}
	}
	if gotErr == nil {
		t.Fatalf("NextBatch: want error, got nil")
	}
	if errors.Is(gotErr, errSlackChannelUnavailable) {
		t.Fatalf("NextBatch error = %v, want unrelated error to propagate unwrapped", gotErr)
	}
}

func TestSlackConnectorJoinRestrictedToSelectedChannels(t *testing.T) {
	withSlackTestHooks(t)
	joinCalls := &atomic.Int64{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		joinCalls.Add(1)
		w.Write([]byte(`{"ok":true}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := mustSlackConnector(t, server.URL, map[string]any{"channels": "general"})
	ctx := context.Background()
	selected := slackChannel{ID: "C1", Name: "general", IsMember: false}
	unselected := slackChannel{ID: "C9", Name: "random", IsMember: false}
	if err := connector.joinChannel(ctx, unselected); err != nil {
		t.Fatalf("joinChannel unselected: %v", err)
	}
	if joinCalls.Load() != 0 {
		t.Fatalf("join calls after unselected channel = %d, want 0", joinCalls.Load())
	}
	if err := connector.joinChannel(ctx, selected); err != nil {
		t.Fatalf("joinChannel selected: %v", err)
	}
	if joinCalls.Load() != 1 {
		t.Fatalf("join calls after selected channel = %d, want 1", joinCalls.Load())
	}

	// Regex selection restricts joins the same way.
	regexConnector := mustSlackConnector(t, server.URL, map[string]any{
		"channels":              "gen.*",
		"channel_regex_enabled": true,
	})
	if err := regexConnector.joinChannel(ctx, unselected); err != nil {
		t.Fatalf("regex joinChannel unselected: %v", err)
	}
	if joinCalls.Load() != 1 {
		t.Fatalf("join calls after regex unselected channel = %d, want 1", joinCalls.Load())
	}
	if err := regexConnector.joinChannel(ctx, selected); err != nil {
		t.Fatalf("regex joinChannel selected: %v", err)
	}
	if joinCalls.Load() != 2 {
		t.Fatalf("join calls after regex selected channel = %d, want 2", joinCalls.Load())
	}
}
