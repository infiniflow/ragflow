package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Fake implementations
// ---------------------------------------------------------------------------

type fakeSessionStore struct {
	mu            sync.Mutex
	sessions      map[string]*entity.ChatSession
	dialogs       map[string]*entity.Chat
	dialogExists  map[string]bool // key: tenantID|chatID
	getByIDErr    error
	createErr     error
	updateByIDErr error
	getDialogErr  error
	// record calls
	createCalled []*entity.ChatSession
	updateCalled []struct {
		id      string
		updates map[string]interface{}
	}
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{
		sessions:     make(map[string]*entity.ChatSession),
		dialogs:      make(map[string]*entity.Chat),
		dialogExists: make(map[string]bool),
	}
}

func (f *fakeSessionStore) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.ChatSession, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	s, ok := f.sessions[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	clone := *s
	return &clone, nil
}

func (f *fakeSessionStore) GetBySessionIDAndChatID(ctx context.Context, db *gorm.DB, sessionID, chatID string) (*entity.ChatSession, error) {
	s, err := f.GetByID(ctx, db, sessionID)
	if err != nil {
		return nil, err
	}
	if s.DialogID != chatID {
		return nil, gorm.ErrRecordNotFound
	}
	return s, nil
}

func (f *fakeSessionStore) Create(ctx context.Context, db *gorm.DB, conv *entity.ChatSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	clone := *conv
	f.sessions[conv.ID] = &clone
	f.createCalled = append(f.createCalled, conv)
	return nil
}

func (f *fakeSessionStore) UpdateByID(ctx context.Context, db *gorm.DB, id string, updates map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateByIDErr != nil {
		return f.updateByIDErr
	}
	s, ok := f.sessions[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	if history, ok := updates["history_update"].(dao.ConversationHistoryUpdate); ok {
		messages := parseMessages(s.Message)
		references := parseReferenceList(s.Reference)
		if history.DeleteMessageID != "" {
			for i, message := range messages {
				if stringValue(message["id"]) != history.DeleteMessageID {
					continue
				}
				end := i + 1
				if end < len(messages) && stringValue(messages[end]["role"]) == "assistant" && stringValue(messages[end]["id"]) == history.DeleteMessageID {
					refIndex := sessionMessageReferenceIndex(messages, end)
					if refIndex >= 0 && refIndex < len(references) {
						references = append(references[:refIndex], references[refIndex+1:]...)
					}
					end++
				}
				messages = append(messages[:i], messages[end:]...)
				break
			}
		} else if history.FeedbackMessageID != "" {
			for _, message := range messages {
				if stringValue(message["id"]) == history.FeedbackMessageID && stringValue(message["role"]) == "assistant" {
					message["thumbup"] = history.Feedback["thumb_up"]
					if feedback, ok := history.Feedback["feedback"]; ok {
						if feedback == nil {
							delete(message, "feedback")
						} else {
							message["feedback"] = feedback
						}
					}
				}
			}
		} else if history.Message != nil {
			messages = append(messages, history.Message)
			if history.AppendReference {
				references = append(references, history.Reference)
			}
		}
		updates["message"], _ = json.Marshal(messages)
		updates["reference"], _ = json.Marshal(references)
		delete(updates, "history_update")
	}
	f.updateCalled = append(f.updateCalled, struct {
		id      string
		updates map[string]interface{}
	}{id, updates})
	for k, v := range updates {
		switch k {
		case "name":
			if str, ok := v.(string); ok {
				s.Name = &str
			}
		case "message":
			if raw, ok := v.([]byte); ok {
				s.Message = append(json.RawMessage(nil), raw...)
			}
		case "reference":
			if raw, ok := v.([]byte); ok {
				s.Reference = append(json.RawMessage(nil), raw...)
			}
		}
	}
	return nil
}

func (f *fakeSessionStore) DeleteByID(ctx context.Context, db *gorm.DB, id string) error {
	delete(f.sessions, id)
	return nil
}

func (f *fakeSessionStore) ListByChatID(ctx context.Context, db *gorm.DB, chatID, sessionID, name string, terms []dao.OrderTerm, page, pageSize int, includeHistory ...bool) ([]*entity.ChatSession, error) {
	var result []*entity.ChatSession
	for _, s := range f.sessions {
		if s.DialogID != chatID {
			continue
		}
		if sessionID != "" && s.ID != sessionID {
			continue
		}
		if name != "" {
			var sessionName string
			if s.Name != nil {
				sessionName = *s.Name
			}
			if sessionName != name {
				continue
			}
		}
		result = append(result, s)
	}
	return result, nil
}

func (f *fakeSessionStore) GetDialogByID(ctx context.Context, db *gorm.DB, chatID string) (*entity.Chat, error) {
	if f.getDialogErr != nil {
		return nil, f.getDialogErr
	}
	d, ok := f.dialogs[chatID]
	if !ok {
		return nil, errors.New("dialog not found")
	}
	return d, nil
}

func (f *fakeSessionStore) CheckDialogExists(ctx context.Context, db *gorm.DB, tenantID, chatID string) (bool, error) {
	key := tenantID + "|" + chatID
	return f.dialogExists[key], nil
}

// ---------------------------------------------------------------------------

type fakeTenantStore struct {
	tenantIDs []string
	err       error
}

func (f *fakeTenantStore) GetTenantIDsByUserID(ctx context.Context, db *gorm.DB, userID string) ([]string, error) {
	return f.tenantIDs, f.err
}

// ---------------------------------------------------------------------------

type fakePipeline struct {
	resultChan <-chan AsyncChatResult
	err        error
	userID     string
	messages   []map[string]interface{}
}

func (f *fakePipeline) AsyncChat(ctx context.Context, userID string, chat *entity.Chat, messages []map[string]interface{}, stream bool, kwargs map[string]interface{}) (<-chan AsyncChatResult, error) {
	f.userID = userID
	f.messages = messages
	return f.resultChan, f.err
}

func makeResultChan(results ...AsyncChatResult) <-chan AsyncChatResult {
	ch := make(chan AsyncChatResult, len(results))
	for _, r := range results {
		ch <- r
	}
	close(ch)
	return ch
}

type fakeChatModelConfigResolver struct {
	tenantID string
	llmID    string
	err      error
}

func (f *fakeChatModelConfigResolver) ResolveModelConfig(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*ModelTarget, error) {
	f.tenantID = tenantID
	f.llmID = modelRef
	if f.err != nil {
		return nil, f.err
	}
	return &ModelTarget{ModelName: "resolved-model", APIConfig: &modelModule.APIConfig{}, MaxTokens: 8192}, nil
}

func (f *fakeChatModelConfigResolver) ResolveDefaultModelConfig(ctx context.Context, tenantID string, modelType entity.ModelType) (*ModelTarget, error) {
	f.tenantID = tenantID
	if f.err != nil {
		return nil, f.err
	}
	return &ModelTarget{ModelName: "resolved-model", APIConfig: &modelModule.APIConfig{}, MaxTokens: 8192}, nil
}

func (f *fakeChatModelConfigResolver) ResolveModelType(ctx context.Context, tenantID, modelRef string) ([]entity.ModelType, error) {
	return []entity.ModelType{entity.ModelTypeChat}, nil
}

type feedbackContextKey struct{}

type fakeFeedbackDocEngine struct {
	engine.DocEngine
	adjustCalls []struct {
		ctx       context.Context
		indexName string
		chunkID   string
		kbID      string
		delta     float64
		minWeight float64
		maxWeight float64
	}
}

func (f *fakeFeedbackDocEngine) GetType() string { return "elasticsearch" }

func (f *fakeFeedbackDocEngine) AdjustChunkPagerank(ctx context.Context, indexName, chunkID, kbID string, delta, minWeight, maxWeight float64) error {
	f.adjustCalls = append(f.adjustCalls, struct {
		ctx       context.Context
		indexName string
		chunkID   string
		kbID      string
		delta     float64
		minWeight float64
		maxWeight float64
	}{ctx: ctx, indexName: indexName, chunkID: chunkID, kbID: kbID, delta: delta, minWeight: minWeight, maxWeight: maxWeight})
	return nil
}

type fakeInfinityFeedbackDocEngine struct {
	fakeFeedbackDocEngine
	getChunkCalled bool
}

func (f *fakeInfinityFeedbackDocEngine) GetType() string { return string(engine.EngineInfinity) }

func (f *fakeInfinityFeedbackDocEngine) GetChunk(ctx context.Context, indexName, chunkID string, datasetIDs []string) (interface{}, error) {
	f.getChunkCalled = true
	return nil, errors.New("fallback should not be used")
}

type fakeFallbackFeedbackDocEngine struct {
	engine.DocEngine
	chunks      map[string]map[string]interface{}
	updateCalls []struct {
		ctx       context.Context
		condition map[string]interface{}
		newValue  map[string]interface{}
		indexName string
		kbID      string
	}
}

func (f *fakeFallbackFeedbackDocEngine) GetType() string { return "elasticsearch" }

func (f *fakeFallbackFeedbackDocEngine) GetChunk(ctx context.Context, indexName, chunkID string, datasetIDs []string) (interface{}, error) {
	chunk, ok := f.chunks[chunkID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return chunk, nil
}

func (f *fakeFallbackFeedbackDocEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, indexName string, kbID string) error {
	f.updateCalls = append(f.updateCalls, struct {
		ctx       context.Context
		condition map[string]interface{}
		newValue  map[string]interface{}
		indexName string
		kbID      string
	}{ctx: ctx, condition: condition, newValue: newValue, indexName: indexName, kbID: kbID})
	return nil
}

func requireFloatClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v want %v", got, want)
	}
}

