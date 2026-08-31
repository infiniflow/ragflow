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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ragflow/internal/utility"
)

func TestNewTeamsConnectorDefaults(t *testing.T) {
	connector, err := NewTeamsConnector(map[string]any{
		"credentials": map[string]any{
			"tenant_id":     "tenant",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewTeamsConnector failed: %v", err)
	}
	if connector.tenantID != "tenant" || connector.clientID != "client" || connector.clientSecret != "secret" {
		t.Fatalf("credentials = %+v", connector)
	}
	if connector.batchSize != defaultTeamsBatchSize {
		t.Fatalf("batch size = %d, want %d", connector.batchSize, defaultTeamsBatchSize)
	}
}

func TestNewTeamsConnectorBatchSize(t *testing.T) {
	connector, err := NewTeamsConnector(map[string]any{
		"batch_size": 5,
		"credentials": map[string]any{
			"tenant_id":     "tenant",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewTeamsConnector failed: %v", err)
	}
	if connector.batchSize != 5 {
		t.Fatalf("batch size = %d, want 5", connector.batchSize)
	}
}

func TestTeamsConnectorValidateMissingCredentials(t *testing.T) {
	connector, err := NewTeamsConnector(map[string]any{"credentials": map[string]any{}})
	if err != nil {
		t.Fatalf("NewTeamsConnector failed: %v", err)
	}
	var credErr *ConnectorMissingCredentialError
	if err := connector.Validate(context.Background()); !errors.As(err, &credErr) {
		t.Fatalf("Validate err = %v, want ConnectorMissingCredentialError", err)
	}
}

func TestTeamsConnectorValidateRejectsNonPositiveBatch(t *testing.T) {
	connector, err := NewTeamsConnector(map[string]any{
		"batch_size": 0,
		"credentials": map[string]any{
			"tenant_id":     "tenant",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewTeamsConnector failed: %v", err)
	}
	var valErr *ConnectorValidationError
	if err := connector.Validate(context.Background()); !errors.As(err, &valErr) {
		t.Fatalf("Validate err = %v, want ConnectorValidationError", err)
	}
}

func TestTeamsConnectorValidateQueriesTeams(t *testing.T) {
	connector := newFixtureTeamsConnector()
	var probed bool
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if !strings.HasSuffix(apiURL, "/teams") {
			t.Fatalf("validate url = %q", apiURL)
		}
		probed = true
		*out.(*teamsPage) = teamsPage{Value: []teamsTeam{{ID: "team-1", DisplayName: "Engineering"}}}
		return nil
	}
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !probed {
		t.Fatalf("Validate did not probe /teams")
	}
}

func TestTeamsGetJSONReadsBodyBeforeCancel(t *testing.T) {
	previousAllowAnyHost := utility.AllowAnyHostForTest
	utility.AllowAnyHostForTest = true
	t.Cleanup(func() { utility.AllowAnyHostForTest = previousAllowAnyHost })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()

	connector := &TeamsConnector{
		tenantID:     "tenant",
		clientID:     "client",
		clientSecret: "secret",
		batchSize:    defaultTeamsBatchSize,
		httpClient:   http.DefaultClient,
		now:          time.Now,
		acquireAccessToken: func(ctx context.Context) (string, error) {
			return "token", nil
		},
	}
	var page teamsPage
	if err := connector.getJSON(context.Background(), server.URL+"/teams", &page); err != nil {
		t.Fatalf("getJSON failed: %v", err)
	}
}

func TestTeamsConnectorValidateMapsHTTPStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: &ConnectorValidationError{}},
		{name: "forbidden", status: http.StatusForbidden, want: &ConnectorValidationError{}},
		{name: "server error", status: http.StatusInternalServerError, want: &ConnectorValidationError{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector := newFixtureTeamsConnector()
			connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
				return &teamsHTTPError{status: tc.status, body: "boom"}
			}
			var valErr *ConnectorValidationError
			if err := connector.Validate(context.Background()); !errors.As(err, &valErr) {
				t.Fatalf("Validate err = %v, want ConnectorValidationError", err)
			}
		})
	}
}

func TestTeamsConnectorOpenSync(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.doJSON = teamsFixtureDoJSON(t)

	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "team-1__channel-1__msg-1" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "General: Hello from Teams  Reply one" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if !doc.UpdatedAt.Equal(mustTime(t, "2026-01-03T00:30:00Z")) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Metadata["team"] != "Engineering" || doc.Metadata["channel"] != "General" {
		t.Fatalf("metadata = %+v", doc.Metadata)
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	blob := string(doc.Blob)
	if !strings.Contains(blob, "Hello from Teams") || !strings.Contains(blob, "Reply one") {
		t.Fatalf("blob = %q", blob)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "team-1__channel-1__msg-2" {
		t.Fatalf("checkpoint = %+v", batch.Checkpoint)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestTeamsConnectorOpenSyncWindowExcludesOldMessages(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.doJSON = teamsFixtureDoJSON(t)

	start := mustTime(t, "2026-01-03T01:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	// msg-1 modified at 2026-01-03T00:00:00Z is before the window start; only
	// msg-2 (2026-01-03T02:00:00Z) qualifies.
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "team-1__channel-1__msg-2" {
		t.Fatalf("documents = %+v, want msg-2 only", batch.Documents)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestTeamsConnectorOpenSyncHTMLBody(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if strings.Contains(apiURL, "/replies") {
			*out.(*teamsMessagesPage) = teamsMessagesPage{}
			return nil
		}
		if strings.Contains(apiURL, "/messages") {
			*out.(*teamsMessagesPage) = teamsMessagesPage{
				Value: []teamsMessage{{
					ID:                   "html-1",
					Body:                 teamsMessageBody{Content: "<p>Hi</p>", ContentType: "html"},
					LastModifiedDateTime: "2026-01-03T05:00:00Z",
					WebURL:               "https://teams.example/message/html-1",
				}},
			}
			return nil
		}
		return teamsFixtureDoJSON(t)(ctx, apiURL, out)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	if batch.Documents[0].Extension != ".html" {
		t.Fatalf("extension = %q, want .html", batch.Documents[0].Extension)
	}
	if batch.Documents[0].Metadata["web_url"] != "https://teams.example/message/html-1" {
		t.Fatalf("metadata = %+v", batch.Documents[0].Metadata)
	}
}

func TestTeamsConnectorOpenSyncResume(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.batchSize = 1
	connector.doJSON = teamsFixtureDoJSON(t)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "team-1__channel-1__msg-1" {
		t.Fatalf("first documents = %+v", first.Documents)
	}
	if first.Checkpoint == nil {
		t.Fatalf("first checkpoint is nil")
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "team-1__channel-1__msg-2" {
		t.Fatalf("resume documents = %+v, want msg-2", second.Documents)
	}
	if _, err = resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resume NextBatch EOF = %v", err)
	}
}

func TestTeamsConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.doJSON = teamsFixtureDoJSON(t)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestTeamsConnectorOpenSyncResumeRejectsMissingRemoteAnchor(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.batchSize = 1
	connector.doJSON = teamsFixtureDoJSON(t)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "team-1__channel-1__msg-1" {
		t.Fatalf("first documents = %+v", first.Documents)
	}
	if first.Checkpoint == nil {
		t.Fatalf("first checkpoint is nil")
	}

	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		switch {
		case strings.Contains(apiURL, "/channels/channel-1/messages/msg-2/replies"):
			*out.(*teamsMessagesPage) = teamsMessagesPage{}
		case strings.Contains(apiURL, "/channels/channel-1/messages"):
			*out.(*teamsMessagesPage) = teamsMessagesPage{
				Value: []teamsMessage{{
					ID:                   "msg-2",
					Body:                 teamsMessageBody{Content: "Second message", ContentType: "text"},
					LastModifiedDateTime: "2026-01-03T02:00:00Z",
					WebURL:               "https://teams.example/message/msg-2",
				}},
			}
		case strings.Contains(apiURL, "/teams/team-1/channels"):
			*out.(*teamsChannelsPage) = teamsChannelsPage{
				Value: []teamsChannel{{ID: "channel-1", DisplayName: "General"}},
			}
		case strings.Contains(apiURL, "/teams"):
			*out.(*teamsPage) = teamsPage{
				Value: []teamsTeam{{ID: "team-1", DisplayName: "Engineering"}},
			}
		default:
			t.Fatalf("unexpected api url %s", apiURL)
		}
		return nil
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	if _, err = resumed.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestTeamsConnectorOpenPrune(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.doJSON = teamsFixtureDoJSON(t)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{}
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	want := []string{"team-1__channel-1__msg-1", "team-1__channel-1__msg-2"}
	if len(got) != len(want) {
		t.Fatalf("prune documents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("prune documents = %v, want %v", got, want)
		}
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("prune NextBatch EOF = %v", err)
	}
}

func TestTeamsConnectorOpenSyncPagination(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		switch {
		case strings.Contains(apiURL, "/channels/channel-2/messages"):
			*out.(*teamsMessagesPage) = teamsMessagesPage{
				Value: []teamsMessage{{
					ID:                   "msg-3",
					Body:                 teamsMessageBody{Content: "Third", ContentType: "text"},
					LastModifiedDateTime: "2026-01-03T03:00:00Z",
				}},
			}
		case strings.Contains(apiURL, "/channels/channel-1/messages"):
			*out.(*teamsMessagesPage) = teamsMessagesPage{
				Value: []teamsMessage{{
					ID:                   "msg-1",
					Body:                 teamsMessageBody{Content: "One", ContentType: "text"},
					LastModifiedDateTime: "2026-01-03T00:00:00Z",
				}},
			}
		case strings.Contains(apiURL, "/teams/team-2/channels"):
			*out.(*teamsChannelsPage) = teamsChannelsPage{
				Value: []teamsChannel{{ID: "channel-2", DisplayName: "Random"}},
			}
		case strings.Contains(apiURL, "/teams/team-1/channels"):
			*out.(*teamsChannelsPage) = teamsChannelsPage{
				Value: []teamsChannel{{ID: "channel-1", DisplayName: "General"}},
			}
		case strings.Contains(apiURL, "/teams"):
			*out.(*teamsPage) = teamsPage{
				Value: []teamsTeam{
					{ID: "team-1", DisplayName: "Engineering"},
					{ID: "team-2", DisplayName: "Marketing"},
				},
			}
		default:
			t.Fatalf("unexpected api url %s", apiURL)
		}
		return nil
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	got := []string{}
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch failed: %v", err)
		}
		for _, doc := range batch.Documents {
			got = append(got, doc.SourceID)
		}
	}
	want := []string{
		"team-1__channel-1__msg-1",
		"team-2__channel-2__msg-3",
	}
	if len(got) != len(want) {
		t.Fatalf("documents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("documents = %v, want %v", got, want)
		}
	}
}

func TestTeamsConnectorValidateConnectorSetting(t *testing.T) {
	connector := newFixtureTeamsConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		*out.(*teamsPage) = teamsPage{Value: []teamsTeam{{ID: "team-1", DisplayName: "Engineering"}}}
		return nil
	}
	if err := connector.ValidateConnectorSetting(context.Background(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}
}

func TestTeamsMessageToSourceDocument(t *testing.T) {
	message := teamsMessage{
		ID:                   "msg-1",
		Body:                 teamsMessageBody{Content: "Post body", ContentType: "text"},
		LastModifiedDateTime: "2026-01-03T00:00:00Z",
		WebURL:               "https://teams.example/message/msg-1",
	}
	replies := []teamsMessage{
		{ID: "reply-1", Body: teamsMessageBody{Content: "Reply one", ContentType: "text"}, CreatedDateTime: "2026-01-03T02:00:00Z"},
	}
	doc := message.toSourceDocument(
		teamsTeam{ID: "team-1", DisplayName: "Engineering"},
		teamsChannel{ID: "channel-1", DisplayName: "General"},
		replies,
	)
	if doc.SourceID != "team-1__channel-1__msg-1" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "General: Post body  Reply one" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if !doc.UpdatedAt.Equal(mustTime(t, "2026-01-03T02:00:00Z")) {
		t.Fatalf("updated at = %s, want latest reply time", doc.UpdatedAt)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
}

func TestRegisterBuiltInsOpensTeams(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("teams", map[string]any{
		"credentials": map[string]any{
			"tenant_id":     "tenant",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*TeamsConnector); !ok {
		t.Fatalf("connector type = %T, want *TeamsConnector", connector)
	}
}

// newFixtureTeamsConnector builds a connector with token acquisition
// short-circuited so unit tests never touch the network.
func newFixtureTeamsConnector() *TeamsConnector {
	connector := &TeamsConnector{
		tenantID:     "tenant",
		clientID:     "client",
		clientSecret: "secret",
		batchSize:    defaultTeamsBatchSize,
		httpClient:   http.DefaultClient,
		now:          time.Now,
	}
	connector.acquireAccessToken = func(ctx context.Context) (string, error) {
		return "token", nil
	}
	return connector
}

// teamsFixtureDoJSON serves teams/channels/messages/replies for unit tests.
func teamsFixtureDoJSON(t *testing.T) func(ctx context.Context, apiURL string, out any) error {
	t.Helper()
	return func(ctx context.Context, apiURL string, out any) error {
		switch {
		case strings.Contains(apiURL, "/channels/channel-1/messages/msg-1/replies"):
			*out.(*teamsMessagesPage) = teamsMessagesPage{
				Value: []teamsMessage{{
					ID:                   "reply-1",
					Body:                 teamsMessageBody{Content: "Reply one", ContentType: "text"},
					CreatedDateTime:      "2026-01-03T00:30:00Z",
					LastModifiedDateTime: "2026-01-03T00:30:00Z",
				}},
			}
		case strings.Contains(apiURL, "/channels/channel-1/messages/msg-2/replies"):
			*out.(*teamsMessagesPage) = teamsMessagesPage{}
		case strings.Contains(apiURL, "/channels/channel-1/messages"):
			*out.(*teamsMessagesPage) = teamsMessagesPage{
				Value: []teamsMessage{
					{
						ID:                   "msg-1",
						Body:                 teamsMessageBody{Content: "Hello from Teams", ContentType: "text"},
						LastModifiedDateTime: "2026-01-03T00:00:00Z",
						WebURL:               "https://teams.example/message/msg-1",
					},
					{
						ID:                   "msg-2",
						Body:                 teamsMessageBody{Content: "Second message", ContentType: "text"},
						LastModifiedDateTime: "2026-01-03T02:00:00Z",
						WebURL:               "https://teams.example/message/msg-2",
					},
				},
			}
		case strings.Contains(apiURL, "/teams/team-1/channels"):
			*out.(*teamsChannelsPage) = teamsChannelsPage{
				Value: []teamsChannel{{ID: "channel-1", DisplayName: "General"}},
			}
		case strings.Contains(apiURL, "/teams"):
			*out.(*teamsPage) = teamsPage{
				Value: []teamsTeam{{ID: "team-1", DisplayName: "Engineering"}},
			}
		default:
			t.Fatalf("unexpected api url %s", apiURL)
		}
		return nil
	}
}
