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
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Discord connector constants.
const (
	discordDefaultBaseURL   = "https://discord.com/api/v10"
	discordDefaultBatchSize = 1024
	discordRequestTimeout   = 60 * time.Second
	discordMessagePageSize  = 100
	discordMaxGuildsPage    = 200
	discord429MaxWaits      = 10
	discordMaxRetryAfter    = 60 * time.Second
	discordSnippetLength    = 30
	discordDocIDPrefix      = "DISCORD_"
)

// discordChannelTypeGuildText is the REST channel type of a text channel.
const discordChannelTypeGuildText = 0

// discordEpoch is the earliest timestamp Discord can represent.
var discordEpoch = time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)

// DiscordConnector reads messages from Discord guild text channels and
// their threads through the REST API with a bot token.
type DiscordConnector struct {
	token        string
	serverIDs    map[string]struct{}
	channelNames []string
	batchSize    int
	startDate    time.Time
	baseURL      string
	client       *http.Client
}

// discordTarget is one message source: either a text channel or a thread.
type discordTarget struct {
	channelID string
	name      string
	isThread  bool
}

// discordGuild is a Discord server returned by the guilds endpoint.
type discordGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// discordChannel is a channel or thread object from the REST API.
type discordChannel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	GuildID  string `json:"guild_id"`
	ParentID string `json:"parent_id"`
}

// discordThreadsResponse is the archived-threads paginated payload.
type discordThreadsResponse struct {
	Threads []discordChannel `json:"threads"`
	HasMore bool             `json:"has_more"`
}

// discordActiveThreadsResponse is the active-threads payload.
type discordActiveThreadsResponse struct {
	Threads []discordChannel `json:"threads"`
}

// discordMessage is a Discord message from the messages endpoint.
type discordMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
	Type      int    `json:"type"`
	Timestamp string `json:"timestamp"`
	EditedAt  string `json:"edited_timestamp"`
	Author    struct {
		ID   string `json:"id"`
		Name string `json:"username"`
		Bot  bool   `json:"bot"`
	} `json:"author"`
}

// discordMessageWithTarget pairs a message with the source it came from.
type discordMessageWithTarget struct {
	message discordMessage
	target  discordTarget
}

// NewDiscordConnector parses a stored connector config into a connector.
func NewDiscordConnector(config map[string]any) (*DiscordConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	token := discordBotToken(stringConfig(credentials["discord_bot_token"]))

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("DISCORD_CONNECTOR_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = discordDefaultBaseURL
	}

	serverIDs := map[string]struct{}{}
	for _, raw := range coerceDiscordStringList(config["server_ids"]) {
		id := strings.TrimSpace(raw)
		if !discordValidServerID(id) {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("Invalid Discord server_ids entry %q; expected a numeric server ID", raw)}
		}
		serverIDs[id] = struct{}{}
	}

	channelNames := coerceDiscordStringList(config["channels"])
	if len(channelNames) == 0 {
		channelNames = coerceDiscordStringList(config["channel_names"])
	}

	batchSize := configInt(config["batch_size"], discordDefaultBatchSize)
	if batchSize <= 0 {
		batchSize = discordDefaultBatchSize
	}

	startDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if raw := strings.TrimSpace(stringConfig(config["start_date"])); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			startDate = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
		}
	}
	if startDate.Before(discordEpoch) {
		startDate = discordEpoch
	}

	return &DiscordConnector{
		token:        token,
		serverIDs:    serverIDs,
		channelNames: channelNames,
		batchSize:    batchSize,
		startDate:    startDate,
		baseURL:      baseURL,
		client:       &http.Client{Timeout: discordRequestTimeout},
	}, nil
}

// discordValidServerID reports whether s is a plausible Discord snowflake ID.
func discordValidServerID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// coerceDiscordStringList normalizes a config field that may be a list or a
// comma-separated string into a clean list of non-empty values.
func coerceDiscordStringList(value any) []string {
	var rawItems []any
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		for _, part := range strings.Split(typed, ",") {
			rawItems = append(rawItems, part)
		}
	case []any:
		rawItems = typed
	case []string:
		for _, item := range typed {
			rawItems = append(rawItems, item)
		}
	default:
		rawItems = append(rawItems, typed)
	}

	out := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		if item == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

