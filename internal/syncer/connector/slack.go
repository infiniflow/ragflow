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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Slack connector constants.
const (
	slackDefaultBaseURL   = "https://slack.com/api"
	slackDefaultBatchSize = 2
	slackRequestTimeout   = 60 * time.Second
	slackPageSize         = 900
	slackMaxRetryDelay    = 60 * time.Second
	slackSnippetLength    = 50
	slackMaxChannelsToLog = 20
	slackUSLACKBOTUser    = "USLACKBOT"
)

// Retry/backoff knobs for Slack API calls. They are package variables so tests
// can shrink the delays.
var (
	slackRetryTries     = 3
	slackRetryBaseDelay = time.Second
	slackRetryBackoff   = 2
)

// slackMaxResponseSize caps Slack API JSON responses. Package variable so
// tests can shrink it.
var slackMaxResponseSize int64 = 32 * 1024 * 1024

// SlackConnector reads public and private channel messages and threads from a
// Slack workspace through the Slack Web API with a bot token.
type SlackConnector struct {
	token               string
	channels            []string
	channelRegexEnabled bool
	batchSize           int
	baseURL             string
	client              *http.Client
	userCache           map[string]string
}

// NewSlackConnector creates a Slack connector from connector config.
func NewSlackConnector(config map[string]any) (*SlackConnector, error) {
	credentials := configAnyMap(config["credentials"])
	token := strings.TrimSpace(stringConfig(credentials["slack_bot_token"]))

	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), slackDefaultBatchSize)
	if batchSize <= 0 {
		batchSize = slackDefaultBatchSize
	}

	return &SlackConnector{
		token:               token,
		channels:            normalizeSlackChannelNames(coerceDiscordStringList(config["channels"])),
		channelRegexEnabled: configBoolDefault(config["channel_regex_enabled"], false),
		batchSize:           batchSize,
		baseURL:             strings.TrimRight(slackDefaultBaseURL, "/"),
		client:              &http.Client{Timeout: slackRequestTimeout},
		userCache:           map[string]string{},
	}, nil
}

// Validate validates Slack connector settings, credentials, and connectivity.
func (c *SlackConnector) Validate(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "Slack connector is nil"}
	}
	if c.token == "" {
		return &ConnectorMissingCredentialError{Message: "Slack connector requires 'slack_bot_token' in credentials"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "Slack connector batch_size must be a positive integer"}
	}

	// 1) Validate workspace connection.
	var auth slackAuthTestResponse
	if err := c.callAPI(ctx, "auth.test", url.Values{}, &auth); err != nil {
		return slackValidationError(err)
	}

	// 2) Confirm listing channels works.
	var list slackListChannelsResponse
	if err := c.callAPI(ctx, "conversations.list", url.Values{
		"limit": {"1"},
		"types": {"public_channel"},
	}, &list); err != nil {
		return slackValidationError(err)
	}

	// 3) Confirm users:read scope is available (required to resolve senders).
	var user slackUserInfoResponse
	if err := c.callAPI(ctx, "users.info", url.Values{"user": {slackUSLACKBOTUser}}, &user); err != nil {
		if errors.As(err, new(*slackUserNotFoundError)) {
			return nil
		}
		return slackValidationError(err)
	}
	return nil
}

