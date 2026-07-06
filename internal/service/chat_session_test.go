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

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
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

func (f *fakeSessionStore) GetByID(id string) (*entity.ChatSession, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	s, ok := f.sessions[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return s, nil
}

func (f *fakeSessionStore) GetBySessionIDAndChatID(sessionID, chatID string) (*entity.ChatSession, error) {
	s, err := f.GetByID(sessionID)
	if err != nil {
		return nil, err
	}
	if s.DialogID != chatID {
		return nil, gorm.ErrRecordNotFound
	}
	return s, nil
}

func (f *fakeSessionStore) Create(conv *entity.ChatSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	f.sessions[conv.ID] = conv
	f.createCalled = append(f.createCalled, conv)
	return nil
}

func (f *fakeSessionStore) UpdateByID(id string, updates map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateByIDErr != nil {
		return f.updateByIDErr
	}
	s, ok := f.sessions[id]
	if !ok {
		return gorm.ErrRecordNotFound
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

func (f *fakeSessionStore) DeleteByID(id string) error {
	delete(f.sessions, id)
	return nil
}

func (f *fakeSessionStore) ListByChatID(chatID string) ([]*entity.ChatSession, error) {
	var result []*entity.ChatSession
	for _, s := range f.sessions {
		if s.DialogID == chatID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (f *fakeSessionStore) GetDialogByID(chatID string) (*entity.Chat, error) {
	if f.getDialogErr != nil {
		return nil, f.getDialogErr
	}
	d, ok := f.dialogs[chatID]
	if !ok {
		return nil, errors.New("dialog not found")
	}
	return d, nil
}

func (f *fakeSessionStore) CheckDialogExists(tenantID, chatID string) (bool, error) {
	key := tenantID + "|" + chatID
	return f.dialogExists[key], nil
}

// ---------------------------------------------------------------------------

type fakeTenantStore struct {
	tenantIDs []string
	err       error
}

func (f *fakeTenantStore) GetTenantIDsByUserID(userID string) ([]string, error) {
	return f.tenantIDs, f.err
}

// ---------------------------------------------------------------------------

type fakePipeline struct {
	resultChan <-chan AsyncChatResult
	err        error
	userID     string
}

func (f *fakePipeline) AsyncChat(ctx context.Context, userID string, chat *entity.Chat, messages []map[string]interface{}, stream bool, kwargs map[string]interface{}) (<-chan AsyncChatResult, error) {
	f.userID = userID
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

	resp, err := svc.ListChatSessions("user-1", "chat-1")
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

	_, err := svc.ListChatSessions("user-1", "chat-1")
	if err == nil || !strings.Contains(err.Error(), "only owner") {
		t.Fatalf("expected 'only owner' error, got %v", err)
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

	resp, code, err := svc.GetSession("user-1", "chat-1", "session-1")
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

	_, code, err := svc.GetSession("user-1", "chat-1", "session-1")
	if err == nil || err.Error() != "No authorization." {
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

	_, code, err := svc.GetSession("user-1", "chat-1", "session-1")
	if err == nil || err.Error() != "Session does not belong to this chat!" {
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
	resp, code, err := svc.UpdateSession("user-1", "chat-1", "session-1", map[string]interface{}{
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
		{name: "empty body", req: map[string]interface{}{}, message: "Request body cannot be empty", code: common.CodeArgumentError},
		{name: "message", req: map[string]interface{}{"message": []interface{}{}}, message: "`messages` cannot be changed.", code: common.CodeDataError},
		{name: "messages", req: map[string]interface{}{"messages": []interface{}{}}, message: "`messages` cannot be changed.", code: common.CodeDataError},
		{name: "reference", req: map[string]interface{}{"reference": []interface{}{}}, message: "`reference` cannot be changed.", code: common.CodeDataError},
		{name: "empty name", req: map[string]interface{}{"name": "   "}, message: "`name` can not be empty.", code: common.CodeDataError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, code, err := svc.UpdateSession("user-1", "chat-1", "session-1", tc.req)
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

	_, code, err := svc.UpdateSession("user-1", "chat-1", "missing", map[string]interface{}{"name": "renamed"})
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

	resp, code, err := svc.DeleteSessionMessage("user-1", "chat-1", "session-1", "msg-1")
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
	ctx := context.WithValue(context.Background(), feedbackContextKey{}, "request-context")

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

	resp, code, err := svc.UpdateMessageFeedback(context.Background(), "user-1", "chat-1", "session-1", "msg-1", map[string]interface{}{
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

	result, err := svc.applyChunkFeedback(context.Background(), "tenant-1", map[string]interface{}{
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

	result, err := svc.applyChunkFeedback(context.Background(), "tenant-1", map[string]interface{}{
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

	result, err := svc.applyChunkFeedback(context.Background(), "tenant-1", map[string]interface{}{
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

	if ok := svc.updateChunkWeight(context.Background(), "tenant-1", "chunk-1", "kb-1", 0.25); !ok {
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

	result, err := svc.applyChunkFeedback(context.Background(), "tenant-1", map[string]interface{}{
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

func TestCompletion_Success(t *testing.T) {
	store := newFakeSessionStore()
	session := &entity.ChatSession{
		ID: "session-1", DialogID: "dialog-1",
		Message:   json.RawMessage(`[{"role":"assistant","content":"Welcome!"}]`),
		Reference: json.RawMessage(`[]`),
	}
	store.sessions["session-1"] = session
	store.dialogs["dialog-1"] = &entity.Chat{
		ID: "dialog-1", TenantID: "tenant-1", LLMID: "chat@factory",
		LLMSetting: entity.JSONMap{},
	}

	pipeline := &fakePipeline{
		resultChan: makeResultChan(
			AsyncChatResult{Answer: "Hello", Reference: map[string]interface{}{"chunks": []interface{}{}}},
			AsyncChatResult{Answer: " world", Final: true, Reference: map[string]interface{}{"chunks": []interface{}{}}},
		),
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       pipeline,
	}

	result, err := svc.Completion("user-1", "session-1", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1")
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
}

func TestChatCompletionsPassesRequestUserIDToPipeline(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID:        "session-1",
		DialogID:  "dialog-1",
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
		context.Background(),
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

func TestCompletion_EmptyMessages(t *testing.T) {
	svc := &ChatSessionService{
		chatSessionDAO: &fakeSessionStore{},
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, err := svc.Completion("user-1", "session-1", nil, "", nil, "msg-1")
	if err == nil || err.Error() != "messages cannot be empty" {
		t.Fatalf("expected 'messages cannot be empty', got %v", err)
	}
}

func TestCompletion_LastMessageNotFromUser(t *testing.T) {
	svc := &ChatSessionService{
		chatSessionDAO: &fakeSessionStore{},
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, err := svc.Completion("user-1", "session-1", []map[string]interface{}{
		{"role": "assistant", "content": "hello"},
	}, "", nil, "msg-1")
	if err == nil || !strings.Contains(err.Error(), "not from user") {
		t.Fatalf("expected 'not from user' error, got %v", err)
	}
}

func TestCompletion_ConversationNotFound(t *testing.T) {
	store := newFakeSessionStore()

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, err := svc.Completion("user-1", "missing", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1")
	if err == nil || err.Error() != "Conversation not found" {
		t.Fatalf("expected 'Conversation not found', got %v", err)
	}
}

func TestCompletion_DialogNotFound(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID: "session-1", DialogID: "dialog-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, err := svc.Completion("user-1", "session-1", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1")
	if err == nil || err.Error() != "Dialog not found" {
		t.Fatalf("expected 'Dialog not found', got %v", err)
	}
}

func TestCompletion_PipelineError(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID: "session-1", DialogID: "dialog-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}
	store.dialogs["dialog-1"] = &entity.Chat{
		ID: "dialog-1", TenantID: "tenant-1", LLMID: "chat@factory",
		LLMSetting: entity.JSONMap{},
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{err: errors.New("model unavailable")},
	}

	_, err := svc.Completion("user-1", "session-1", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1")
	if err == nil || err.Error() != "model unavailable" {
		t.Fatalf("expected 'model unavailable' error, got %v", err)
	}
}

// ===================================================================
// CompletionStream tests
// ===================================================================

func readStreamChan(ch <-chan string, n int) []string {
	var msgs []string
	for i := 0; i < n; i++ {
		select {
		case msg, ok := <-ch:
			if !ok {
				return msgs
			}
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
	return msgs
}

func TestCompletionStream_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID: "session-1", DialogID: "dialog-1",
		Message:   json.RawMessage(`{"messages":[{"role":"assistant","content":"Welcome!"}]}`),
		Reference: json.RawMessage(`[]`),
	}
	store.dialogs["dialog-1"] = &entity.Chat{
		ID: "dialog-1", TenantID: "tenant-1", LLMID: "chat@factory",
		LLMSetting: entity.JSONMap{},
	}

	pipeline := &fakePipeline{
		resultChan: makeResultChan(
			AsyncChatResult{Answer: "stream", Reference: map[string]interface{}{"chunks": []interface{}{}}},
			AsyncChatResult{Answer: " answer", Reference: map[string]interface{}{"chunks": []interface{}{}}},
		),
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       pipeline,
	}

	streamChan := make(chan string, 10)
	err := svc.CompletionStream(context.Background(), "user-1", "session-1", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1", streamChan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should receive data events and final signal
	msgs := readStreamChan(streamChan, 5)
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 stream messages, got %d: %v", len(msgs), msgs)
	}
	// Check final signal
	finalFound := false
	for _, m := range msgs {
		if strings.Contains(m, `"data":true`) {
			finalFound = true
			break
		}
	}
	if !finalFound {
		t.Fatal("expected final=true signal in stream")
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
	if got[2]["role"] != "assistant" || got[2]["content"] != "stream answer" || got[2]["id"] != "msg-1" {
		t.Fatalf("stored assistant message=%#v", got[2])
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
