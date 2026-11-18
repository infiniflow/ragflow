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
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// discordFixture is an in-memory Discord API stub.
type discordFixture struct {
	guilds       []map[string]any
	channels     map[string][]map[string]any // guildID -> channels
	activeThread map[string][]map[string]any // guildID -> threads
	archived     map[string][]map[string]any // channelID -> threads (public)
	messages     map[string][]map[string]any // channelID -> messages (newest first)

	forbidPrivateArchived bool
	forbidChannels        map[string]bool // channelID -> messages endpoint returns 403
	rateLimitOnce         atomic.Bool
	failArchivedThreads   bool
	failActiveThreads     bool

	requests atomic.Int64
}

func newDiscordFixture() *discordFixture {
	return &discordFixture{
		guilds: []map[string]any{
			{"id": "1001", "name": "Guild One"},
			{"id": "2001", "name": "Guild Two"},
		},
		channels: map[string][]map[string]any{
			"1001": {
				{"id": "ch-1", "name": "general", "type": 0, "guild_id": "1001"},
				{"id": "ch-2", "name": "announcements", "type": 0, "guild_id": "1001"},
				{"id": "ch-3", "name": "mod-only", "type": 0, "guild_id": "1001"},
				{"id": "cat-1", "name": "Category", "type": 4, "guild_id": "1001"},
				{"id": "forum-1", "name": "Forum", "type": 15, "guild_id": "1001"},
			},
			"2001": {
				{"id": "ch-g2", "name": "general", "type": 0, "guild_id": "2001"},
			},
		},
		activeThread: map[string][]map[string]any{
			"1001": {
				{"id": "thread-a", "name": "Active Thread", "type": 12, "guild_id": "1001", "parent_id": "ch-1"},
				{"id": "thread-other", "name": "Other Thread", "type": 12, "guild_id": "1001", "parent_id": "ch-2"},
			},
		},
		archived: map[string][]map[string]any{
			"ch-1": {
				{"id": "thread-b", "name": "Archived Thread", "type": 11, "guild_id": "1001", "parent_id": "ch-1"},
			},
		},
		messages: map[string][]map[string]any{},
	}
}

func (f *discordFixture) setMessages(channelID string, messages ...map[string]any) {
	f.messages[channelID] = messages
}

func (f *discordFixture) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		path := r.URL.Path
		query := r.URL.Query()

		switch {
		case path == "/users/@me/guilds":
			json.NewEncoder(w).Encode(f.guilds)
			return
		case strings.HasPrefix(path, "/guilds/") && strings.HasSuffix(path, "/channels"):
			guildID := strings.TrimSuffix(strings.TrimPrefix(path, "/guilds/"), "/channels")
			json.NewEncoder(w).Encode(f.channels[guildID])
			return
		case strings.HasPrefix(path, "/guilds/") && strings.HasSuffix(path, "/threads/active"):
			guildID := strings.TrimSuffix(strings.TrimPrefix(path, "/guilds/"), "/threads/active")
			if f.failActiveThreads {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"threads": f.activeThread[guildID]})
			return
		case strings.HasPrefix(path, "/channels/") && strings.Contains(path, "/threads/archived/"):
			channelID := strings.TrimPrefix(path, "/channels/")
			channelID = strings.TrimSuffix(channelID, "/threads/archived/public")
			channelID = strings.TrimSuffix(channelID, "/threads/archived/private")
			if strings.Contains(r.URL.Path, "/threads/archived/private") && f.forbidPrivateArchived {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if f.failArchivedThreads {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			all := f.archived[channelID]
			threads := all
			start := 0
			if before := query.Get("before"); before != "" {
				for i, thread := range all {
					if thread["id"] == before {
						start = i + 1
						break
					}
				}
				threads = threads[start:]
			}
			json.NewEncoder(w).Encode(map[string]any{"threads": threads, "has_more": len(all) > start+len(threads)})
			return
		case strings.HasPrefix(path, "/channels/") && strings.HasSuffix(path, "/messages"):
			if f.rateLimitOnce.CompareAndSwap(false, true) {
				w.Header().Set("Retry-After", "0.001")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]any{"message": "rate limited"})
				return
			}
			channelID := strings.TrimSuffix(strings.TrimPrefix(path, "/channels/"), "/messages")
			if f.forbidChannels[channelID] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			all := f.messages[channelID]
			start := 0
			if before := query.Get("before"); before != "" {
				for i, message := range all {
					if message["id"] == before {
						start = i + 1
						break
					}
				}
			}
			limit := 100
			end := start + limit
			if end > len(all) {
				end = len(all)
			}
			json.NewEncoder(w).Encode(all[start:end])
			return
		default:
			http.NotFound(w, r)
		}
	})
}

