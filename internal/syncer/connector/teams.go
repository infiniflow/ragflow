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
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/utility"
)

const (
	defaultTeamsBatchSize  = 32
	teamsGraphBase         = "https://graph.microsoft.com/v1.0"
	teamsTokenURLFormat    = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	teamsGraphScope        = "https://graph.microsoft.com/.default"
	teamsRequestTimeout    = 60 * time.Second
	teamsRetryCount        = 4
	teamsRetryBaseDelay    = 200 * time.Millisecond
	teamsTokenExpiryMargin = 5 * time.Minute
)

// teamsHTTPError carries a non-2xx Microsoft Graph response.
type teamsHTTPError struct {
	status int
	body   string
}

func (e *teamsHTTPError) Error() string {
	return fmt.Sprintf("Microsoft Graph API returned HTTP %d: %s", e.status, e.body)
}

// TeamsConnector reads Microsoft Teams channel conversations through the
// Microsoft Graph API. It authenticates with an Azure AD app-only
// client-credentials token (Team.ReadBasic.All + ChannelMessage.Read.All).
type TeamsConnector struct {
	tenantID     string
	clientID     string
	clientSecret string
	batchSize    int

	clientMu    sync.Mutex
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	now         func() time.Time

	acquireAccessToken func(ctx context.Context) (string, error)
	doJSON             func(ctx context.Context, apiURL string, out any) error
}