// ===================================================================
// ListChatSessions tests
// ===================================================================

func TestListChatSessions_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["s1"] = &entity.ChatSession{ID: "s1", DialogID: "chat-1"}
	store.sessions["s2"] = &entity.ChatSession{ID: "s2", DialogID: "chat-1"}
	store.dialogExists["tenant-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-1"}},
		pipeline:       &fakePipeline{},
	}

	ctx := t.Context()
	resp, err := svc.ListChatSessions(ctx, "user-1", "chat-1", "", "", []dao.OrderTerm{{Column: "create_time", Desc: true}}, 1, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
	}
}

func TestListChatSessions_NotOwner(t *testing.T) {
	store := newFakeSessionStore()

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-other"}},
		pipeline:       &fakePipeline{},
	}

	ctx := t.Context()
	_, err := svc.ListChatSessions(ctx, "user-1", "chat-1", "", "", []dao.OrderTerm{{Column: "create_time", Desc: true}}, 1, 30)
	if err == nil || !strings.Contains(err.Error(), "no authorization") {
		t.Fatalf("got %v", err)
	}
}

// ===================================================================
// GetSession / UpdateSession tests
// ===================================================================

func TestGetSession_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:       "session-1",
		DialogID: "chat-1",
		Name:     strPtr("session"),
		Message:  json.RawMessage(`[{"role":"assistant","content":"hello"}]`),
		Reference: json.RawMessage(`[
			{"chunks":[{"chunk_id":"chunk-1","content_with_weight":"hello","doc_id":"doc-1","docnm_kwd":"Doc 1","kb_id":"kb-1"}]},
			[]
		]`),
		UserID: strPtr("user-1"),
	}
	icon := "avatar.png"
	store.dialogs["chat-1"] = &entity.Chat{ID: "chat-1", Icon: &icon}
	store.dialogExists["user-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	ctx := t.Context()
	resp, code, err := svc.GetSession(ctx, "user-1", "chat-1", "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("unexpected code: %v", code)
	}
	if resp.ChatID != "chat-1" {
		t.Fatalf("chat_id=%q", resp.ChatID)
	}
	if resp.Avatar == nil || *resp.Avatar != "avatar.png" {
		t.Fatalf("avatar=%v", resp.Avatar)
	}
	if len(resp.Messages) != 1 || resp.Messages[0]["content"] != "hello" {
		t.Fatalf("messages=%#v", resp.Messages)
	}
	if len(resp.Reference) != 2 {
		t.Fatalf("reference len=%d", len(resp.Reference))
	}
	firstRef, ok := resp.Reference[0].(map[string]interface{})
	if !ok {
		t.Fatalf("reference[0] type=%T", resp.Reference[0])
	}
	chunks, ok := firstRef["chunks"].([]FormattedChunk)
	if !ok {
		t.Fatalf("chunks type=%T", firstRef["chunks"])
	}
	if len(chunks) != 1 || chunks[0].ID != "chunk-1" {
		t.Fatalf("chunks=%#v", chunks)
	}
	if _, ok := resp.Reference[1].([]interface{}); !ok {
		t.Fatalf("reference[1] changed unexpectedly: %T", resp.Reference[1])
	}
}