func fixtureMessage(id, content, author string, ts time.Time) map[string]any {
	return map[string]any{
		"id":               id,
		"content":          content,
		"type":             0,
		"timestamp":        ts.Format("2006-01-02T15:04:05.000000+00:00"),
		"edited_timestamp": nil,
		"author":           map[string]any{"id": "u-" + id, "username": author, "bot": false},
	}
}

func newDiscordTestConnector(t *testing.T, config map[string]any) *DiscordConnector {
	t.Helper()
	connector, err := NewDiscordConnector(config)
	if err != nil {
		t.Fatalf("NewDiscordConnector: %v", err)
	}
	return connector
}

func TestDiscordConfigParsing(t *testing.T) {
	connector := newDiscordTestConnector(t, map[string]any{
		"server_ids":  []any{"1", "2"},
		"channels":    []any{"general", "announcements"},
		"batch_size":  7,
		"start_date":  "2026-01-02",
		"credentials": map[string]any{"discord_bot_token": "Bot token-1"},
	})
	if connector.batchSize != 7 {
		t.Fatalf("batchSize = %d, want 7", connector.batchSize)
	}
	if len(connector.serverIDs) != 2 {
		t.Fatalf("serverIDs = %d, want 2", len(connector.serverIDs))
	}
	if _, ok := connector.serverIDs["2"]; !ok {
		t.Fatalf("serverIDs missing id 2")
	}
	if len(connector.channelNames) != 2 || connector.channelNames[0] != "general" {
		t.Fatalf("channelNames = %v", connector.channelNames)
	}
	if connector.startDate.Format("2006-01-02") != "2026-01-02" {
		t.Fatalf("startDate = %v", connector.startDate)
	}
	if connector.token != "token-1" {
		t.Fatalf("token = %q, want token-1", connector.token)
	}

	// comma-separated strings and the legacy channel_names key are accepted too.
	legacy := newDiscordTestConnector(t, map[string]any{
		"server_ids":    "1, 3",
		"channel_names": "general",
		"credentials":   map[string]any{"discord_bot_token": "token-2"},
	})
	if len(legacy.serverIDs) != 2 {
		t.Fatalf("legacy serverIDs = %d, want 2", len(legacy.serverIDs))
	}
	if len(legacy.channelNames) != 1 || legacy.channelNames[0] != "general" {
		t.Fatalf("legacy channelNames = %v", legacy.channelNames)
	}

	// default start date is clamped to the earliest representable Discord time.
	def := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token-3"},
	})
	if !def.startDate.Equal(discordEpoch) {
		t.Fatalf("default startDate = %v, want %v", def.startDate, discordEpoch)
	}
}

func TestDiscordConfigParsingInvalidServerID(t *testing.T) {
	for _, serverIDs := range []any{
		[]any{"1001", "not-a-number"},
		"1001, not-a-number",
	} {
		_, err := NewDiscordConnector(map[string]any{
			"server_ids":  serverIDs,
			"credentials": map[string]any{"discord_bot_token": "token"},
		})
		var valErr *ConnectorValidationError
		if !errors.As(err, &valErr) {
			t.Fatalf("NewDiscordConnector(server_ids=%v) err = %v, want ConnectorValidationError", serverIDs, err)
		}
	}
}