// ValidateConnectorSetting validates Slack settings from an unsaved config.
func (c *SlackConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Slack sync session.
func (c *SlackConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	channels, err := c.resolvedChannels(ctx)
	if err != nil {
		return nil, err
	}
	session := &slackSyncSession{
		connector:   c,
		request:     request,
		channels:    channels,
		batchSize:   c.batchSize,
		seenThreads: map[string]struct{}{},
	}
	if request.Resume != nil {
		if err := session.applyResume(request.Resume); err != nil {
			return nil, err
		}
	}
	return session, nil
}

// OpenPrune opens one complete Slack prune snapshot session.
func (c *SlackConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	channels, err := c.resolvedChannels(ctx)
	if err != nil {
		return nil, err
	}
	return &slackPruneSession{connector: c, channels: channels, batchSize: c.batchSize}, nil
}

// ---------------------------------------------------------------------------
// Slack Web API client
// ---------------------------------------------------------------------------

// slackAPIResponse is embedded in every Slack API response payload.
type slackAPIResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// slackResponseMetadata carries the opaque pagination cursor.
type slackResponseMetadata struct {
	NextCursor string `json:"next_cursor"`
}

type slackAuthTestResponse struct {
	slackAPIResponse
	URL    string `json:"url"`
	Team   string `json:"team"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
}

type slackChannel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsArchived bool   `json:"is_archived"`
	IsMember   bool   `json:"is_member"`
	IsPrivate  bool   `json:"is_private"`
}

type slackListChannelsResponse struct {
	slackAPIResponse
	Channels         []slackChannel        `json:"channels"`
	ResponseMetadata slackResponseMetadata `json:"response_metadata"`
}

type slackMessage struct {
	TS         string `json:"ts"`
	ThreadTS   string `json:"thread_ts"`
	User       string `json:"user"`
	Text       string `json:"text"`
	Subtype    string `json:"subtype"`
	BotID      string `json:"bot_id"`
	AppID      string `json:"app_id"`
	BotProfile struct {
		Name string `json:"name"`
	} `json:"bot_profile"`
}

type slackHistoryResponse struct {
	slackAPIResponse
	Messages         []slackMessage        `json:"messages"`
	HasMore          bool                  `json:"has_more"`
	ResponseMetadata slackResponseMetadata `json:"response_metadata"`
}

type slackUserInfoResponse struct {
	slackAPIResponse
	User struct {
		ID       string `json:"id"`
		RealName string `json:"real_name"`
		Profile  struct {
			DisplayName string `json:"display_name"`
			RealName    string `json:"real_name"`
		} `json:"profile"`
	} `json:"user"`
}

// slackTransientError marks a retryable Slack API failure.
type slackTransientError struct {
	retryAfter  time.Duration
	rateLimited bool
	message     string
}

func (e *slackTransientError) Error() string { return e.message }

// slackUserNotFoundError is returned when users.info cannot resolve a user.
type slackUserNotFoundError struct{ message string }

func (e *slackUserNotFoundError) Error() string { return e.message }

// slackAPIError wraps a classified Slack API error with the raw error code so
// callers can branch on the specific Slack failure.
type slackAPIError struct {
	code string
	err  error
}

func (e *slackAPIError) Error() string { return e.err.Error() }
func (e *slackAPIError) Unwrap() error { return e.err }

// callAPI sends one Slack Web API call, retrying transient failures.
func (c *SlackConnector) callAPI(ctx context.Context, method string, params url.Values, out any) error {
	return c.retrySlack(ctx, func() error {
		return c.callAPINoRetry(ctx, method, params, out)
	})
}

func (c *SlackConnector) callAPINoRetry(ctx context.Context, method string, params url.Values, out any) error {
	endpoint := c.baseURL + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, slackMaxResponseSize+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > slackMaxResponseSize {
		return &ConnectorValidationError{Message: fmt.Sprintf("Slack API response from %s exceeds maximum size of %d bytes", method, slackMaxResponseSize)}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return &slackTransientError{
			retryAfter:  slackRetryAfterSeconds(resp.Header.Get("Retry-After"), body),
			rateLimited: true,
			message:     "Slack API rate limited",
		}
	}

	var probe slackAPIResponse
	if err := json.Unmarshal(body, &probe); err != nil {
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return &ConnectorValidationError{Message: fmt.Sprintf("Slack API %s request failed with status %d", method, resp.StatusCode)}
		}
		return fmt.Errorf("Slack API %s returned invalid JSON: %w", method, err)
	}
	if !probe.OK {
		return classifySlackAPIError(method, probe.Error, body)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &ConnectorValidationError{Message: fmt.Sprintf("Slack API %s request failed with status %d", method, resp.StatusCode)}
	}
	return json.Unmarshal(body, out)
}

func classifySlackAPIError(method, apiError string, body []byte) error {
	var err error
	switch apiError {
	case "ratelimited":
		err = &slackTransientError{
			retryAfter:  slackRetryAfterSeconds("", body),
			rateLimited: true,
			message:     "Slack API rate limited",
		}
	case "invalid_auth":
		err = &ConnectorValidationError{Message: "Invalid Slack bot token (invalid_auth)."}
	case "not_authed":
		err = &ConnectorMissingCredentialError{Message: "Invalid or expired Slack bot token (not_authed)."}
	case "missing_scope":
		err = &ConnectorValidationError{Message: slackMissingScopeMessage(method, body)}
	case "not_in_channel":
		err = &ConnectorValidationError{Message: "Slack bot is not a member of the channel (not_in_channel)."}
	case "channel_not_found":
		err = &ConnectorValidationError{Message: "Slack channel not found (channel_not_found)."}
	case "user_not_found":
		err = &slackUserNotFoundError{message: "Slack user not found (user_not_found)."}
	case "method_not_supported_for_channel_type":
		err = &ConnectorValidationError{Message: "Slack method is not supported for this channel type."}
	default:
		if apiError == "" {
			err = &ConnectorValidationError{Message: fmt.Sprintf("Slack API %s request failed without an error code", method)}
		} else {
			err = &ConnectorValidationError{Message: fmt.Sprintf("Slack API %s request failed: %s", method, apiError)}
		}
	}
	if apiError == "" {
		return err
	}
	return &slackAPIError{code: apiError, err: err}
}

// slackMissingScopeMessage builds an actionable message for a missing_scope
// Slack error, naming the scope the call needed. It falls back to a generic
// message when Slack does not include the needed scope in the response.
func slackMissingScopeMessage(method string, body []byte) string {
	var payload struct {
		Needed any `json:"needed"`
	}
	msg := fmt.Sprintf("Slack bot token lacks the necessary OAuth scope to access %s (missing_scope)", method)
	if err := json.Unmarshal(body, &payload); err != nil {
		return msg + "."
	}
	if needed := slackScopeNames(payload.Needed); needed != "" {
		return fmt.Sprintf("%s. Needed: %s", msg, needed)
	}
	return msg + "."
}

// slackScopeNames flattens a Slack scope field, which is either a single
// comma-separated string or a JSON array of scope names.
func slackScopeNames(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func slackRetryAfterSeconds(header string, body []byte) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	var payload struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.RetryAfter > 0 {
		return time.Duration(payload.RetryAfter * float64(time.Second))
	}
	return 0
}

func (c *SlackConnector) retrySlack(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < slackRetryTries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isSlackTransient(err) || attempt+1 >= slackRetryTries {
			break
		}
		delay := slackRetryBaseDelay
		for i := 0; i < attempt; i++ {
			delay *= time.Duration(slackRetryBackoff)
		}
		var transient *slackTransientError
		if errors.As(err, &transient) && transient.retryAfter > delay {
			delay = transient.retryAfter
		}
		if delay > slackMaxRetryDelay {
			delay = slackMaxRetryDelay
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

func isSlackTransient(err error) bool {
	if err == nil {
		return false
	}
	var transient *slackTransientError
	if errors.As(err, &transient) {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// slackValidationError treats a persistent rate limit as a warning so that
// settings validation proceeds, matching the reference SDK behavior.
func slackValidationError(err error) error {
	var transient *slackTransientError
	if errors.As(err, &transient) && transient.rateLimited {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Channel and message enumeration
// ---------------------------------------------------------------------------

// resolvedChannels returns the configured channels, sorted by ID so checkpoints
// advance deterministically.
func (c *SlackConnector) resolvedChannels(ctx context.Context) ([]slackChannel, error) {
	all, err := c.listChannels(ctx)
	if err != nil {
		return nil, err
	}
	filtered, err := filterSlackChannels(all, c.channels, c.channelRegexEnabled)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].ID < filtered[j].ID
	})
	return filtered, nil
}

// listChannels lists public and private channels, falling back to public-only
// when private channel enumeration fails.
func (c *SlackConnector) listChannels(ctx context.Context) ([]slackChannel, error) {
	channels, err := c.collectChannels(ctx, []string{"public_channel", "private_channel"})
	if err != nil {
		channels, err = c.collectChannels(ctx, []string{"public_channel"})
	}
	return channels, err
}

func (c *SlackConnector) collectChannels(ctx context.Context, channelTypes []string) ([]slackChannel, error) {
	var channels []slackChannel
	cursor := ""
	for {
		params := url.Values{
			"limit":            {strconv.Itoa(slackPageSize)},
			"types":            {strings.Join(channelTypes, ",")},
			"exclude_archived": {"true"},
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		var resp slackListChannelsResponse
		if err := c.callAPI(ctx, "conversations.list", params, &resp); err != nil {
			return nil, err
		}
		channels = append(channels, resp.Channels...)
		if resp.ResponseMetadata.NextCursor == "" {
			return channels, nil
		}
		cursor = resp.ResponseMetadata.NextCursor
	}
}

// channelMessages returns every top-level message in a channel, optionally
// bounded by oldest/latest Slack epoch-second strings (empty means unbounded).
func (c *SlackConnector) channelMessages(ctx context.Context, channel slackChannel, oldest, latest string) ([]slackMessage, error) {
	if err := c.joinChannel(ctx, channel); err != nil {
		return nil, err
	}
	var messages []slackMessage
	cursor := ""
	for {
		params := url.Values{
			"channel": {channel.ID},
			"limit":   {strconv.Itoa(slackPageSize)},
		}
		if oldest != "" {
			params.Set("oldest", oldest)
		}
		if latest != "" {
			params.Set("latest", latest)
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		var resp slackHistoryResponse
		if err := c.callAPI(ctx, "conversations.history", params, &resp); err != nil {
			return nil, err
		}
		messages = append(messages, resp.Messages...)
		if resp.ResponseMetadata.NextCursor == "" {
			return messages, nil
		}
		cursor = resp.ResponseMetadata.NextCursor
	}
}

// getThread returns the full reply thread for a channel message timestamp.
func (c *SlackConnector) getThread(ctx context.Context, channelID, threadTS string) ([]slackMessage, error) {
	var messages []slackMessage
	cursor := ""
	for {
		params := url.Values{
			"channel": {channelID},
			"ts":      {threadTS},
			"limit":   {strconv.Itoa(slackPageSize)},
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		var resp slackHistoryResponse
		if err := c.callAPI(ctx, "conversations.replies", params, &resp); err != nil {
			return nil, err
		}
		messages = append(messages, resp.Messages...)
		if resp.ResponseMetadata.NextCursor == "" {
			return messages, nil
		}
		cursor = resp.ResponseMetadata.NextCursor
	}
}

// errSlackChannelUnavailable marks Slack channels that the bot cannot join so
// callers can skip them instead of failing the whole sync/prune run.
var errSlackChannelUnavailable = errors.New("slack channel unavailable")

// slackUnjoinableErrors are conversations.join failure codes that mean the bot
// cannot join the channel.
var slackUnjoinableErrors = map[string]struct{}{
	"is_archived":                           {},
	"method_not_supported_for_channel_type": {},
	"channel_not_found":                     {},
	"restricted_action":                     {},
}

// joinChannel joins a channel so the bot can read its messages. Channels the
// bot cannot join are wrapped in errSlackChannelUnavailable so callers can
// skip them; unrelated API errors propagate unchanged. Automatic joins are
// limited to the configured channels selection when one is present.
func (c *SlackConnector) joinChannel(ctx context.Context, channel slackChannel) error {
	if channel.IsMember {
		return nil
	}
	if len(c.channels) > 0 && !slackChannelSelected(channel, c.channels, c.channelRegexEnabled) {
		return nil
	}
	var resp struct {
		slackAPIResponse
	}
	if err := c.callAPI(ctx, "conversations.join", url.Values{"channel": {channel.ID}}, &resp); err != nil {
		var apiErr *slackAPIError
		if errors.As(err, &apiErr) {
			if _, ok := slackUnjoinableErrors[apiErr.code]; ok {
				return fmt.Errorf("%w: %w", errSlackChannelUnavailable, err)
			}
		}
		return err
	}
	return nil
}

// slackChannelSelected reports whether a channel is part of the configured
// channels selection (exact names, or regex patterns when enabled). An empty
// selection matches every channel.
func slackChannelSelected(channel slackChannel, configured []string, regexEnabled bool) bool {
	if len(configured) == 0 {
		return true
	}
	for _, pattern := range configured {
		if regexEnabled {
			re, err := regexp.Compile("^" + pattern + "$")
			if err != nil {
				continue
			}
			if re.MatchString(channel.Name) {
				return true
			}
			continue
		}
		if channel.Name == pattern {
			return true
		}
	}
	return false
}

// displayName resolves a Slack user ID to a display name, caching results.
func (c *SlackConnector) displayName(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", nil
	}
	if name, ok := c.userCache[userID]; ok {
		return name, nil
	}
	var resp slackUserInfoResponse
	if err := c.callAPI(ctx, "users.info", url.Values{"user": {userID}}, &resp); err != nil {
		var notFound *slackUserNotFoundError
		if errors.As(err, &notFound) {
			return "", nil
		}
		return "", err
	}
	name := resp.User.Profile.DisplayName
	if name == "" {
		name = resp.User.RealName
	}
	if name == "" {
		name = resp.User.Profile.RealName
	}
	if name == "" {
		return "", nil
	}
	c.userCache[userID] = name
	return name, nil
}

// filterSlackChannels narrows the workspace channels to the configured set.
func filterSlackChannels(all []slackChannel, channels []string, regexEnabled bool) ([]slackChannel, error) {
	if len(channels) == 0 {
		return all, nil
	}
	if regexEnabled {
		var out []slackChannel
		for _, pattern := range channels {
			re, err := regexp.Compile("^" + pattern + "$")
			if err != nil {
				return nil, &ConnectorValidationError{Message: fmt.Sprintf("Invalid Slack channel regex %q", pattern)}
			}
			for _, channel := range all {
				if re.MatchString(channel.Name) {
					out = append(out, channel)
				}
			}
		}
		return out, nil
	}

	available := map[string]slackChannel{}
	for _, channel := range all {
		available[channel.Name] = channel
	}
	var missing []string
	for _, name := range channels {
		if _, ok := available[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		names := make([]string, 0, len(available))
		for name := range available {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) > slackMaxChannelsToLog {
			names = names[:slackMaxChannelsToLog]
		}
		return nil, &ConnectorValidationError{Message: fmt.Sprintf(
			"Channel(s) %s not found in workspace. Available channels: %s",
			strings.Join(missing, ", "), strings.Join(names, ", "))}
	}
	out := make([]slackChannel, 0, len(channels))
	for _, name := range channels {
		out = append(out, available[name])
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Message filtering and text cleaning
// ---------------------------------------------------------------------------

// slackDisallowedSubtypes lists message subtypes that carry no informative
// content.
var slackDisallowedSubtypes = map[string]struct{}{
	"channel_join":                {},
	"channel_leave":               {},
	"channel_archive":             {},
	"channel_unarchive":           {},
	"pinned_item":                 {},
	"unpinned_item":               {},
	"ekm_access_denied":           {},
	"channel_posting_permissions": {},
	"group_join":                  {},
	"group_leave":                 {},
	"group_archive":               {},
	"group_unarchive":             {},
	"channel_name":                {},
}

// acceptSlackMessage reports whether a message should be indexed.
func acceptSlackMessage(message slackMessage) bool {
	if message.BotID != "" || message.AppID != "" {
		if message.BotProfile.Name == "DanswerBot Testing" {
			return true
		}
		return false
	}
	if message.Subtype != "" {
		if _, ok := slackDisallowedSubtypes[message.Subtype]; ok {
			return false
		}
	}
	return true
}

var (
	slackUserMentionRe     = regexp.MustCompile(`<@(.*?)>`)
	slackChannelMentionRe  = regexp.MustCompile(`<#(.*?)\|(.*?)>`)
	slackSpecialCatchallRe = regexp.MustCompile(`<!([^|]+)\|([^>]+)>`)
)

// indexClean normalizes Slack markdown links and mentions into readable text.
func (c *SlackConnector) indexClean(ctx context.Context, text string) (string, error) {
	cleaned, err := c.replaceUserIDsWithNames(ctx, text)
	if err != nil {
		return "", err
	}
	cleaned = slackReplaceChannelsBasic(cleaned)
	cleaned = slackReplaceSpecialMentions(cleaned)
	cleaned = slackReplaceSpecialCatchall(cleaned)
	return cleaned, nil
}

func (c *SlackConnector) replaceUserIDsWithNames(ctx context.Context, text string) (string, error) {
	matches := slackUserMentionRe.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		userID := match[1]
		name, err := c.displayName(ctx, userID)
		if err != nil {
			return "", err
		}
		if name != "" {
			text = strings.ReplaceAll(text, match[0], "@"+name)
		}
	}
	return text, nil
}