// discordBotToken strips an optional "Bot " or "Bearer " prefix.
func discordBotToken(token string) string {
	token = strings.TrimSpace(token)
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "bot ") {
		return strings.TrimSpace(token[4:])
	}
	if strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(token[7:])
	}
	return token
}

// discordAuthorizationHeader builds the REST Authorization header value.
func discordAuthorizationHeader(token string) string {
	token = discordBotToken(token)
	if token == "" {
		return ""
	}
	return "Bot " + token
}

// Validate checks configuration and credential presence without network I/O.
func (c *DiscordConnector) Validate(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "discord connector is nil"}
	}
	if strings.TrimSpace(c.token) == "" {
		return &ConnectorMissingCredentialError{Message: "Discord connector requires 'discord_bot_token' in credentials"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "Discord connector batch_size must be a positive integer"}
	}
	return nil
}

// ValidateConnectorSetting validates Discord settings from an unsaved config.
func (c *DiscordConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if err := c.Validate(ctx); err != nil {
		return err
	}
	targets, err := c.listTargets(ctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return &ConnectorValidationError{Message: "Discord connector found no accessible text channels"}
	}
	return nil
}

// OpenSync opens one sync session over the configured channels and threads.
func (c *DiscordConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	targets, err := c.listTargets(ctx)
	if err != nil {
		return nil, err
	}

	lowerBound := c.startDate
	if !request.FromBeginning && request.WindowStart != nil && request.WindowStart.After(lowerBound) {
		lowerBound = *request.WindowStart
	}
	upperBound := time.Time{}
	if !request.FromBeginning {
		upperBound = request.WindowEnd
	}

	resumeTarget, resumeBefore, err := discordResumePosition(targets, request.Resume)
	if err != nil {
		return nil, err
	}
	return &discordSyncSession{
		connector:          c,
		iter:               newDiscordMessageIterator(c, targets, lowerBound, upperBound, resumeTarget, resumeBefore),
		targetsFingerprint: discordCursorFingerprint(targets),
	}, nil
}

// discordResumePosition returns the target and before cursor to continue from,
// or ErrSyncResumeInvalid when the checkpoint does not match the current
// enumeration.
func discordResumePosition(targets []discordTarget, checkpoint *SyncCheckpoint) (string, string, error) {
	if checkpoint == nil {
		return "", "", nil
	}
	if checkpoint.Cursor == "" {
		return "", "", fmt.Errorf("discord sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor discordSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return "", "", fmt.Errorf("discord sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	if cursor.Target == "" || cursor.Message == "" {
		return "", "", fmt.Errorf("discord sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.Targets == "" {
		return "", "", fmt.Errorf("discord sync cursor has no target listing: %w", ErrSyncResumeInvalid)
	}
	if cursor.Targets != "" && cursor.Targets != discordCursorFingerprint(targets) {
		return "", "", fmt.Errorf("discord resume anchor %q was not found in the current target listing: %w", cursor.Target, ErrSyncResumeInvalid)
	}
	for _, target := range targets {
		if target.channelID == cursor.Target {
			return cursor.Target, cursor.Message, nil
		}
	}
	return "", "", fmt.Errorf("discord resume anchor %q was not found in the current target listing: %w", cursor.Target, ErrSyncResumeInvalid)
}

// discordSyncCursor is the resume position serialized into SyncCheckpoint.Cursor.
// Target is the channel/thread of the last committed group and Message is the
// oldest message ID of that group, i.e. the next `before` cursor for it.
type discordSyncCursor struct {
	Targets string `json:"targets"`
	Target  string `json:"target"`
	Message string `json:"message"`
}

// discordCursorFingerprint summarizes the ordered target list so a resume can
// detect enumeration changes and reject a stale resume.
func discordCursorFingerprint(targets []discordTarget) string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.channelID)
	}
	return stableFingerprint(ids)
}