func TestDiscordValidateNoNetwork(t *testing.T) {
	fixture := newDiscordFixture()
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
	})
	connector.baseURL = server.URL
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if fixture.requests.Load() != 0 {
		t.Fatalf("Validate made %d network requests, want 0", fixture.requests.Load())
	}

	missing := newDiscordTestConnector(t, map[string]any{"credentials": map[string]any{}})
	var credErr *ConnectorMissingCredentialError
	if err := missing.Validate(context.Background()); !errors.As(err, &credErr) {
		t.Fatalf("Validate missing token err = %v, want ConnectorMissingCredentialError", err)
	}
}

func TestDiscordConnectorValidateConnectorSetting(t *testing.T) {
	fixture := newDiscordFixture()
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"server_ids":  []any{"1001"},
		"channels":    []any{"general"},
	})
	connector.baseURL = server.URL
	if err := connector.ValidateConnectorSetting(t.Context(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
	if fixture.requests.Load() == 0 {
		t.Fatalf("ValidateConnectorSetting made no network requests")
	}

	empty := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"server_ids":  []any{"1001"},
		"channels":    []any{"missing"},
	})
	empty.baseURL = server.URL
	var valErr *ConnectorValidationError
	if err := empty.ValidateConnectorSetting(t.Context(), nil); !errors.As(err, &valErr) {
		t.Fatalf("ValidateConnectorSetting empty err = %v, want ConnectorValidationError", err)
	}
}

func TestDiscordOpenSyncMergesBatches(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1",
		fixtureMessage("m1", "one", "alice", base),
		fixtureMessage("m2", "two", "bob", base.Add(-time.Minute)),
		fixtureMessage("m3", "three", "carol", base.Add(-2*time.Minute)),
		fixtureMessage("m4", "four", "dave", base.Add(-3*time.Minute)),
	)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  2,
	})
	connector.baseURL = server.URL

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents = %d, want 1 merged document", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "DISCORD_m1" {
		t.Fatalf("SourceID = %q, want DISCORD_m1", doc.SourceID)
	}
	if string(doc.Blob) != "one\n\ntwo" {
		t.Fatalf("blob = %q", string(doc.Blob))
	}
	if doc.SizeBytes != 6 {
		t.Fatalf("size = %d, want 6 (sum of message contents)", doc.SizeBytes)
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	if doc.Metadata == nil || doc.Metadata["Channel"] != "general" {
		t.Fatalf("metadata = %v", doc.Metadata)
	}
	if !doc.UpdatedAt.Equal(base) {
		t.Fatalf("UpdatedAt = %v, want %v", doc.UpdatedAt, base)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second NextBatch: %v", err)
	}
	if second.Documents[0].SourceID != "DISCORD_m3" || string(second.Documents[0].Blob) != "three\n\nfour" {
		t.Fatalf("second doc = %q %q", second.Documents[0].SourceID, second.Documents[0].Blob)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v, want io.EOF", err)
	}
}

func TestDiscordOpenSyncWindowFilter(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1",
		fixtureMessage("m1", "new", "alice", base),
		fixtureMessage("m2", "mid-1", "bob", base.Add(-time.Minute)),
		fixtureMessage("m3", "mid-2", "carol", base.Add(-2*time.Minute)),
		fixtureMessage("m4", "old", "dave", base.Add(-3*time.Minute)),
	)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  10,
	})
	connector.baseURL = server.URL

	start := base.Add(-2 * time.Minute)
	end := base
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: false,
		WindowStart:   &start,
		WindowEnd:     end,
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents = %d, want 1", len(batch.Documents))
	}
	if string(batch.Documents[0].Blob) != "mid-1\n\nmid-2" {
		t.Fatalf("blob = %q, want mid-1 + mid-2", batch.Documents[0].Blob)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v, want io.EOF", err)
	}
}