func TestGetSession_NotOwner(t *testing.T) {
	svc := &ChatSessionService{
		chatSessionDAO: newFakeSessionStore(),
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	ctx := t.Context()
	_, code, err := svc.GetSession(ctx, "user-1", "chat-1", "session-1")
	if err == nil || err.Error() != "no authorization" {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("code=%v", code)
	}
}

func TestGetSession_WrongChat(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{ID: "session-1", DialogID: "chat-2"}
	store.dialogExists["user-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	ctx := t.Context()
	_, code, err := svc.GetSession(ctx, "user-1", "chat-1", "session-1")
	if err == nil || err.Error() != "session does not belong to this chat" {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%v", code)
	}
}

func TestUpdateSession_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:       "session-1",
		DialogID: "chat-1",
		Name:     strPtr("old"),
		Message:  json.RawMessage(`[{"role":"assistant","content":"hello"}]`),
	}
	store.dialogExists["user-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	longName := "  " + strings.Repeat("x", 260) + "  "
	ctx := t.Context()
	resp, code, err := svc.UpdateSession(ctx, "user-1", "chat-1", "session-1", map[string]interface{}{
		"name":    longName,
		"user_id": "spoof",
		"chat_id": "spoof-chat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if resp.Name == nil || len(*resp.Name) != 255 {
		t.Fatalf("name=%v", resp.Name)
	}
	if len(store.updateCalled) != 1 {
		t.Fatalf("update calls=%d", len(store.updateCalled))
	}
	if _, ok := store.updateCalled[0].updates["user_id"]; ok {
		t.Fatalf("unexpected user_id update: %#v", store.updateCalled[0].updates)
	}
	if _, ok := store.updateCalled[0].updates["chat_id"]; ok {
		t.Fatalf("unexpected chat_id update: %#v", store.updateCalled[0].updates)
	}
	if !reflect.DeepEqual(resp.Messages, []map[string]interface{}{{"role": "assistant", "content": "hello"}}) {
		t.Fatalf("messages=%#v", resp.Messages)
	}
}

func TestUpdateSession_ValidationErrors(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{ID: "session-1", DialogID: "chat-1"}
	store.dialogExists["user-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	cases := []struct {
		name    string
		req     map[string]interface{}
		message string
		code    common.ErrorCode
	}{
		// Empty body is now a valid no-op per the contract.
		{name: "message", req: map[string]interface{}{"message": []interface{}{}}, message: "`messages` cannot be changed", code: common.CodeDataError},
		{name: "messages", req: map[string]interface{}{"messages": []interface{}{}}, message: "`messages` cannot be changed", code: common.CodeDataError},
		{name: "reference", req: map[string]interface{}{"reference": []interface{}{}}, message: "`reference` cannot be changed", code: common.CodeDataError},
		{name: "empty name", req: map[string]interface{}{"name": "   "}, message: "`name` can not be empty", code: common.CodeDataError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			_, code, err := svc.UpdateSession(ctx, "user-1", "chat-1", "session-1", tc.req)
			if err == nil || err.Error() != tc.message {
				t.Fatalf("err=%v", err)
			}
			if code != tc.code {
				t.Fatalf("code=%v", code)
			}
		})
	}
}

func TestUpdateSession_NotFound(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogExists["user-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	ctx := t.Context()
	_, code, err := svc.UpdateSession(ctx, "user-1", "chat-1", "missing", map[string]interface{}{"name": "renamed"})
	if err == nil || err.Error() != "Session not found!" {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%v", code)
	}
}

// ===================================================================
// Session message delete / feedback tests
// ===================================================================

func TestDeleteSessionMessage_RemovesMessagePairAndReference(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:       "session-1",
		DialogID: "chat-1",
		UserID:   strPtr("user-1"),
		Message: json.RawMessage(`[
			{"role":"assistant","content":"Welcome!"},
			{"role":"user","content":"first","id":"msg-1"},
			{"role":"assistant","content":"answer 1","id":"msg-1"},
			{"role":"user","content":"second","id":"msg-2"},
			{"role":"assistant","content":"answer 2","id":"msg-2"}
		]`),
		Reference: json.RawMessage(`[
			{"chunks":[{"id":"chunk-1","kb_id":"kb-1"}]},
			{"chunks":[{"id":"chunk-2","kb_id":"kb-2"}]}
		]`),
	}
	store.dialogExists["tenant-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-1"}},
		pipeline:       &fakePipeline{},
	}

	ctx := t.Context()
	resp, code, err := svc.DeleteSessionMessage(ctx, "user-1", "chat-1", "session-1", "msg-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if len(resp.Messages) != 3 {
		t.Fatalf("messages=%#v", resp.Messages)
	}
	if resp.Messages[1]["id"] != "msg-2" || resp.Messages[2]["id"] != "msg-2" {
		t.Fatalf("remaining pair=%#v", resp.Messages)
	}
	if len(resp.Reference) != 1 {
		t.Fatalf("reference=%#v", resp.Reference)
	}
	ref, ok := resp.Reference[0].(map[string]interface{})
	if !ok {
		t.Fatalf("reference type=%T", resp.Reference[0])
	}
	chunks, ok := ref["chunks"].([]FormattedChunk)
	if !ok || len(chunks) != 1 || chunks[0].ID != "chunk-2" {
		t.Fatalf("remaining chunks=%#v", ref["chunks"])
	}
}