// NewTeamsConnector creates a Teams connector from config.
func NewTeamsConnector(config map[string]any) (*TeamsConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	return &TeamsConnector{
		tenantID:     strings.TrimSpace(stringConfig(credentials["tenant_id"])),
		clientID:     strings.TrimSpace(stringConfig(credentials["client_id"])),
		clientSecret: stringConfig(credentials["client_secret"]),
		batchSize:    teamsBatchSize(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"]))),
		httpClient:   http.DefaultClient,
		now:          time.Now,
	}, nil
}

// teamsBatchSize preserves explicit non-positive values so validation can
// reject them; only missing/unparseable values fall back to the default.
func teamsBatchSize(value string) int {
	if value == "" {
		return defaultTeamsBatchSize
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return defaultTeamsBatchSize
	}
	return parsed
}

// Validate validates Teams connector settings and credentials.
func (c *TeamsConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("teams connector is nil")
	}
	if c.tenantID == "" || c.clientID == "" || c.clientSecret == "" {
		return &ConnectorMissingCredentialError{Message: "Microsoft Teams credentials are incomplete: tenant_id, client_id, and client_secret are required"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}
	if _, err := c.token(ctx); err != nil {
		return err
	}
	var page teamsPage
	if err := c.getJSON(ctx, teamsGraphBase+"/teams", &page); err != nil {
		var httpErr *teamsHTTPError
		if errors.As(err, &httpErr) {
			if httpErr.status == http.StatusUnauthorized || httpErr.status == http.StatusForbidden {
				return &ConnectorValidationError{Message: "Invalid credentials or insufficient permissions for Microsoft Teams (requires Team.ReadBasic.All and ChannelMessage.Read.All application permissions)"}
			}
			return &ConnectorValidationError{Message: fmt.Sprintf("Microsoft Teams validation error (HTTP %d): %s", httpErr.status, httpErr.body)}
		}
		return &ConnectorValidationError{Message: fmt.Sprintf("Microsoft Teams validation error: %v", err)}
	}
	return nil
}

// ValidateConnectorSetting validates Teams settings from an unsaved config.
func (c *TeamsConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

func (c *TeamsConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return defaultTeamsBatchSize
}

// OpenSync opens one Teams sync session.
func (c *TeamsConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	teams, err := c.listTeams(ctx)
	if err != nil {
		return nil, err
	}
	session := &teamsSyncSession{
		connector:   c,
		teams:       teams,
		batchSize:   c.effectiveBatchSize(),
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
	}
	if request.FromBeginning {
		session.windowStart = nil
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Teams prune snapshot session.
func (c *TeamsConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	teams, err := c.listTeams(ctx)
	if err != nil {
		return nil, err
	}
	return &teamsPruneSession{
		connector: c,
		teams:     teams,
		batchSize: c.effectiveBatchSize(),
	}, nil
}

// listTeams returns every team, following @odata.nextLink pages.
func (c *TeamsConnector) listTeams(ctx context.Context) ([]teamsTeam, error) {
	var teams []teamsTeam
	apiURL := teamsGraphBase + "/teams"
	for apiURL != "" {
		var page teamsPage
		if err := c.getJSON(ctx, apiURL, &page); err != nil {
			return nil, err
		}
		teams = append(teams, page.Value...)
		apiURL = page.NextLink
	}
	return teams, nil
}

// listChannels returns every channel of a team, following @odata.nextLink pages.
func (c *TeamsConnector) listChannels(ctx context.Context, teamID string) ([]teamsChannel, error) {
	var channels []teamsChannel
	apiURL := teamsGraphBase + "/teams/" + url.PathEscape(teamID) + "/channels"
	for apiURL != "" {
		var page teamsChannelsPage
		if err := c.getJSON(ctx, apiURL, &page); err != nil {
			return nil, err
		}
		channels = append(channels, page.Value...)
		apiURL = page.NextLink
	}
	return channels, nil
}

// messagesPage fetches one page of channel messages.
func (c *TeamsConnector) messagesPage(ctx context.Context, apiURL string) (teamsMessagesPage, error) {
	var page teamsMessagesPage
	err := c.getJSON(ctx, apiURL, &page)
	return page, err
}

// listReplies returns every reply of a channel message, following pages.
func (c *TeamsConnector) listReplies(ctx context.Context, teamID, channelID, messageID string) ([]teamsMessage, error) {
	var replies []teamsMessage
	apiURL := teamsGraphBase + "/teams/" + url.PathEscape(teamID) + "/channels/" + url.PathEscape(channelID) + "/messages/" + url.PathEscape(messageID) + "/replies"
	for apiURL != "" {
		var page teamsMessagesPage
		if err := c.getJSON(ctx, apiURL, &page); err != nil {
			return nil, err
		}
		replies = append(replies, page.Value...)
		apiURL = page.NextLink
	}
	return replies, nil
}

func (c *TeamsConnector) token(ctx context.Context) (string, error) {
	c.clientMu.Lock()
	if c.accessToken != "" && !c.cachedTokenExpiredLocked() {
		token := c.accessToken
		c.clientMu.Unlock()
		return token, nil
	}
	c.clientMu.Unlock()

	var cached teamsCachedToken
	var err error
	if c.acquireAccessToken != nil {
		token, tokenErr := c.acquireAccessToken(ctx)
		if tokenErr != nil {
			err = tokenErr
		} else {
			cached = teamsCachedToken{
				accessToken: token,
				expiresAt:   c.currentTime().Add(time.Hour),
			}
		}
	} else {
		cached, err = c.requestAccessToken(ctx)
	}
	if err != nil {
		return "", err
	}
	if cached.accessToken == "" {
		return "", fmt.Errorf("Teams token endpoint returned an empty access token")
	}

	c.clientMu.Lock()
	c.accessToken = cached.accessToken
	c.tokenExpiry = cached.expiresAt
	c.clientMu.Unlock()
	return cached.accessToken, nil
}

func (c *TeamsConnector) cachedTokenExpiredLocked() bool {
	return c.tokenExpiry.IsZero() || !c.currentTime().Add(teamsTokenExpiryMargin).Before(c.tokenExpiry)
}

func (c *TeamsConnector) invalidateToken(token string) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.accessToken == token {
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
	}
}

func (c *TeamsConnector) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *TeamsConnector) requestAccessToken(ctx context.Context) (teamsCachedToken, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
		"scope":         {teamsGraphScope},
	}
	requestCtx, cancel := context.WithTimeout(ctx, teamsRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, fmt.Sprintf(teamsTokenURLFormat, url.PathEscape(c.tenantID)), strings.NewReader(form.Encode()))
	if err != nil {
		return teamsCachedToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return teamsCachedToken{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return teamsCachedToken{}, &teamsHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	var token teamsTokenResponse
	if err = json.Unmarshal(body, &token); err != nil {
		return teamsCachedToken{}, err
	}
	if token.ExpiresIn <= 0 {
		return teamsCachedToken{}, fmt.Errorf("Teams token endpoint returned invalid expires_in")
	}
	return teamsCachedToken{
		accessToken: token.AccessToken,
		expiresAt:   c.currentTime().Add(time.Duration(token.ExpiresIn) * time.Second),
	}, nil
}

// getJSON GETs a Microsoft Graph endpoint and decodes JSON into out.
func (c *TeamsConnector) getJSON(ctx context.Context, apiURL string, out any) error {
	if c.doJSON != nil {
		return c.doJSON(ctx, apiURL, out)
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	hostname, resolvedIP, err := utility.AssertURLSafe(apiURL)
	if err != nil {
		return err
	}

	var lastErr error
	retriedUnauthorized := false
	for attempt := 1; attempt <= teamsRetryCount; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, teamsRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, apiURL, nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")

		client := utility.PinnedHTTPClient(hostname, resolvedIP, teamsRequestTimeout)
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
			resp.Body.Close()
			cancel()
			if resp.StatusCode < 400 {
				if readErr != nil {
					return readErr
				}
				return json.Unmarshal(body, out)
			}
			lastErr = &teamsHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
			if resp.StatusCode == http.StatusUnauthorized && !retriedUnauthorized {
				c.invalidateToken(token)
				token, err = c.token(ctx)
				if err != nil {
					return err
				}
				retriedUnauthorized = true
				attempt--
				continue
			}
			if !isTeamsRetryable(resp.StatusCode) {
				return lastErr
			}
		}
		if attempt == teamsRetryCount {
			break
		}
		delay := time.Duration(attempt) * teamsRetryBaseDelay
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func isTeamsRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

// parseTeamsTime parses a Graph ISO-8601 timestamp.
func parseTeamsTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.Replace(value, "Z", "+00:00", 1)); err == nil {
		return parsed
	}
	return time.Time{}
}

// teamsSyncCursor is the positional resume cursor.
type teamsSyncCursor struct {
	TeamIndex     int    `json:"team_index"`
	ChannelIndex  int    `json:"channel_index"`
	MessagesPage  string `json:"messages_page,omitempty"`
	MessageOffset int    `json:"message_offset,omitempty"`
	SourceID      string `json:"source_id,omitempty"`
}

// teamsSyncSession streams Teams documents for one fixed sync window.
type teamsSyncSession struct {
	connector   *TeamsConnector
	teams       []teamsTeam
	batchSize   int
	windowStart *time.Time
	windowEnd   time.Time

	teamIndex    int
	channels     []teamsChannel
	channelIndex int
	pageURL      string
	buffer       []teamsBufferedDocument

	resumeChannelIndex int
	resumePageURL      string
	resumeOffset       int
	resumeSourceID     string
}

type teamsBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
}

// applyResume advances the session to the last committed Teams position.
func (s *teamsSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("teams sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor teamsSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("teams sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" {
		return fmt.Errorf("teams sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.TeamIndex < 0 || cursor.TeamIndex >= len(s.teams) {
		return fmt.Errorf("teams resume team %d was not found in the current listing: %w", cursor.TeamIndex, ErrSyncResumeInvalid)
	}
	s.teamIndex = cursor.TeamIndex
	s.resumeChannelIndex = cursor.ChannelIndex
	s.resumePageURL = cursor.MessagesPage
	s.resumeOffset = cursor.MessageOffset
	s.resumeSourceID = sourceID
	s.pageURL = cursor.MessagesPage
	return nil
}

// NextBatch returns the next Teams document batch.
func (s *teamsSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	var checkpoint *SyncCheckpoint
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		for _, buffered := range s.buffer[:n] {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.teamIndex >= len(s.teams) && len(s.buffer) == 0 {
			if len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			break
		}
		page, err := s.nextDocumentPage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		if len(page) == 0 {
			continue
		}
		remaining := s.batchSize - len(documents)
		if len(page) > remaining {
			for _, buffered := range page[:remaining] {
				documents = append(documents, buffered.document)
				checkpoint = buffered.checkpoint
			}
			s.buffer = append(s.buffer, page[remaining:]...)
			break
		}
		for _, buffered := range page {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close closes the Teams sync session.
func (s *teamsSyncSession) Close() error {
	return nil
}

// nextDocumentPage streams one page of channel messages for the current
// team/channel, advancing channels/teams when a collection drains.
func (s *teamsSyncSession) nextDocumentPage(ctx context.Context) ([]teamsBufferedDocument, error) {
	for s.teamIndex < len(s.teams) {
		if len(s.channels) == 0 {
			channels, err := s.connector.listChannels(ctx, s.teams[s.teamIndex].ID)
			if err != nil {
				return nil, err
			}
			s.channels = channels
			s.channelIndex = 0
			if s.resumeChannelIndex > 0 {
				s.channelIndex = s.resumeChannelIndex
				s.resumeChannelIndex = 0
			}
		}
		if s.resumeSourceID != "" && s.channelIndex >= len(s.channels) {
			return nil, fmt.Errorf("teams resume channel %d was not found in team %s: %w", s.channelIndex, s.teams[s.teamIndex].ID, ErrSyncResumeInvalid)
		}
		if s.channelIndex >= len(s.channels) {
			s.advanceTeam()
			continue
		}
		team := s.teams[s.teamIndex]
		channel := s.channels[s.channelIndex]
		requestURL := s.pageURL
		if requestURL == "" {
			requestURL = teamsMessagesURL(team.ID, channel.ID)
		}
		page, err := s.connector.messagesPage(ctx, requestURL)
		if err != nil {
			return nil, err
		}

		candidates := make([]teamsBufferedDocument, 0, len(page.Value))
		pageOffset := 0
		for _, message := range page.Value {
			if message.ID == "" {
				continue
			}
			doc, ok, err := s.documentForMessage(ctx, team, channel, message)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			pageOffset++
			candidates = append(candidates, teamsBufferedDocument{
				document: doc,
				checkpoint: s.checkpoint(teamsSyncCursor{
					TeamIndex:     s.teamIndex,
					ChannelIndex:  s.channelIndex,
					MessagesPage:  requestURL,
					MessageOffset: pageOffset,
					SourceID:      doc.SourceID,
				}, doc),
				offset: pageOffset,
			})
		}

		documents, err := s.filterResumedDocuments(requestURL, candidates)
		if err != nil {
			return nil, err
		}
		if page.NextLink != "" {
			s.pageURL = page.NextLink
			return documents, nil
		}
		s.channelIndex++
		s.pageURL = ""
		return documents, nil
	}
	return nil, nil
}

// advanceTeam moves to the next team and clears per-team channel state.
func (s *teamsSyncSession) advanceTeam() {
	s.teamIndex++
	s.channels = nil
	s.channelIndex = 0
	s.pageURL = ""
	s.clearResumeOffset()
}

// documentForMessage flattens a post and its replies into one SourceDocument.
// Reply fetch failures propagate so the run aborts and resumes from the last
// committed checkpoint instead of silently dropping a channel message.
func (s *teamsSyncSession) documentForMessage(ctx context.Context, team teamsTeam, channel teamsChannel, message teamsMessage) (SourceDocument, bool, error) {
	replies, err := s.connector.listReplies(ctx, team.ID, channel.ID, message.ID)
	if err != nil {
		return SourceDocument{}, false, err
	}
	doc := message.toSourceDocument(team, channel, replies)
	if !s.inWindow(doc.UpdatedAt) {
		return SourceDocument{}, false, nil
	}
	return doc, true, nil
}

func (s *teamsSyncSession) inWindow(updatedAt time.Time) bool {
	if afterWindowEnd(updatedAt, s.windowEnd) {
		return false
	}
	if beforeOrAtWindowStart(updatedAt, s.windowStart) {
		return false
	}
	return true
}

// checkpoint serializes the current positional cursor.
func (s *teamsSyncSession) checkpoint(cursor teamsSyncCursor, doc SourceDocument) *SyncCheckpoint {
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    string(data),
		UpdatedAt: &updatedAt,
		SourceID:  doc.SourceID,
	}
}

// filterResumedDocuments drops documents through the committed checkpoint.
func (s *teamsSyncSession) filterResumedDocuments(pageURL string, candidates []teamsBufferedDocument) ([]teamsBufferedDocument, error) {
	if s.resumeSourceID == "" {
		return candidates, nil
	}
	if pageURL != s.resumePageURL {
		return nil, fmt.Errorf("teams resume page no longer matches checkpoint page: %w", ErrSyncResumeInvalid)
	}
	for index, candidate := range candidates {
		if candidate.document.SourceID == s.resumeSourceID {
			s.clearResumeOffset()
			return candidates[index+1:], nil
		}
	}
	return nil, fmt.Errorf("teams resume anchor %q was not found on %s: %w", s.resumeSourceID, pageURL, ErrSyncResumeInvalid)
}

func (s *teamsSyncSession) clearResumeOffset() {
	s.resumePageURL = ""
	s.resumeOffset = 0
	s.resumeSourceID = ""
}

// teamsPruneSession streams a complete Teams slim snapshot.
type teamsPruneSession struct {
	connector    *TeamsConnector
	teams        []teamsTeam
	batchSize    int
	teamIndex    int
	channels     []teamsChannel
	channelIndex int
	pageURL      string
	buffer       []SlimDocument
}

// NextBatch returns the next Teams prune snapshot batch.
func (s *teamsPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.teamIndex >= len(s.teams) {
			if len(documents) == 0 {
				return PruneBatch{}, io.EOF
			}
			break
		}
		page, err := s.nextSlimPage(ctx)
		if err != nil {
			return PruneBatch{}, err
		}
		if len(page) == 0 {
			continue
		}
		remaining := s.batchSize - len(documents)
		if len(page) > remaining {
			documents = append(documents, page[:remaining]...)
			s.buffer = append(s.buffer, page[remaining:]...)
			break
		}
		documents = append(documents, page...)
	}
	return PruneBatch{Documents: documents}, nil
}

// Close closes the Teams prune session.
func (s *teamsPruneSession) Close() error {
	return nil
}

// nextSlimPage streams one page of slim IDs for the current channel.
func (s *teamsPruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	for s.teamIndex < len(s.teams) {
		if len(s.channels) == 0 {
			channels, err := s.connector.listChannels(ctx, s.teams[s.teamIndex].ID)
			if err != nil {
				return nil, err
			}
			s.channels = channels
			s.channelIndex = 0
		}
		if s.channelIndex >= len(s.channels) {
			s.teamIndex++
			s.channels = nil
			s.channelIndex = 0
			s.pageURL = ""
			continue
		}
		team := s.teams[s.teamIndex]
		channel := s.channels[s.channelIndex]
		requestURL := s.pageURL
		if requestURL == "" {
			requestURL = teamsMessagesURL(team.ID, channel.ID)
		}
		page, err := s.connector.messagesPage(ctx, requestURL)
		if err != nil {
			return nil, err
		}
		documents := make([]SlimDocument, 0, len(page.Value))
		for _, message := range page.Value {
			if message.ID == "" {
				continue
			}
			documents = append(documents, SlimDocument{SourceID: teamsMessageID(team.ID, channel.ID, message.ID)})
		}
		if page.NextLink != "" {
			s.pageURL = page.NextLink
		} else {
			s.channelIndex++
			s.pageURL = ""
		}
		if len(documents) > 0 {
			return documents, nil
		}
	}
	return nil, nil
}

// teamsMessagesURL builds the channel messages list URL.
func teamsMessagesURL(teamID, channelID string) string {
	return teamsGraphBase + "/teams/" + url.PathEscape(teamID) + "/channels/" + url.PathEscape(channelID) + "/messages"
}

// teamsMessageID builds the stable source ID for a channel message.
func teamsMessageID(teamID, channelID, messageID string) string {
	return fmt.Sprintf("%s__%s__%s", teamID, channelID, messageID)
}

// toSourceDocument flattens a post and its replies into one SourceDocument.
func (m teamsMessage) toSourceDocument(team teamsTeam, channel teamsChannel, replies []teamsMessage) SourceDocument {
	thread := make([]teamsMessage, 0, 1+len(replies))
	thread = append(thread, m)
	thread = append(thread, replies...)

	contents := []string{}
	contentType := "text"
	var latest time.Time
	for _, item := range thread {
		text, ctype := item.bodyContent()
		if text != "" {
			contents = append(contents, text)
		}
		if ctype == "html" {
			contentType = "html"
		}
		modified := parseTeamsTime(item.LastModifiedDateTime)
		if modified.IsZero() {
			modified = parseTeamsTime(item.CreatedDateTime)
		}
		if !modified.IsZero() && (latest.IsZero() || modified.After(latest)) {
			latest = modified
		}
	}

	joined := strings.Join(contents, "\n\n")
	blob := []byte(joined)

	snippet := strings.TrimSpace(strings.ReplaceAll(joined, "\n", " "))
	if len(snippet) > 50 {
		snippet = strings.TrimRight(snippet[:50], " ") + "..."
	}
	semanticIdentifier := fmt.Sprintf("%s: %s", channel.DisplayName, snippet)
	if snippet == "" {
		semanticIdentifier = channel.DisplayName + " message"
	}

	metadata := map[string]any{
		"team":    team.DisplayName,
		"channel": channel.DisplayName,
	}
	if m.WebURL != "" {
		metadata["web_url"] = m.WebURL
	}

	extension := ".txt"
	if contentType == "html" {
		extension = ".html"
	}
	updatedAt := latest
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return SourceDocument{
		SourceID:           teamsMessageID(team.ID, channel.ID, m.ID),
		SemanticIdentifier: semanticIdentifier,
		Extension:          extension,
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        contentFingerprint(blob),
	}
}

// bodyContent returns the message body content and lowercased content type.
func (m teamsMessage) bodyContent() (string, string) {
	content := strings.TrimSpace(m.Body.Content)
	contentType := strings.ToLower(strings.TrimSpace(m.Body.ContentType))
	if contentType == "" {
		contentType = "text"
	}
	return content, contentType
}

// teamsCachedToken is a cached Graph access token.
type teamsCachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// teamsTokenResponse is the Azure AD client-credentials token response.
type teamsTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// teamsPage is a paged team collection.
type teamsPage struct {
	NextLink string      `json:"@odata.nextLink"`
	Value    []teamsTeam `json:"value"`
}

// teamsChannelsPage is a paged channel collection.
type teamsChannelsPage struct {
	NextLink string         `json:"@odata.nextLink"`
	Value    []teamsChannel `json:"value"`
}

// teamsMessagesPage is a paged message collection.
type teamsMessagesPage struct {
	NextLink string         `json:"@odata.nextLink"`
	Value    []teamsMessage `json:"value"`
}

// teamsTeam is a Microsoft Teams team.
type teamsTeam struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// teamsChannel is a Teams channel.
type teamsChannel struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// teamsMessage is a Teams channel message or reply.
type teamsMessage struct {
	ID                   string           `json:"id"`
	Body                 teamsMessageBody `json:"body"`
	LastModifiedDateTime string           `json:"lastModifiedDateTime"`
	CreatedDateTime      string           `json:"createdDateTime"`
	WebURL               string           `json:"webUrl"`
}

// teamsMessageBody carries message content.
type teamsMessageBody struct {
	Content     string `json:"content"`
	ContentType string `json:"contentType"`
}