func TestDiscordOpenSyncChannelsAndThreads(t *testing.T) {
	fixture := newDiscordFixture()
	fixture.forbidPrivateArchived = true
	fixture.forbidChannels = map[string]bool{"ch-3": true}
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1", fixtureMessage("m-ch", "channel msg", "alice", base))
	fixture.setMessages("thread-a", fixtureMessage("m-ta", "active thread msg", "bob", base))
	fixture.setMessages("thread-b", fixtureMessage("m-tb", "archived thread msg", "carol", base))
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"server_ids":  []any{"1001"},
		"channels":    []any{"general", "mod-only"},
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  10,
	})
	connector.baseURL = server.URL

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	var blobs []string
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		blobs = append(blobs, string(batch.Documents[0].Blob))
	}
	want := []string{"channel msg", "archived thread msg", "active thread msg"}
	if len(blobs) != len(want) {
		t.Fatalf("documents = %v, want %v", blobs, want)
	}
	for i := range want {
		if blobs[i] != want[i] {
			t.Fatalf("document %d = %q, want %q", i, blobs[i], want[i])
		}
	}
}

func TestDiscordThreadListErrorsPropagate(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail func(*discordFixture)
	}{
		{"archived", func(f *discordFixture) { f.failArchivedThreads = true }},
		{"active", func(f *discordFixture) { f.failActiveThreads = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDiscordFixture()
			tc.fail(fixture)
			server := httptest.NewServer(fixture.handler())
			defer server.Close()

			connector := newDiscordTestConnector(t, map[string]any{
				"credentials": map[string]any{"discord_bot_token": "token"},
			})
			connector.baseURL = server.URL

			if _, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true}); err == nil {
				t.Fatalf("OpenSync: want error, got nil")
			}
			if _, err := connector.OpenPrune(t.Context(), PruneRequest{}); err == nil {
				t.Fatalf("OpenPrune: want error, got nil")
			}
		})
	}
}

func TestDiscordResumeSameTarget(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1",
		fixtureMessage("m1", "one", "alice", base),
		fixtureMessage("m2", "two", "bob", base.Add(-time.Minute)),
		fixtureMessage("m3", "three", "carol", base.Add(-2*time.Minute)),
		fixtureMessage("m4", "four", "dave", base.Add(-3*time.Minute)),
	)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	newConnector := func() *DiscordConnector {
		connector := newDiscordTestConnector(t, map[string]any{
			"credentials": map[string]any{"discord_bot_token": "token"},
			"batch_size":  2,
		})
		connector.baseURL = server.URL
		return connector
	}

	session, err := newConnector().OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	session.Close()
	if batch.Checkpoint == nil {
		t.Fatalf("batch checkpoint is nil")
	}
	var cursor discordSyncCursor
	if err := json.Unmarshal([]byte(batch.Checkpoint.Cursor), &cursor); err != nil {
		t.Fatalf("cursor unmarshal: %v", err)
	}
	if cursor.Target != "ch-1" || cursor.Message != "m2" || cursor.Targets == "" {
		t.Fatalf("cursor = %+v, want target ch-1 message m2", cursor)
	}

	resumed, err := newConnector().OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		Resume:        batch.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resumed OpenSync: %v", err)
	}
	defer resumed.Close()

	first, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch: %v", err)
	}
	if first.Documents[0].SourceID != "DISCORD_m3" || string(first.Documents[0].Blob) != "three\n\nfour" {
		t.Fatalf("resumed doc = %q %q, want DISCORD_m3 three+four", first.Documents[0].SourceID, first.Documents[0].Blob)
	}
	if _, err := resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resumed NextBatch EOF = %v, want io.EOF", err)
	}
}