func TestUpdateMessageFeedback_AppliesChunkFeedbackWithResolvedTenantAndContext(t *testing.T) {
	t.Setenv("CHUNK_FEEDBACK_ENABLED", "true")
	t.Setenv("CHUNK_FEEDBACK_WEIGHTING", "uniform")

	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:       "session-1",
		DialogID: "chat-1",
		UserID:   strPtr("user-1"),
		Message: json.RawMessage(`[
			{"role":"assistant","content":"Welcome!"},
			{"role":"user","content":"question","id":"msg-1"},
			{"role":"assistant","content":"answer","id":"msg-1","feedback":"old"}
		]`),
		Reference: json.RawMessage(`[
			{"chunks":[{"id":"chunk-1","kb_id":"kb-1","similarity":0.9}]}
		]`),
	}
	store.dialogExists["tenant-owner|chat-1"] = true
	docEngine := &fakeFeedbackDocEngine{}
	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-owner"}},
		pipeline:       &fakePipeline{},
		docEngine:      docEngine,
	}
	ctx := context.WithValue(t.Context(), feedbackContextKey{}, "request-context")

	resp, code, err := svc.UpdateMessageFeedback(ctx, "user-1", "chat-1", "session-1", "msg-1", map[string]interface{}{
		"thumbup": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if len(docEngine.adjustCalls) != 1 {
		t.Fatalf("adjust calls=%d", len(docEngine.adjustCalls))
	}
	call := docEngine.adjustCalls[0]
	if call.indexName != "ragflow_tenant-owner" || call.chunkID != "chunk-1" || call.kbID != "kb-1" {
		t.Fatalf("call=%#v", call)
	}
	if call.delta != 1 || call.minWeight != 0 || call.maxWeight != 100 {
		t.Fatalf("weights=%#v", call)
	}
	if got := call.ctx.Value(feedbackContextKey{}); got != "request-context" {
		t.Fatalf("context value=%v", got)
	}
	assistant := resp.Messages[2]
	if assistant["thumbup"] != true {
		t.Fatalf("assistant=%#v", assistant)
	}
	if _, ok := assistant["feedback"]; ok {
		t.Fatalf("positive feedback should remove text feedback: %#v", assistant)
	}
}

func TestUpdateMessageFeedback_ToggleUsesResolvedTenantForUndoAndApply(t *testing.T) {
	t.Setenv("CHUNK_FEEDBACK_ENABLED", "true")
	t.Setenv("CHUNK_FEEDBACK_WEIGHTING", "uniform")

	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:       "session-1",
		DialogID: "chat-1",
		UserID:   strPtr("user-1"),
		Message: json.RawMessage(`[
			{"role":"assistant","content":"Welcome!"},
			{"role":"user","content":"question","id":"msg-1"},
			{"role":"assistant","content":"answer","id":"msg-1","thumbup":true}
		]`),
		Reference: json.RawMessage(`[
			{"chunks":[{"chunk_id":"chunk-1","dataset_id":"kb-1"}]}
		]`),
	}
	store.dialogExists["tenant-owner|chat-1"] = true
	docEngine := &fakeFeedbackDocEngine{}
	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-owner"}},
		pipeline:       &fakePipeline{},
		docEngine:      docEngine,
	}

	resp, code, err := svc.UpdateMessageFeedback(t.Context(), "user-1", "chat-1", "session-1", "msg-1", map[string]interface{}{
		"thumbup":  false,
		"feedback": "not useful",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if len(docEngine.adjustCalls) != 2 {
		t.Fatalf("adjust calls=%d", len(docEngine.adjustCalls))
	}
	for _, call := range docEngine.adjustCalls {
		if call.indexName != "ragflow_tenant-owner" || call.delta != -1 {
			t.Fatalf("call=%#v", call)
		}
	}
	assistant := resp.Messages[2]
	if assistant["thumbup"] != false || assistant["feedback"] != "not useful" {
		t.Fatalf("assistant=%#v", assistant)
	}
}

func TestApplyChunkFeedback_DisabledDoesNotTouchEngine(t *testing.T) {
	t.Setenv("CHUNK_FEEDBACK_ENABLED", "false")
	docEngine := &fakeFeedbackDocEngine{}
	svc := &ChatSessionService{docEngine: docEngine}

	result, err := svc.applyChunkFeedback(t.Context(), "tenant-1", map[string]interface{}{
		"chunks": []interface{}{map[string]interface{}{"id": "chunk-1", "kb_id": "kb-1"}},
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["disabled"] != true {
		t.Fatalf("result=%#v", result)
	}
	if len(docEngine.adjustCalls) != 0 {
		t.Fatalf("adjust calls=%d", len(docEngine.adjustCalls))
	}
}

func TestApplyChunkFeedback_UniformSplitsOneVoteAcrossChunks(t *testing.T) {
	t.Setenv("CHUNK_FEEDBACK_ENABLED", "true")
	t.Setenv("CHUNK_FEEDBACK_WEIGHTING", "uniform")

	docEngine := &fakeFeedbackDocEngine{}
	svc := &ChatSessionService{docEngine: docEngine}

	result, err := svc.applyChunkFeedback(t.Context(), "tenant-1", map[string]interface{}{
		"chunks": []interface{}{
			map[string]interface{}{"id": "chunk-1", "kb_id": "kb-1"},
			map[string]interface{}{"id": "chunk-2", "kb_id": "kb-1"},
		},
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["success_count"] != 2 {
		t.Fatalf("result=%#v", result)
	}
	if len(docEngine.adjustCalls) != 2 {
		t.Fatalf("adjust calls=%d", len(docEngine.adjustCalls))
	}
	requireFloatClose(t, docEngine.adjustCalls[0].delta, 0.5)
	requireFloatClose(t, docEngine.adjustCalls[1].delta, 0.5)
	requireFloatClose(t, docEngine.adjustCalls[0].delta+docEngine.adjustCalls[1].delta, 1)
}

func TestApplyChunkFeedback_RelevanceDistributesOneVoteBySignals(t *testing.T) {
	t.Setenv("CHUNK_FEEDBACK_ENABLED", "true")
	t.Setenv("CHUNK_FEEDBACK_WEIGHTING", "relevance")

	docEngine := &fakeFeedbackDocEngine{}
	svc := &ChatSessionService{docEngine: docEngine}

	result, err := svc.applyChunkFeedback(t.Context(), "tenant-1", map[string]interface{}{
		"chunks": []interface{}{
			map[string]interface{}{"id": "chunk-1", "kb_id": "kb-1", "similarity": 2.0},
			map[string]interface{}{"id": "chunk-2", "kb_id": "kb-1", "vector_similarity": 1.0},
			map[string]interface{}{"id": "chunk-3", "kb_id": "kb-1"},
		},
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["success_count"] != 3 {
		t.Fatalf("result=%#v", result)
	}
	if len(docEngine.adjustCalls) != 3 {
		t.Fatalf("adjust calls=%d", len(docEngine.adjustCalls))
	}
	requireFloatClose(t, docEngine.adjustCalls[0].delta, -0.5)
	requireFloatClose(t, docEngine.adjustCalls[1].delta, -0.25)
	requireFloatClose(t, docEngine.adjustCalls[2].delta, -0.25)
	requireFloatClose(t, docEngine.adjustCalls[0].delta+docEngine.adjustCalls[1].delta+docEngine.adjustCalls[2].delta, -1)
}

func TestUpdateChunkWeight_InfinityUsesAtomicAdjuster(t *testing.T) {
	docEngine := &fakeInfinityFeedbackDocEngine{}
	svc := &ChatSessionService{docEngine: docEngine}

	if ok := svc.updateChunkWeight(t.Context(), "tenant-1", "chunk-1", "kb-1", 0.25); !ok {
		t.Fatal("expected updateChunkWeight to succeed")
	}
	if docEngine.getChunkCalled {
		t.Fatal("expected Infinity adjuster path, got GetChunk fallback")
	}
	if len(docEngine.adjustCalls) != 1 {
		t.Fatalf("adjust calls=%d", len(docEngine.adjustCalls))
	}
	call := docEngine.adjustCalls[0]
	if call.indexName != "ragflow_tenant-1" || call.chunkID != "chunk-1" || call.kbID != "kb-1" {
		t.Fatalf("call=%#v", call)
	}
	requireFloatClose(t, call.delta, 0.25)
}

func TestApplyChunkFeedback_FallbackClampsAndRemovesPagerank(t *testing.T) {
	t.Setenv("CHUNK_FEEDBACK_ENABLED", "true")
	t.Setenv("CHUNK_FEEDBACK_WEIGHTING", "uniform")

	docEngine := &fakeFallbackFeedbackDocEngine{
		chunks: map[string]map[string]interface{}{
			"chunk-1": {common.PAGERANK_FLD: 0},
		},
	}
	svc := &ChatSessionService{docEngine: docEngine}

	result, err := svc.applyChunkFeedback(t.Context(), "tenant-1", map[string]interface{}{
		"chunks": []interface{}{map[string]interface{}{"id": "chunk-1", "kb_id": "kb-1"}},
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["success_count"] != 1 {
		t.Fatalf("result=%#v", result)
	}
	if len(docEngine.updateCalls) != 1 {
		t.Fatalf("update calls=%d", len(docEngine.updateCalls))
	}
	call := docEngine.updateCalls[0]
	if call.indexName != "ragflow_tenant-1" || call.kbID != "kb-1" {
		t.Fatalf("call=%#v", call)
	}
	if call.condition["id"] != "chunk-1" || call.newValue["remove"] != common.PAGERANK_FLD {
		t.Fatalf("call=%#v", call)
	}
}

// ===================================================================
// Completion tests
// ===================================================================

func TestChatCompletions_AppendOnly(t *testing.T) {
	store := newFakeSessionStore()
	session := &entity.ChatSession{
		ID: "session-1", DialogID: "dialog-1",
		Message:   json.RawMessage(`[{"role":"assistant","content":"Welcome!"}]`),
		Reference: json.RawMessage(`[]`),
	}
	store.dialogExists["user-1|dialog-1"] = true
	store.sessions["session-1"] = session
	store.dialogs["dialog-1"] = &entity.Chat{
		ID: "dialog-1", TenantID: "user-1", LLMID: "chat@factory",
		LLMSetting: entity.JSONMap{},
	}

	pipeline := &fakePipeline{
		resultChan: makeResultChan(
			AsyncChatResult{Answer: "Hello", Reference: map[string]interface{}{"chunks": []interface{}{}}},
			// The real pipeline's final result carries the complete answer,
			// not a trailing delta.
			AsyncChatResult{Answer: "Hello world", Final: true, Reference: map[string]interface{}{"chunks": []interface{}{}}},
		),
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       pipeline,
	}

	ctx := t.Context()
	result, err := svc.ChatCompletions(ctx, "user-1", "dialog-1", "session-1", []map[string]interface{}{
		{"role": "user", "content": "ignored client history"},
		{"id": "msg-1", "role": "user", "content": "hi"},
	}, "", nil, "", nil, nil, false, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ans, _ := result["answer"].(string)
	if ans != "Hello world" {
		t.Fatalf("expected answer 'Hello world', got %q", ans)
	}
	if pipeline.userID != "user-1" {
		t.Fatalf("pipeline userID = %q, want user-1", pipeline.userID)
	}

	got := parseMessages(store.sessions["session-1"].Message)
	if len(got) != 3 {
		t.Fatalf("stored messages=%#v", got)
	}
	if got[0]["role"] != "assistant" || got[0]["content"] != "Welcome!" {
		t.Fatalf("stored prologue=%#v", got[0])
	}
	if got[1]["role"] != "user" || got[1]["content"] != "hi" {
		t.Fatalf("stored user message=%#v", got[1])
	}
	if got[2]["role"] != "assistant" || got[2]["content"] != "Hello world" || got[2]["id"] != "msg-1" {
		t.Fatalf("stored assistant message=%#v", got[2])
	}
	if len(store.updateCalled) != 2 {
		t.Fatalf("want separate question and answer writes, got %d", len(store.updateCalled))
	}
	pending := parseMessages(store.updateCalled[0].updates["message"].([]byte))
	if len(pending) != 2 || pending[1]["role"] != "user" || pending[1]["created_at"] != got[1]["created_at"] {
		t.Fatalf("first write must contain only the prologue and timestamped question: %#v", pending)
	}
	if got[2]["created_at"].(float64) < got[1]["created_at"].(float64) {
		t.Fatalf("answer timestamp precedes question: %#v", got)
	}

	t.Run("non-storing model tests", func(t *testing.T) {
		stored := store.sessions["session-1"]
		storedMessages, storedReference := string(stored.Message), string(stored.Reference)
		payload := []map[string]interface{}{
			{"role": "system", "content": "client system prompt"},
			{"role": "user", "content": "earlier question"},
			{"role": "assistant", "content": "earlier answer"},
			{"id": "test-question", "role": "user", "content": "latest question"},
		}
		var wg sync.WaitGroup
		for _, sessionID := range []string{"", "session-1"} {
			for _, stream := range []bool{false, true} {
				for _, legacy := range []bool{false, true} {
					wg.Add(1)
					go func() {
						defer wg.Done()
						ref := map[string]interface{}{"chunks": []interface{}{"test chunk"}}
						pipeline := &fakePipeline{resultChan: makeResultChan(
							AsyncChatResult{Answer: "test answer", Reference: ref},
							AsyncChatResult{Answer: "test answer", Reference: ref, Final: true},
						)}
						svc := &ChatSessionService{chatSessionDAO: store, userTenantDAO: &fakeTenantStore{}, pipeline: pipeline}
						streamChan := make(chan string, 8)
						result, err := svc.ChatCompletions(t.Context(), "user-1", "dialog-1", sessionID, payload, "must not replace payload", nil, "", nil, map[string]interface{}{"store_history_messages": false}, legacy, stream, streamChan)
						if err != nil {
							t.Errorf("completion failed: %v", err)
							return
						}
						if !reflect.DeepEqual(pipeline.messages, payload) {
							t.Errorf("full payload not passed: %#v", pipeline.messages)
						}
						if !stream && (result["answer"] != "test answer" || !reflect.DeepEqual(result["reference"], ref)) {
							t.Errorf("unexpected result: %#v", result)
						}
						if stream {
							foundReference := false
							for len(streamChan) > 0 {
								foundReference = strings.Contains(<-streamChan, "test chunk") || foundReference
							}
							if !foundReference {
								t.Error("stream lost reference")
							}
						}
					}()
				}
			}
		}
		wg.Wait()
		stored = store.sessions["session-1"]
		if len(store.updateCalled) != 2 || len(store.createCalled) != 0 || string(stored.Message) != storedMessages || string(stored.Reference) != storedReference {
			t.Fatal("model tests changed stored history or created a session")
		}
		if _, err := svc.ChatCompletions(t.Context(), "user-1", "dialog-1", "", nil, "question without messages", nil, "", nil, map[string]interface{}{"store_history_messages": false}, false, false, nil); err == nil {
			t.Fatal("non-storing completion must require messages")
		}
	})
}

func TestChatCompletionsPassesRequestUserIDToPipeline(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:        "session-1",
		DialogID:  "dialog-1",
		UserID:    strPtr("user-1"),
		Message:   json.RawMessage(`[{"role":"assistant","content":"Welcome!"}]`),
		Reference: json.RawMessage(`[]`),
	}
	store.dialogs["dialog-1"] = &entity.Chat{
		ID:         "dialog-1",
		TenantID:   "tenant-owner",
		LLMID:      "chat@factory",
		LLMSetting: entity.JSONMap{},
		PromptConfig: entity.JSONMap{
			"parameters": []interface{}{},
		},
	}
	store.dialogExists["tenant-owner|dialog-1"] = true

	pipeline := &fakePipeline{
		resultChan: makeResultChan(
			AsyncChatResult{Answer: "ok", Final: true, Reference: map[string]interface{}{"chunks": []interface{}{}}},
		),
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-owner"}},
		pipeline:       pipeline,
	}

	_, err := svc.ChatCompletions(
		t.Context(),
		"user-1",
		"dialog-1",
		"session-1",
		[]map[string]interface{}{{"role": "user", "content": "hi"}},
		"",
		nil,
		"",
		nil,
		nil,
		false,
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("ChatCompletions failed: %v", err)
	}
	if pipeline.userID != "user-1" {
		t.Fatalf("pipeline userID = %q, want request user user-1", pipeline.userID)
	}
	if pipeline.userID == store.dialogs["dialog-1"].TenantID {
		t.Fatalf("pipeline used dialog tenant %q instead of request user", pipeline.userID)
	}
}

func TestChatCompletionsStreamFinalCarriesDecoratedReference(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:        "session-1",
		DialogID:  "dialog-1",
		UserID:    strPtr("user-1"),
		Message:   json.RawMessage(`[{"role":"assistant","content":"Welcome!"}]`),
		Reference: json.RawMessage(`[]`),
	}
	store.dialogs["dialog-1"] = &entity.Chat{
		ID:         "dialog-1",
		TenantID:   "tenant-owner",
		LLMID:      "chat@factory",
		LLMSetting: entity.JSONMap{},
		PromptConfig: entity.JSONMap{
			"parameters": []interface{}{},
		},
	}
	store.dialogExists["tenant-owner|dialog-1"] = true

	finalReference := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{
				"id":            "chunk-1",
				"content":       "Marigold is a depth-estimation model.",
				"document_id":   "doc-1",
				"document_name": "paper.pdf",
			},
		},
		"doc_aggs": []interface{}{
			map[string]interface{}{"doc_id": "doc-1", "doc_name": "paper.pdf", "count": 1},
		},
		"total": 1,
	}
	pipeline := &fakePipeline{
		resultChan: makeResultChan(
			AsyncChatResult{StartToThink: true, Reference: map[string]interface{}{"chunks": []interface{}{}}, Final: false},
			AsyncChatResult{Reasoning: "checking sources", Reference: map[string]interface{}{"chunks": []interface{}{}}, Final: false},
			AsyncChatResult{EndToThink: true, Reference: map[string]interface{}{"chunks": []interface{}{}}, Final: false},
			AsyncChatResult{Answer: "Marigold is a depth-estimation model.", Reference: map[string]interface{}{"chunks": []interface{}{}}, Final: false},
			AsyncChatResult{
				Answer:    "Marigold is a depth-estimation model. [ID:0]",
				Reference: finalReference,
				Prompt:    "### Query: what is marigold",
				CreatedAt: 123,
				Final:     true,
			},
		),
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-owner"}},
		pipeline:       pipeline,
	}

	streamChan := make(chan string, 8)
	_, err := svc.ChatCompletions(
		t.Context(),
		"user-1",
		"dialog-1",
		"session-1",
		[]map[string]interface{}{{"role": "user", "content": "what is marigold"}},
		"",
		nil,
		"",
		nil,
		nil,
		false,
		true,
		streamChan,
	)
	if err != nil {
		t.Fatalf("ChatCompletions failed: %v", err)
	}

	var finalData map[string]interface{}
	eventCount := len(streamChan)
	for i := 0; i < eventCount; i++ {
		event := <-streamChan
		payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(event), "data:"))
		var wrapper map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &wrapper); err != nil {
			t.Fatalf("failed to parse SSE payload %q: %v", payload, err)
		}
		data, ok := wrapper["data"].(map[string]interface{})
		if !ok {
			continue
		}
		if final, _ := data["final"].(bool); final {
			finalData = data
			break
		}
	}
	if finalData == nil {
		t.Fatal("missing final SSE data")
	}
	if got := finalData["answer"]; got != "Marigold is a depth-estimation model. [ID:0]" {
		t.Fatalf("final answer = %v", got)
	}
	if got := finalData["prompt"]; got != "### Query: what is marigold" {
		t.Fatalf("final prompt = %v", got)
	}
	ref, ok := finalData["reference"].(map[string]interface{})
	if !ok {
		t.Fatalf("final reference = %#v", finalData["reference"])
	}
	chunks, ok := ref["chunks"].([]interface{})
	if !ok || len(chunks) != 1 {
		t.Fatalf("final reference chunks = %#v", ref["chunks"])
	}
	docAggs, ok := ref["doc_aggs"].([]interface{})
	if !ok || len(docAggs) != 1 {
		t.Fatalf("final reference doc_aggs = %#v", ref["doc_aggs"])
	}
	if got := ref["total"]; got != float64(1) {
		t.Fatalf("final reference total = %v", got)
	}
	stored := parseMessages(store.sessions["session-1"].Message)
	if got := stored[len(stored)-1]["content"]; got != "<think>checking sources</think>Marigold is a depth-estimation model." {
		t.Fatalf("stored assistant content = %q, want tagged reasoning and visible answer", got)
	}
}

func TestChatCompletionsModelIDOverrideUsesModelResolver(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:        "session-1",
		DialogID:  "dialog-1",
		UserID:    strPtr("user-1"),
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}
	store.dialogs["dialog-1"] = &entity.Chat{
		ID:           "dialog-1",
		TenantID:     "tenant-owner",
		LLMID:        "old-model@default@Provider",
		LLMSetting:   entity.JSONMap{},
		PromptConfig: entity.JSONMap{"parameters": []interface{}{}},
	}
	store.dialogExists["tenant-owner|dialog-1"] = true

	pipeline := &fakePipeline{
		resultChan: makeResultChan(
			AsyncChatResult{Answer: "ok", Final: true, Reference: map[string]interface{}{"chunks": []interface{}{}}},
		),
	}
	resolver := &fakeChatModelConfigResolver{}
	modelID := "3d2d824e7e5d11f1a845455b140cef90"

	svc := &ChatSessionService{
		chatSessionDAO:   store,
		userTenantDAO:    &fakeTenantStore{tenantIDs: []string{"tenant-owner"}},
		pipeline:         pipeline,
		modelProviderSvc: resolver,
	}

	_, err := svc.ChatCompletions(
		t.Context(),
		"user-1",
		"dialog-1",
		"session-1",
		[]map[string]interface{}{{"role": "user", "content": "hi"}},
		"",
		nil,
		modelID,
		nil,
		nil,
		false,
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("ChatCompletions failed: %v", err)
	}
	if resolver.tenantID != "tenant-owner" || resolver.llmID != modelID {
		t.Fatalf("resolver got tenantID=%q llmID=%q, want tenant-owner/%s", resolver.tenantID, resolver.llmID, modelID)
	}
	if store.dialogs["dialog-1"].LLMID != modelID {
		t.Fatalf("dialog LLMID=%q, want model id %q", store.dialogs["dialog-1"].LLMID, modelID)
	}
}

func TestStructureAnswerWithConv_ParsesArrayMessages(t *testing.T) {
	session := &entity.ChatSession{
		ID:      "session-1",
		Message: json.RawMessage(`[{"role":"assistant","content":"Welcome!"}]`),
	}
	svc := &ChatSessionService{}

	ans := svc.structureAnswerWithConv(session, map[string]interface{}{
		"answer":    "Final answer",
		"reference": map[string]interface{}{"chunks": []interface{}{}},
		"final":     true,
	}, "msg-1", "session-1", []interface{}{map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}}})

	if ans["id"] != "msg-1" || ans["session_id"] != "session-1" {
		t.Fatalf("ans=%#v", ans)
	}

	got := parseMessages(session.Message)
	if len(got) != 1 {
		t.Fatalf("stored messages=%#v", got)
	}
	if got[0]["role"] != "assistant" || got[0]["content"] != "Final answer" || got[0]["id"] != "msg-1" {
		t.Fatalf("stored assistant message=%#v", got[0])
	}
}