func slackReplaceChannelsBasic(text string) string {
	return slackChannelMentionRe.ReplaceAllString(text, "#$2")
}

func slackReplaceSpecialMentions(text string) string {
	text = strings.ReplaceAll(text, "<!channel>", "@channel")
	text = strings.ReplaceAll(text, "<!here>", "@here")
	text = strings.ReplaceAll(text, "<!everyone>", "@everyone")
	return text
}

func slackReplaceSpecialCatchall(text string) string {
	return slackSpecialCatchallRe.ReplaceAllString(text, "$2")
}

// ---------------------------------------------------------------------------
// Document building
// ---------------------------------------------------------------------------

func slackDocID(channelID, ts string) string {
	return channelID + "__" + ts
}

// threadToDocument flattens a filtered thread into one text document. docTS is
// the thread root timestamp used as the document identity.
func (c *SlackConnector) threadToDocument(ctx context.Context, channel slackChannel, thread []slackMessage, docTS string) (*SourceDocument, error) {
	senderName := "Unknown"
	if len(thread) > 0 {
		if name, err := c.displayName(ctx, thread[0].User); err != nil {
			return nil, err
		} else if name != "" {
			senderName = name
		}
	}

	cleaned := make([]string, 0, len(thread))
	for _, message := range thread {
		text, err := c.indexClean(ctx, message.Text)
		if err != nil {
			return nil, err
		}
		cleaned = append(cleaned, text)
	}

	first := ""
	if len(cleaned) > 0 {
		first = cleaned[0]
	}
	snippet := first
	if runes := []rune(first); len(runes) > slackSnippetLength {
		snippet = strings.TrimRight(string(runes[:slackSnippetLength]), " \t\n") + "..."
	}

	semantic := fmt.Sprintf("%s in #%s: %s", senderName, channel.Name, snippet)
	semantic = strings.ReplaceAll(semantic, "\n", " ")

	content := strings.Join(cleaned, "\n\n")
	blob := []byte(content)

	var updatedAt time.Time
	for _, message := range thread {
		if ts := slackMessageTime(message.TS); ts.After(updatedAt) {
			updatedAt = ts
		}
	}

	doc := &SourceDocument{
		SourceID:           slackDocID(channel.ID, docTS),
		SemanticIdentifier: semantic,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           map[string]any{"Channel": channel.Name},
	}
	return doc, nil
}