func TestDiscordResumeAcrossTargets(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1",
		fixtureMessage("m1", "one", "alice", base),
		fixtureMessage("m2", "two", "bob", base.Add(-time.Minute)),
		fixtureMessage("m3", "three", "carol", base.Add(-2*time.Minute)),
		fixtureMessage("m4", "four", "dave", base.Add(-3*time.Minute)),
	)
	fixture.setMessages("thread-a",
		fixtureMessage("t1", "thread one", "eve", base.Add(-4*time.Minute)),
		fixtureMessage("t2", "thread two", "frank", base.Add(-5*time.Minute)),
	)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	newConnector := func() *DiscordConnector {
		connector := newDiscordTestConnector(t, map[string]any{
			"server_ids":  []any{"1001"},
			"channels":    []any{"general"},
			"credentials": map[string]any{"discord_bot_token": "token"},
			"batch_size":  3,
		})
		connector.baseURL = server.URL
		return connector
	}

	session, err := newConnector().OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	session.Close()
	if batch.Checkpoint == nil {
		t.Fatalf("batch checkpoint is nil")
	}

	resumed, err := newConnector().OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		Resume:        batch.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resumed OpenSync: %v", err)
	}
	defer resumed.Close()

	first, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch: %v", err)
	}
	if first.Documents[0].SourceID != "DISCORD_m4" || string(first.Documents[0].Blob) != "four" {
		t.Fatalf("resumed doc = %q %q, want DISCORD_m4", first.Documents[0].SourceID, first.Documents[0].Blob)
	}

	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed second NextBatch: %v", err)
	}
	if second.Documents[0].SourceID != "DISCORD_t1" || string(second.Documents[0].Blob) != "thread one\n\nthread two" {
		t.Fatalf("resumed second doc = %q %q", second.Documents[0].SourceID, second.Documents[0].Blob)
	}
	if _, err := resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resumed NextBatch EOF = %v, want io.EOF", err)
	}
}

func TestDiscordResumeFingerprintMismatch(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1",
		fixtureMessage("m1", "one", "alice", base),
		fixtureMessage("m2", "two", "bob", base.Add(-time.Minute)),
	)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	// Run 1 enumerates all text channels of the guild.
	firstConnector := newDiscordTestConnector(t, map[string]any{
		"server_ids":  []any{"1001"},
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  2,
	})
	firstConnector.baseURL = server.URL
	session, err := firstConnector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	session.Close()

	// Run 2 filters to one channel, changing the enumeration: resume must be
	// rejected so the runner can restart the same fixed window.
	secondConnector := newDiscordTestConnector(t, map[string]any{
		"server_ids":  []any{"1001"},
		"channels":    []any{"general"},
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  2,
	})
	secondConnector.baseURL = server.URL
	resumed, err := secondConnector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		Resume:        batch.Checkpoint,
	})
	if resumed != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resumed OpenSync = session %v, err %v, want ErrSyncResumeInvalid", resumed, err)
	}
}

func TestDiscordResumeInvalidCursor(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1", fixtureMessage("m1", "one", "alice", base))
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  2,
	})
	connector.baseURL = server.URL

	session, err := connector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: "not-json"},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestDiscordMessagePagination(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	messages := make([]map[string]any, 0, 250)
	for i := 0; i < 250; i++ {
		messages = append(messages, fixtureMessage(
			fmt.Sprintf("m%03d", i),
			fmt.Sprintf("content %d", i),
			"alice",
			base.Add(-time.Duration(i)*time.Minute),
		))
	}
	fixture.setMessages("ch-1", messages...)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  10,
	})
	connector.baseURL = server.URL

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	total := 0
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		total += len(batch.Documents[0].Blob)
	}
	if total == 0 {
		t.Fatalf("no documents streamed")
	}
	if fixture.requests.Load() < 3 {
		t.Fatalf("message requests = %d, want pagination across multiple pages", fixture.requests.Load())
	}
}