func TestParseMessages_LegacyWrappedObject(t *testing.T) {
	got := parseMessages(json.RawMessage(`{"messages":[{"role":"assistant","content":"legacy"}]}`))
	if !reflect.DeepEqual(got, []map[string]interface{}{{"role": "assistant", "content": "legacy"}}) {
		t.Fatalf("messages=%#v", got)
	}
}

func TestBuildSessionPayload_EmptyCollectionsEncodeAsEmptyArrays(t *testing.T) {
	svc := &ChatSessionService{}
	payload := svc.buildSessionPayload(&entity.ChatSession{
		ID:        "session-1",
		DialogID:  "chat-1",
		Message:   nil,
		Reference: json.RawMessage(`null`),
	}, nil, false)

	if payload.Messages == nil {
		t.Fatal("messages is nil")
	}
	if payload.Reference == nil {
		t.Fatal("reference is nil")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(body), `"messages":[]`) {
		t.Fatalf("messages did not encode as empty array: %s", string(body))
	}
	if !strings.Contains(string(body), `"reference":[]`) {
		t.Fatalf("reference did not encode as empty array: %s", string(body))
	}
}

func TestParseCollections_ReturnEmptySlicesForMissingOrNull(t *testing.T) {
	messageInputs := []json.RawMessage{
		nil,
		json.RawMessage(`null`),
		json.RawMessage(`{"messages":null}`),
	}
	for _, input := range messageInputs {
		got := parseMessages(input)
		if got == nil || len(got) != 0 {
			t.Fatalf("parseMessages(%s)=%#v", string(input), got)
		}
	}

	referenceInputs := []json.RawMessage{
		nil,
		json.RawMessage(`null`),
	}
	for _, input := range referenceInputs {
		got := parseReferenceList(input)
		if got == nil || len(got) != 0 {
			t.Fatalf("parseReferenceList(%s)=%#v", string(input), got)
		}
	}
}