func encodeDiscordCursor(fingerprint, target, message string) string {
	raw, err := json.Marshal(discordSyncCursor{Targets: fingerprint, Target: target, Message: message})
	if err != nil {
		return ""
	}
	return string(raw)
}

// OpenPrune opens one complete slim snapshot session.
func (c *DiscordConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	targets, err := c.listTargets(ctx)
	if err != nil {
		return nil, err
	}
	return &discordPruneSession{
		connector: c,
		iter:      newDiscordMessageIterator(c, targets, c.startDate, time.Time{}, "", ""),
	}, nil
}

// listTargets enumerates guild text channels and their threads.
func (c *DiscordConnector) listTargets(ctx context.Context) ([]discordTarget, error) {
	guildIDs := make([]string, 0, len(c.serverIDs))
	if len(c.serverIDs) > 0 {
		for guildID := range c.serverIDs {
			guildIDs = append(guildIDs, guildID)
		}
	} else {
		guilds, err := c.listGuilds(ctx)
		if err != nil {
			return nil, err
		}
		for _, guild := range guilds {
			guildIDs = append(guildIDs, guild.ID)
		}
	}
	sort.Strings(guildIDs)

	var targets []discordTarget
	for _, guildID := range guildIDs {
		var channels []discordChannel
		if status, err := c.getJSON(ctx, http.MethodGet, "/guilds/"+url.PathEscape(guildID)+"/channels", nil, &channels); err != nil {
			if status == http.StatusForbidden {
				continue
			}
			return nil, err
		}

		selected := map[string]discordChannel{}
		var textChannels []discordChannel
		for _, ch := range channels {
			if ch.Type != discordChannelTypeGuildText {
				continue
			}
			if len(c.channelNames) > 0 && !discordContainsString(c.channelNames, ch.Name) {
				continue
			}
			selected[ch.ID] = ch
			textChannels = append(textChannels, ch)
		}

		for _, ch := range textChannels {
			targets = append(targets, discordTarget{channelID: ch.ID, name: ch.Name})
			for _, kind := range []string{"public", "private"} {
				threads, status, err := c.listArchivedThreads(ctx, ch.ID, kind)
				if err != nil {
					if status == http.StatusForbidden {
						continue
					}
					return nil, err
				}
				for _, thread := range threads {
					targets = append(targets, discordTarget{channelID: thread.ID, name: thread.Name, isThread: true})
				}
			}
		}

		activeThreads, status, err := c.listActiveThreads(ctx, guildID)
		if err != nil {
			if status == http.StatusForbidden {
				continue
			}
			return nil, err
		}
		for _, thread := range activeThreads {
			if _, ok := selected[thread.ParentID]; ok {
				targets = append(targets, discordTarget{channelID: thread.ID, name: thread.Name, isThread: true})
			}
		}
	}
	return targets, nil
}