func slackMessageTime(ts string) time.Time {
	seconds, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return time.Time{}
	}
	whole := int64(seconds)
	nanos := int64((seconds - float64(whole)) * 1e9)
	return time.Unix(whole, nanos).UTC()
}

func normalizeSlackChannelNames(channels []string) []string {
	out := make([]string, 0, len(channels))
	for _, name := range channels {
		name = strings.TrimSpace(strings.TrimPrefix(name, "#"))
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Sync session with checkpoint resume
// ---------------------------------------------------------------------------

type slackSyncSession struct {
	connector   *SlackConnector
	request     SyncRequest
	channels    []slackChannel
	batchSize   int
	seenThreads map[string]struct{}

	resumeAfterChannelID string
	channelIndex         int
	pending              []SourceDocument
	pendingCheckpoint    *SyncCheckpoint
	done                 bool
}

// NextBatch returns the next Slack document batch.
func (s *slackSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	for {
		if s.done {
			return SyncBatch{}, io.EOF
		}
		if len(s.pending) == 0 {
			if s.channelIndex >= len(s.channels) {
				s.done = true
				return SyncBatch{}, io.EOF
			}
			channel := s.channels[s.channelIndex]
			s.channelIndex++
			if channel.ID <= s.resumeAfterChannelID {
				continue
			}
			documents, err := s.channelDocuments(ctx, channel)
			if err != nil {
				return SyncBatch{}, err
			}
			if len(documents) == 0 {
				continue
			}
			s.pending = documents
			updatedAt := documents[len(documents)-1].UpdatedAt
			checkpoint := &SyncCheckpoint{
				Cursor:    fmt.Sprintf("slack_channel_%s", channel.ID),
				SourceID:  fmt.Sprintf("slack_channel_%s", channel.ID),
				UpdatedAt: &updatedAt,
			}
			// The checkpoint only advances once the whole channel is flushed so
			// an interrupted run re-processes a partially committed channel.
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

// Close closes the Slack sync session.
func (s *slackSyncSession) Close() error {
	return nil
}

func (s *slackSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	const prefix = "slack_channel_"
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("slack sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	channelID := strings.TrimPrefix(sourceID, prefix)
	if channelID == "" {
		return fmt.Errorf("slack sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for _, channel := range s.channels {
		if channel.ID == channelID {
			s.resumeAfterChannelID = channelID
			return nil
		}
	}
	return fmt.Errorf("slack resume channel %q was not found in the current listing: %w", channelID, ErrSyncResumeInvalid)
}

// channelDocuments builds the documents for one channel.
func (s *slackSyncSession) channelDocuments(ctx context.Context, channel slackChannel) ([]SourceDocument, error) {
	s.seenThreads = map[string]struct{}{}
	oldest, latest := "", ""
	if !s.request.FromBeginning {
		if s.request.WindowStart != nil {
			oldest = strconv.FormatInt(s.request.WindowStart.Unix(), 10)
		}
		if !s.request.WindowEnd.IsZero() {
			latest = strconv.FormatInt(s.request.WindowEnd.Unix(), 10)
		}
	}
	messages, err := s.connector.channelMessages(ctx, channel, oldest, latest)
	if err != nil {
		if errors.Is(err, errSlackChannelUnavailable) {
			return nil, nil
		}
		return nil, err
	}

	var documents []SourceDocument
	for _, message := range messages {
		doc, err := s.processMessage(ctx, channel, message)
		if err != nil {
			return nil, err
		}
		if doc != nil {
			documents = append(documents, *doc)
		}
	}
	return documents, nil
}

// processMessage converts one top-level message into a document, fetching and
// merging its reply thread when present.
func (s *slackSyncSession) processMessage(ctx context.Context, channel slackChannel, message slackMessage) (*SourceDocument, error) {
	threadOrMessageTS := message.ThreadTS
	if threadOrMessageTS == "" {
		threadOrMessageTS = message.TS
	}
	if _, seen := s.seenThreads[threadOrMessageTS]; seen {
		return nil, nil
	}
	s.seenThreads[threadOrMessageTS] = struct{}{}

	if message.ThreadTS != "" {
		thread, err := s.connector.getThread(ctx, channel.ID, message.ThreadTS)
		if err != nil {
			return nil, err
		}
		filtered := filterSlackMessages(thread)
		if len(filtered) == 0 {
			return nil, nil
		}
		return s.connector.threadToDocument(ctx, channel, filtered, threadOrMessageTS)
	}
	if !acceptSlackMessage(message) {
		return nil, nil
	}
	return s.connector.threadToDocument(ctx, channel, []slackMessage{message}, threadOrMessageTS)
}

func filterSlackMessages(messages []slackMessage) []slackMessage {
	filtered := make([]slackMessage, 0, len(messages))
	for _, message := range messages {
		if acceptSlackMessage(message) {
			filtered = append(filtered, message)
		}
	}
	return filtered
}

type slackPruneSession struct {
	connector *SlackConnector
	channels  []slackChannel
	batchSize int

	channelIndex int
	pending      []SlimDocument
	done         bool
}

// NextBatch returns the next Slack prune snapshot batch, paging channels
// lazily so the whole workspace is never collected in memory at once.
func (s *slackPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	for {
		if s.done {
			return PruneBatch{}, io.EOF
		}
		if len(s.pending) == 0 {
			if s.channelIndex >= len(s.channels) {
				s.done = true
				return PruneBatch{}, io.EOF
			}
			channel := s.channels[s.channelIndex]
			s.channelIndex++
			documents, err := s.connector.pruneChannelDocuments(ctx, channel)
			if err != nil {
				return PruneBatch{}, err
			}
			if len(documents) == 0 {
				continue
			}
			s.pending = documents
		}
		n := s.batchSize
		if n > len(s.pending) {
			n = len(s.pending)
		}
		chunk := s.pending[:n]
		s.pending = s.pending[n:]
		return PruneBatch{Documents: chunk}, nil
	}
}

// Close closes the Slack prune session.
func (s *slackPruneSession) Close() error {
	return nil
}

// pruneChannelDocuments returns the slim snapshot documents for one channel.
func (c *SlackConnector) pruneChannelDocuments(ctx context.Context, channel slackChannel) ([]SlimDocument, error) {
	messages, err := c.channelMessages(ctx, channel, "", "")
	if err != nil {
		if errors.Is(err, errSlackChannelUnavailable) {
			return nil, nil
		}
		return nil, err
	}
	return c.slackPruneDocuments(ctx, channel, messages)
}

// slackPruneDocuments maps channel history to slim snapshot documents. Each
// thread is resolved with the same full-thread view and acceptance logic as
// sync (conversations.replies), so prune identities match sync documents.
func (c *SlackConnector) slackPruneDocuments(ctx context.Context, channel slackChannel, messages []slackMessage) ([]SlimDocument, error) {
	seen := map[string]struct{}{}
	var documents []SlimDocument
	for _, message := range messages {
		rootTS := slackThreadRootTS(message)
		if _, ok := seen[rootTS]; ok {
			continue
		}
		seen[rootTS] = struct{}{}
		accepted := false
		if message.ThreadTS != "" {
			thread, err := c.getThread(ctx, channel.ID, message.ThreadTS)
			if err != nil {
				return nil, err
			}
			accepted = len(filterSlackMessages(thread)) > 0
		} else {
			accepted = acceptSlackMessage(message)
		}
		if accepted {
			documents = append(documents, SlimDocument{SourceID: slackDocID(channel.ID, rootTS)})
		}
	}
	return documents, nil
}

// slackThreadRootTS returns the root timestamp of a message's thread, falling
// back to the message's own timestamp for top-level messages.
func slackThreadRootTS(message slackMessage) string {
	if message.ThreadTS != "" {
		return message.ThreadTS
	}
	return message.TS
}