func TestParseCollections_ReturnNilForMalformedData(t *testing.T) {
	messageInputs := []json.RawMessage{
		json.RawMessage(`not-json`),
		json.RawMessage(`{"unexpected":[]}`),
	}
	for _, input := range messageInputs {
		if got := parseMessages(input); got != nil {
			t.Fatalf("parseMessages(%s)=%#v, want nil", string(input), got)
		}
	}

	referenceInputs := []json.RawMessage{
		json.RawMessage(`not-json`),
		json.RawMessage(`{"unexpected":[]}`),
	}
	for _, input := range referenceInputs {
		if got := parseReferenceList(input); got != nil {
			t.Fatalf("parseReferenceList(%s)=%#v, want nil", string(input), got)
		}
	}
}

// ===================================================================
// chunksFormat tests — verifies field normalization after the rewrite.
// ===================================================================

// TestChunksFormat_ContentPrefersContent pins Python's precedence in
// chunks_format: `get_value(chunk, "content", "content_with_weight")`
// (rag/prompts/generator.py:50) reads the display field FIRST and only falls
// back to the engine spelling. The two normally hold the same string, so the
// order is only observable when they differ.
func TestChunksFormat_ContentPrefersContent(t *testing.T) {
	svc := &ChatSessionService{}
	result := svc.chunksFormat(map[string]interface{}{
		"chunks": []map[string]interface{}{{
			"chunk_id":            "c1",
			"content":             "display text",
			"content_with_weight": "engine body",
		}},
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}
	if got := result[0]["content"]; got != "display text" {
		t.Fatalf("content=%v, want display text (Python reads content first)", got)
	}
}

func TestChunksFormat_NormalizesRawFieldNames(t *testing.T) {
	svc := &ChatSessionService{}
	ref := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{
				"chunk_id":            "c1",
				"content_with_weight": "hello world",
				"content_ltks":        "hello world ltks",
				"doc_id":              "d1",
				"docnm_kwd":           "Document 1",
				"kb_id":               "kb1",
				"image_id":            "img1",
				"img_id":              "img2",
				"positions":           []int{0, 10},
				"position_int":        []int{1, 11},
				"doc_type_kwd":        "pdf",
				"similarity":          0.95,
				"vector_similarity":   0.9,
				"term_similarity":     0.85,
				"row_id":              "r1",
				"url":                 "http://example.com",
				"document_metadata":   map[string]interface{}{"author": "Alice"},
			},
		},
	}

	result := svc.chunksFormat(ref)
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}
	c := result[0]

	if c["id"] != "c1" {
		t.Fatalf("id=%v", c["id"])
	}
	if c["content"] != "hello world" {
		t.Fatalf("content=%v", c["content"])
	}
	if c["document_id"] != "d1" {
		t.Fatalf("document_id=%v", c["document_id"])
	}
	if c["document_name"] != "Document 1" {
		t.Fatalf("document_name=%v", c["document_name"])
	}
	if c["dataset_id"] != "kb1" {
		t.Fatalf("dataset_id=%v", c["dataset_id"])
	}
	if c["image_id"] != "img1" {
		t.Fatalf("image_id=%v", c["image_id"])
	}
	if c["doc_type"] != "pdf" {
		t.Fatalf("doc_type=%v", c["doc_type"])
	}
	if c["similarity"] != 0.95 {
		t.Fatalf("similarity=%v", c["similarity"])
	}
	if c["url"] != "http://example.com" {
		t.Fatalf("url=%v", c["url"])
	}

	pos, ok := c["positions"].([]int)
	if !ok || len(pos) != 2 || pos[0] != 0 {
		t.Fatalf("positions=%v (%T)", c["positions"], c["positions"])
	}

	// Raw keys must be normalized away.
	if _, exists := c["content_with_weight"]; exists {
		t.Fatal("content_with_weight should not be present after normalization")
	}
	if _, exists := c["content_ltks"]; exists {
		t.Fatal("content_ltks should not be present after normalization")
	}
}