// listGuilds returns all guilds the bot can see.
func (c *DiscordConnector) listGuilds(ctx context.Context) ([]discordGuild, error) {
	var out []discordGuild
	after := ""
	for {
		query := url.Values{"limit": {strconv.Itoa(discordMaxGuildsPage)}}
		if after != "" {
			query.Set("after", after)
		}
		var batch []discordGuild
		if _, err := c.getJSON(ctx, http.MethodGet, "/users/@me/guilds", query, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		out = append(out, batch...)
		after = batch[len(batch)-1].ID
		if len(batch) < discordMaxGuildsPage {
			break
		}
	}
	return out, nil
}

// listActiveThreads returns all active threads of a guild.
func (c *DiscordConnector) listActiveThreads(ctx context.Context, guildID string) ([]discordChannel, int, error) {
	var resp discordActiveThreadsResponse
	status, err := c.getJSON(ctx, http.MethodGet, "/guilds/"+url.PathEscape(guildID)+"/threads/active", nil, &resp)
	if err != nil {
		return nil, status, err
	}
	return resp.Threads, status, nil
}

// listArchivedThreads paginates archived public or private threads of a channel.
func (c *DiscordConnector) listArchivedThreads(ctx context.Context, channelID, kind string) ([]discordChannel, int, error) {
	var out []discordChannel
	before := ""
	for {
		query := url.Values{"limit": {strconv.Itoa(discordMessagePageSize)}}
		if before != "" {
			query.Set("before", before)
		}
		var resp discordThreadsResponse
		status, err := c.getJSON(ctx, http.MethodGet, "/channels/"+url.PathEscape(channelID)+"/threads/archived/"+kind, query, &resp)
		if err != nil {
			return nil, status, err
		}
		if len(resp.Threads) == 0 {
			break
		}
		out = append(out, resp.Threads...)
		if !resp.HasMore {
			break
		}
		before = resp.Threads[len(resp.Threads)-1].ID
	}
	return out, http.StatusOK, nil
}

// getJSON performs one authenticated Discord REST request with 429 handling.
// The returned status code lets callers distinguish permission errors.
func (c *DiscordConnector) getJSON(ctx context.Context, method, path string, query url.Values, out any) (int, error) {
	fullURL := strings.TrimRight(c.baseURL, "/") + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", discordAuthorizationHeader(c.token))
	req.Header.Set("Accept", "application/json")

	for wait := 0; wait < discord429MaxWaits; wait++ {
		resp, err := c.client.Do(req)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := discordRetryAfter(resp)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(retryAfter):
			}
			continue
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		resp.Body.Close()
		if readErr != nil {
			return resp.StatusCode, readErr
		}
		if resp.StatusCode >= 500 {
			return resp.StatusCode, fmt.Errorf("discord api request failed with http %d", resp.StatusCode)
		}
		if resp.StatusCode >= 400 {
			return resp.StatusCode, fmt.Errorf("discord api request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		if out != nil && len(bytes.TrimSpace(respBody)) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return resp.StatusCode, &ConnectorValidationError{Message: "discord api response is not valid JSON"}
			}
		}
		return resp.StatusCode, nil
	}
	return 0, &RateLimitTriedTooManyTimesError{Message: fmt.Sprintf("Discord API rate limited: exceeded %d retries (too many requests)", discord429MaxWaits)}
}

// discordRetryAfter extracts the wait duration from a 429 response.
func discordRetryAfter(resp *http.Response) time.Duration {
	for _, key := range []string{"Retry-After", "X-RateLimit-Reset-After"} {
		if raw := resp.Header.Get(key); raw != "" {
			if seconds, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && seconds >= 0 {
				retryAfter := time.Duration(seconds * float64(time.Second))
				if retryAfter > discordMaxRetryAfter {
					return discordMaxRetryAfter
				}
				return retryAfter
			}
		}
	}
	return 0
}

// discordContainsString reports whether a slice contains a value.
func discordContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// discordMessageIterator walks messages of the enumerated targets in
// newest-to-oldest order, honoring a [lowerBound, upperBound) window.
type discordMessageIterator struct {
	connector  *DiscordConnector
	targets    []discordTarget
	lowerBound time.Time
	upperBound time.Time

	targetIndex int
	page        []discordMessage
	before      string
	targetDone  bool
}

func newDiscordMessageIterator(c *DiscordConnector, targets []discordTarget, lowerBound, upperBound time.Time, resumeTarget, resumeBefore string) *discordMessageIterator {
	it := &discordMessageIterator{
		connector:  c,
		targets:    targets,
		lowerBound: lowerBound,
		upperBound: upperBound,
	}
	if resumeTarget != "" {
		for i, target := range targets {
			if target.channelID == resumeTarget {
				it.targetIndex = i
				it.before = resumeBefore
				break
			}
		}
	}
	return it
}