func TestDiscord429Retry(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1", fixtureMessage("m1", "hello", "alice", base))
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  10,
	})
	connector.baseURL = server.URL

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch after 429: %v", err)
	}
	if string(batch.Documents[0].Blob) != "hello" {
		t.Fatalf("blob = %q", batch.Documents[0].Blob)
	}
}

func TestDiscordPruneSlimDocuments(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1",
		fixtureMessage("m1", "one", "alice", base),
		fixtureMessage("m2", "two", "bob", base.Add(-time.Minute)),
		fixtureMessage("m3", "three", "carol", base.Add(-2*time.Minute)),
		fixtureMessage("m4", "four", "dave", base.Add(-3*time.Minute)),
		fixtureMessage("m5", "five", "eve", base.Add(-4*time.Minute)),
	)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  2,
	})
	connector.baseURL = server.URL

	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()

	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(first.Documents) != 2 {
		t.Fatalf("first slim batch = %d, want 2", len(first.Documents))
	}
	if first.Documents[0].SourceID != "DISCORD_m1" || first.Documents[1].SourceID != "DISCORD_m3" {
		t.Fatalf("slim ids = %q, %q", first.Documents[0].SourceID, first.Documents[1].SourceID)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second NextBatch: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "DISCORD_m5" {
		t.Fatalf("second slim batch = %v", second.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v, want io.EOF", err)
	}
}

func TestDiscordPruneGroupBreaksOnTargetSwitch(t *testing.T) {
	fixture := newDiscordFixture()
	base := mustTime(t, "2026-08-14T10:00:00Z")
	fixture.setMessages("ch-1",
		fixtureMessage("m1", "one", "alice", base),
		fixtureMessage("m2", "two", "bob", base.Add(-time.Minute)),
	)
	fixture.setMessages("thread-a",
		fixtureMessage("m3", "three", "carol", base.Add(-2*time.Minute)),
		fixtureMessage("m4", "four", "dave", base.Add(-3*time.Minute)),
	)
	server := httptest.NewServer(fixture.handler())
	defer server.Close()

	connector := newDiscordTestConnector(t, map[string]any{
		"credentials": map[string]any{"discord_bot_token": "token"},
		"batch_size":  10,
	})
	connector.baseURL = server.URL

	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	defer session.Close()

	var ids []string
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		for _, doc := range batch.Documents {
			ids = append(ids, doc.SourceID)
		}
	}
	want := []string{"DISCORD_m1", "DISCORD_m3"}
	if len(ids) != len(want) {
		t.Fatalf("slim ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("slim id %d = %q, want %q", i, ids[i], want[i])
		}
	}
}

func TestDiscordFingerprintStable(t *testing.T) {
	base := mustTime(t, "2026-08-14T10:00:00Z")
	target := discordTarget{channelID: "ch-1", name: "general"}
	item := discordMessageWithTarget{
		message: discordMessage{
			ID:        "m1",
			Content:   "hello",
			Type:      0,
			Timestamp: base.Format("2006-01-02T15:04:05.000000+00:00"),
			Author: struct {
				ID   string `json:"id"`
				Name string `json:"username"`
				Bot  bool   `json:"bot"`
			}{ID: "u1", Name: "alice"},
		},
		target: target,
	}

	first := discordMessageDocument(item)
	second := discordMessageDocument(item)
	if first.Fingerprint == "" || first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprint unstable: %q %q", first.Fingerprint, second.Fingerprint)
	}
	if first.SemanticIdentifier != "alice said in Channel: #general: hello" {
		t.Fatalf("semantic identifier = %q", first.SemanticIdentifier)
	}

	edited := item
	edited.message.Content = "hello world"
	if changed := discordMessageDocument(edited); changed.Fingerprint == first.Fingerprint {
		t.Fatalf("fingerprint did not change with content")
	}
}