func TestChunksFormat_PreservesAlreadyNormalizedFields(t *testing.T) {
	svc := &ChatSessionService{}
	ref := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{
				"id":            "c2",
				"content":       "already normalized",
				"document_id":   "d2",
				"document_name": "Doc 2",
			},
		},
	}

	result := svc.chunksFormat(ref)
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}
	c := result[0]
	if c["id"] != "c2" {
		t.Fatalf("id=%v", c["id"])
	}
	if c["content"] != "already normalized" {
		t.Fatalf("content=%v", c["content"])
	}
}

func TestChunksFormat_EmptyReference(t *testing.T) {
	svc := &ChatSessionService{}

	if n := len(svc.chunksFormat(nil)); n != 0 {
		t.Fatalf("nil ref: expected 0, got %d", n)
	}
	if n := len(svc.chunksFormat(map[string]interface{}{})); n != 0 {
		t.Fatalf("empty ref: expected 0, got %d", n)
	}
	if n := len(svc.chunksFormat(map[string]interface{}{"chunks": nil})); n != 0 {
		t.Fatalf("nil chunks: expected 0, got %d", n)
	}
	if n := len(svc.chunksFormat(map[string]interface{}{"chunks": []map[string]interface{}{}})); n != 0 {
		t.Fatalf("empty chunks: expected 0, got %d", n)
	}
}

func TestChunksFormat_ChunksAsInterfaceSlice(t *testing.T) {
	svc := &ChatSessionService{}
	ref := map[string]interface{}{
		"chunks": []interface{}{
			map[string]interface{}{
				"chunk_id":            "c3",
				"content_with_weight": "from interface slice",
			},
		},
	}
	result := svc.chunksFormat(ref)
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}
	if result[0]["id"] != "c3" {
		t.Fatalf("id=%v", result[0]["id"])
	}
	if result[0]["content"] != "from interface slice" {
		t.Fatalf("content=%v", result[0]["content"])
	}
}

func TestChunksFormat_IgnoresNonMapItems(t *testing.T) {
	svc := &ChatSessionService{}
	ref := map[string]interface{}{
		"chunks": []interface{}{
			"not a map",
			map[string]interface{}{
				"chunk_id":            "c4",
				"content_with_weight": "valid chunk",
			},
		},
	}
	result := svc.chunksFormat(ref)
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk (non-maps skipped), got %d", len(result))
	}
	if result[0]["id"] != "c4" {
		t.Fatalf("id=%v", result[0]["id"])
	}
}

func TestChunksFormat_UnsupportedTypeReturnsEmpty(t *testing.T) {
	svc := &ChatSessionService{}
	ref := map[string]interface{}{"chunks": "not a slice"}
	result := svc.chunksFormat(ref)
	if len(result) != 0 {
		t.Fatalf("expected empty for string type, got %d", len(result))
	}
}