// next returns the next in-window message, or io.EOF when exhausted.
func (it *discordMessageIterator) next(ctx context.Context) (discordMessageWithTarget, error) {
	for {
		if it.targetIndex >= len(it.targets) {
			return discordMessageWithTarget{}, io.EOF
		}
		target := it.targets[it.targetIndex]

		if len(it.page) == 0 && !it.targetDone {
			query := url.Values{"limit": {strconv.Itoa(discordMessagePageSize)}}
			if it.before != "" {
				query.Set("before", it.before)
			}
			var messages []discordMessage
			if status, err := it.connector.getJSON(ctx, http.MethodGet, "/channels/"+url.PathEscape(target.channelID)+"/messages", query, &messages); err != nil {
				if status == http.StatusForbidden {
					it.advanceTarget()
					continue
				}
				return discordMessageWithTarget{}, err
			}
			if len(messages) == 0 {
				it.advanceTarget()
				continue
			}
			it.page = messages
			it.before = messages[len(messages)-1].ID
		}

		if it.targetDone && len(it.page) == 0 {
			it.advanceTarget()
			continue
		}

		msg := it.page[0]
		it.page = it.page[1:]

		createdAt := discordMessageCreatedAt(msg)
		if createdAt.IsZero() {
			continue
		}
		if !it.upperBound.IsZero() && !createdAt.Before(it.upperBound) {
			continue
		}
		if createdAt.Before(it.lowerBound) {
			it.targetDone = true
			it.page = nil
			continue
		}
		return discordMessageWithTarget{message: msg, target: target}, nil
	}
}

func (it *discordMessageIterator) advanceTarget() {
	it.targetIndex++
	it.page = nil
	it.before = ""
	it.targetDone = false
}

// discordMessageCreatedAt parses the Discord RFC3339 timestamp as UTC.
func discordMessageCreatedAt(msg discordMessage) time.Time {
	created, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return created.UTC()
}

// discordMessageUpdatedAt prefers the edit timestamp, falling back to created.
func discordMessageUpdatedAt(msg discordMessage) time.Time {
	if strings.TrimSpace(msg.EditedAt) != "" {
		if edited, err := time.Parse(time.RFC3339Nano, msg.EditedAt); err == nil {
			return edited.UTC()
		}
	}
	return discordMessageCreatedAt(msg)
}

// discordSyncSession streams merged documents for one sync window.
type discordSyncSession struct {
	connector          *DiscordConnector
	iter               *discordMessageIterator
	targetsFingerprint string
	pending            []discordMessageWithTarget
	currentTarget      string
	carry              *discordMessageWithTarget
}

// NextBatch returns one merged document per call until io.EOF.
func (s *discordSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	for {
		for len(s.pending) < s.connector.batchSize {
			var item discordMessageWithTarget
			if s.carry != nil {
				item = *s.carry
				s.carry = nil
			} else {
				next, err := s.iter.next(ctx)
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return SyncBatch{}, err
				}
				item = next
			}
			if s.currentTarget != "" && item.target.channelID != s.currentTarget {
				s.carry = &item
				s.currentTarget = ""
				break
			}
			s.currentTarget = item.target.channelID
			s.pending = append(s.pending, item)
		}
		if len(s.pending) > 0 {
			break
		}
		if s.carry == nil {
			return SyncBatch{}, io.EOF
		}
	}

	docs := make([]SourceDocument, 0, len(s.pending))
	for _, item := range s.pending {
		docs = append(docs, discordMessageDocument(item))
	}
	merged := mergeDiscordDocuments(docs)
	oldest := s.pending[len(s.pending)-1]
	s.pending = nil
	if s.carry != nil {
		s.currentTarget = ""
	}

	batch := SyncBatch{Documents: []SourceDocument{merged}}
	if cursor := encodeDiscordCursor(s.targetsFingerprint, oldest.target.channelID, oldest.message.ID); cursor != "" {
		batch.Checkpoint = &SyncCheckpoint{Cursor: cursor}
	}
	return batch, nil
}

// Close releases the sync session.
func (s *discordSyncSession) Close() error {
	return nil
}