func drainResults(events []AsyncChatResult) <-chan AsyncChatResult {
	ch := make(chan AsyncChatResult, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

func TestAccumulateNonStreamAnswer_MetadataFromFinalEventOnly(t *testing.T) {
	// Intermediate events carry conflicting metadata; the response must
	// reflect only the final event's values.
	ans := accumulateNonStreamAnswer(drainResults([]AsyncChatResult{
		{Answer: "partial ", AudioBinary: "intermediate-audio", Prompt: "intermediate-prompt", CreatedAt: 111},
		{Answer: "answer", Final: true, AudioBinary: "final-audio", Prompt: "final-prompt", CreatedAt: 222},
	}))
	if ans["audio_binary"] != "final-audio" {
		t.Fatalf("audio_binary=%v, want final-audio", ans["audio_binary"])
	}
	if ans["prompt"] != "final-prompt" {
		t.Fatalf("prompt=%v, want final-prompt", ans["prompt"])
	}
	if ans["created_at"] != float64(222) {
		t.Fatalf("created_at=%v, want 222", ans["created_at"])
	}
	if ans["answer"] != "answer" {
		t.Fatalf("answer=%v, want final decorated answer", ans["answer"])
	}
}

func TestAccumulateNonStreamAnswer_FinalEventOmitsMetadata(t *testing.T) {
	// The final event omits metadata; it must be assigned as-is (nil/zero)
	// rather than leaking values from intermediate events.
	ans := accumulateNonStreamAnswer(drainResults([]AsyncChatResult{
		{Answer: "partial ", AudioBinary: "intermediate-audio", Prompt: "intermediate-prompt", CreatedAt: 111},
		{Answer: "full answer", Final: true},
	}))
	if ans["audio_binary"] != nil {
		t.Fatalf("audio_binary=%v, want nil", ans["audio_binary"])
	}
	if ans["prompt"] != "" {
		t.Fatalf("prompt=%v, want empty", ans["prompt"])
	}
	if _, ok := ans["created_at"]; ok {
		t.Fatalf("created_at should be omitted, got %v", ans["created_at"])
	}
}

func TestAccumulateNonStreamAnswer_AccumulatesDeltasUntilFinal(t *testing.T) {
	ans := accumulateNonStreamAnswer(drainResults([]AsyncChatResult{
		{Answer: "Hello, "},
		{Answer: "world"},
	}))
	if ans["answer"] != "Hello, world" {
		t.Fatalf("answer=%v, want accumulated deltas", ans["answer"])
	}
	if ans["final"] != true {
		t.Fatalf("final=%v, want true", ans["final"])
	}
	if ref, _ := ans["reference"].(map[string]interface{}); ref != nil {
		t.Fatalf("reference=%v, want nil", ref)
	}
}

// ===================================================================
// Shared-session readonly rule (team-shared chats): only the chat owner
// (the dialog tenant) or the session's creator may mutate a session.
// ===================================================================

func newSharedChatReadonlyService() (*ChatSessionService, *fakeSessionStore) {
	store := newFakeSessionStore()
	// The chat is owned by tenant-owner and team-shared: "user-1" joined
	// that tenant, so reads pass ensureOwnedChat.
	store.dialogExists["tenant-owner|chat-1"] = true
	// The session was created by the chat owner, not by user-1.
	owner := "tenant-owner"
	store.sessions["session-1"] = &entity.ChatSession{
		ID:       "session-1",
		DialogID: "chat-1",
		Name:     &owner,
		UserID:   &owner,
		Message:  json.RawMessage(`[{"role":"assistant","content":"Welcome!"}]`),
	}
	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-owner"}},
		pipeline:       &fakePipeline{},
	}
	return svc, store
}

func TestUpdateSession_SharedSessionReadonlyForTeammate(t *testing.T) {
	svc, store := newSharedChatReadonlyService()

	_, code, err := svc.UpdateSession(t.Context(), "user-1", "chat-1", "session-1", map[string]interface{}{"name": "renamed"})
	if err == nil || err.Error() != "shared session is readonly" {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("code=%v", code)
	}
	if len(store.updateCalled) != 0 {
		t.Fatalf("update calls=%d, want 0", len(store.updateCalled))
	}
}

func TestUpdateSession_SessionCreatorCanRenameInSharedChat(t *testing.T) {
	svc, store := newSharedChatReadonlyService()
	// Re-attribute the session to the requester: a team member may still
	// rename sessions they created themselves on the shared chat.
	store.sessions["session-1"].UserID = strPtr("user-1")

	resp, code, err := svc.UpdateSession(t.Context(), "user-1", "chat-1", "session-1", map[string]interface{}{"name": "renamed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if resp.Name == nil || *resp.Name != "renamed" {
		t.Fatalf("name=%v", resp.Name)
	}
}

func TestUpdateSession_ChatOwnerCanRenameAnySession(t *testing.T) {
	svc, store := newSharedChatReadonlyService()
	// The dialog tenant itself may manage every session of its chat.
	store.dialogExists["tenant-owner|chat-1"] = true

	_, code, err := svc.UpdateSession(t.Context(), "tenant-owner", "chat-1", "session-1", map[string]interface{}{"name": "renamed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
}

func TestDeleteSessions_SharedSessionReadonlyForTeammate(t *testing.T) {
	svc, store := newSharedChatReadonlyService()

	resp, msg, code, err := svc.DeleteSessions(t.Context(), "user-1", "chat-1", map[string]interface{}{"ids": []interface{}{"session-1"}})
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%v", code)
	}
	// The all-failed branch carries the detail in the error, not the message.
	_ = msg
	if _, stillThere := store.sessions["session-1"]; !stillThere {
		t.Fatalf("shared session should not be deleted by a team member")
	}
	if resp != nil {
		t.Fatalf("resp=%v, want nil", resp)
	}
}

func TestDeleteSessionMessage_SharedSessionReadonlyForTeammate(t *testing.T) {
	svc, _ := newSharedChatReadonlyService()

	_, code, err := svc.DeleteSessionMessage(t.Context(), "user-1", "chat-1", "session-1", "msg-1")
	if err == nil || err.Error() != "shared session is readonly" {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("code=%v", code)
	}
}

func TestUpdateMessageFeedback_SharedSessionReadonlyForTeammate(t *testing.T) {
	svc, _ := newSharedChatReadonlyService()

	_, code, err := svc.UpdateMessageFeedback(t.Context(), "user-1", "chat-1", "session-1", "msg-1", map[string]interface{}{"thumbup": true})
	if err == nil || err.Error() != "shared session is readonly" {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("code=%v", code)
	}
}

func TestChatCompletions_SharedSessionReadonlyForTeammate(t *testing.T) {
	svc, store := newSharedChatReadonlyService()
	store.dialogs["chat-1"] = &entity.Chat{
		ID:           "chat-1",
		TenantID:     "tenant-owner",
		LLMID:        "chat@factory",
		LLMSetting:   entity.JSONMap{},
		PromptConfig: entity.JSONMap{"parameters": []interface{}{}},
	}

	pipeline := &fakePipeline{resultChan: makeResultChan(AsyncChatResult{Answer: "ok", Final: true})}
	svc.pipeline = pipeline

	_, err := svc.ChatCompletions(
		t.Context(),
		"user-1",
		"chat-1",
		"session-1",
		[]map[string]interface{}{{"role": "user", "content": "hi"}},
		"", nil, "", nil, nil,
		false, false, nil,
	)
	if err == nil {
		t.Fatalf("expected readonly rejection")
	}
	coded := common.NewCodedError(0, "")
	if !errors.As(err, &coded) || coded.Code != common.CodeAuthenticationError {
		t.Fatalf("err=%v", err)
	}
	if len(store.updateCalled) != 0 {
		t.Fatalf("update calls=%d, want 0", len(store.updateCalled))
	}
}