// discordMessageDocument converts one message into a source document.
func discordMessageDocument(item discordMessageWithTarget) SourceDocument {
	content := item.message.Content
	snippet := content
	if runes := []rune(snippet); len(runes) > discordSnippetLength {
		snippet = string(runes[:discordSnippetLength]) + "..."
	}

	semantic := item.message.Author.Name + " said"
	if item.target.isThread {
		semantic += " in Thread: " + item.target.name
	} else {
		semantic += " in Channel: #" + item.target.name
	}
	semantic += ": " + snippet

	updatedAt := discordMessageUpdatedAt(item.message)
	blob := []byte(content)

	var metadata map[string]any
	if !item.target.isThread {
		metadata = map[string]any{"Channel": item.target.name}
	}

	return SourceDocument{
		SourceID:           discordDocIDPrefix + item.message.ID,
		SemanticIdentifier: semantic,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint: stableFingerprint(map[string]any{
			"id":         item.message.ID,
			"content":    content,
			"author":     item.message.Author.Name,
			"updated_at": updatedAt,
		}),
	}
}

// mergeDiscordDocuments merges consecutive messages into one document.
func mergeDiscordDocuments(docs []SourceDocument) SourceDocument {
	first := docs[0]
	minUpdated, maxUpdated := first.UpdatedAt, first.UpdatedAt
	ids := make([]string, 0, len(docs))
	var blob strings.Builder
	var size int64
	for _, doc := range docs {
		if doc.UpdatedAt.Before(minUpdated) {
			minUpdated = doc.UpdatedAt
		}
		if doc.UpdatedAt.After(maxUpdated) {
			maxUpdated = doc.UpdatedAt
		}
		ids = append(ids, doc.SourceID)
		if blob.Len() > 0 {
			blob.WriteString("\n\n")
		}
		blob.Write(doc.Blob)
		size += doc.SizeBytes
	}

	format := "2006-01-02 15:04:05Z07:00"
	return SourceDocument{
		SourceID:           first.SourceID,
		SemanticIdentifier: fmt.Sprintf("%s -> %s", minUpdated.Format(format), maxUpdated.Format(format)),
		Extension:          ".txt",
		Blob:               []byte(blob.String()),
		UpdatedAt:          maxUpdated,
		SizeBytes:          size,
		Metadata:           first.Metadata,
		Fingerprint: stableFingerprint(map[string]any{
			"ids":        ids,
			"content":    blob.String(),
			"updated_at": maxUpdated,
		}),
	}
}

// discordPruneSession streams a complete slim message snapshot.
type discordPruneSession struct {
	connector *DiscordConnector
	iter      *discordMessageIterator

	groupSize     int
	firstID       string
	currentTarget string
	groupDocs     []SlimDocument
}

// NextBatch returns slim documents grouped by the connector batch size.
func (s *discordPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	for len(s.groupDocs) < s.connector.batchSize {
		item, err := s.iter.next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return PruneBatch{}, err
		}

		if s.currentTarget != "" && item.target.channelID != s.currentTarget {
			s.groupDocs = append(s.groupDocs, SlimDocument{SourceID: s.firstID})
			s.groupSize = 0
			s.firstID = ""
		}
		if s.groupSize == 0 {
			s.firstID = discordDocIDPrefix + item.message.ID
			s.currentTarget = item.target.channelID
		}
		s.groupSize++
		if s.groupSize >= s.connector.batchSize {
			s.groupDocs = append(s.groupDocs, SlimDocument{SourceID: s.firstID})
			s.groupSize = 0
			s.firstID = ""
			s.currentTarget = ""
		}
	}

	if len(s.groupDocs) == 0 && s.groupSize > 0 {
		s.groupDocs = append(s.groupDocs, SlimDocument{SourceID: s.firstID})
		s.groupSize = 0
		s.firstID = ""
		s.currentTarget = ""
	}
	if len(s.groupDocs) == 0 {
		return PruneBatch{}, io.EOF
	}

	batch := s.groupDocs
	s.groupDocs = nil
	return PruneBatch{Documents: batch}, nil
}

// Close releases the prune session.
func (s *discordPruneSession) Close() error {
	return nil
}
