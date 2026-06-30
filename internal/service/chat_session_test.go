package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/common"
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
	deleteByIDErr error
	getDialogErr  error
	// record calls
	createCalled []*entity.ChatSession
	updateCalled []struct {
		id      string
		updates map[string]interface{}
	}
	deleteByIDIDs []string
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
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteByIDErr != nil {
		return f.deleteByIDErr
	}
	f.deleteByIDIDs = append(f.deleteByIDIDs, id)
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
}

func (f *fakePipeline) AsyncChat(ctx context.Context, chat *entity.Chat, messages []map[string]interface{}, stream bool, kwargs map[string]interface{}) (<-chan AsyncChatResult, error) {
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

// ===================================================================
// SetChatSession tests
// ===================================================================

func TestSetChatSession_CreateNew(t *testing.T) {
	store := newFakeSessionStore()
	dialog := &entity.Chat{ID: "dialog-1", PromptConfig: entity.JSONMap{"prologue": "Welcome!"}}
	store.dialogs["dialog-1"] = dialog

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	resp, err := svc.SetChatSession("user-1", &SetChatSessionRequest{
		DialogID: "dialog-1",
		IsNew:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("expected session ID to be generated")
	}
	if resp.DialogID != "dialog-1" {
		t.Fatalf("expected dialog_id=dialog-1, got %s", resp.DialogID)
	}
	if len(store.createCalled) != 1 {
		t.Fatalf("expected 1 Create call, got %d", len(store.createCalled))
	}

	// Verify prologue is in the message list.
	var msgs []map[string]interface{}
	if err := json.Unmarshal(store.createCalled[0].Message, &msgs); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 initial message, got %d", len(msgs))
	}
	firstMsg := msgs[0]
	if firstMsg["role"] != "assistant" || firstMsg["content"] != "Welcome!" {
		t.Fatalf("unexpected prologue message: %#v", firstMsg)
	}
}

func TestSetChatSession_CreateNewDefaultPrologue(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogs["dialog-1"] = &entity.Chat{ID: "dialog-1"}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	resp, err := svc.SetChatSession("user-1", &SetChatSessionRequest{
		DialogID: "dialog-1",
		IsNew:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("expected session ID")
	}
	// Default prologue
	var msgs []map[string]interface{}
	json.Unmarshal(store.createCalled[0].Message, &msgs)
	firstMsg := msgs[0]
	if !strings.Contains(firstMsg["content"].(string), "Hi! I'm your assistant") {
		t.Fatalf("expected default prologue, got %q", firstMsg["content"])
	}
}

func TestSetChatSession_CreateNewDialogNotFound(t *testing.T) {
	store := newFakeSessionStore()

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, err := svc.SetChatSession("user-1", &SetChatSessionRequest{
		DialogID: "nonexistent",
		IsNew:    true,
	})
	if err == nil || err.Error() != "Dialog not found" {
		t.Fatalf("expected 'Dialog not found' error, got %v", err)
	}
}

func TestSetChatSession_UpdateExisting(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["session-1"] = &entity.ChatSession{
		ID: "session-1", DialogID: "dialog-1", Name: strPtr("old name"),
	}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	resp, err := svc.SetChatSession("user-1", &SetChatSessionRequest{
		SessionID: "session-1",
		Name:      "new name",
		IsNew:     false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "session-1" {
		t.Fatalf("expected session-1, got %s", resp.ID)
	}
	if len(store.updateCalled) != 1 {
		t.Fatalf("expected UpdateByID call, got %d", len(store.updateCalled))
	}
}

func TestSetChatSession_UpdateNotFound(t *testing.T) {
	store := newFakeSessionStore()
	store.updateByIDErr = errors.New("Chat session not found")

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, err := svc.SetChatSession("user-1", &SetChatSessionRequest{
		SessionID: "missing",
		IsNew:     false,
	})
	if err == nil || err.Error() != "Chat session not found" {
		t.Fatalf("expected 'Chat session not found' error, got %v", err)
	}
}

func TestSetChatSession_NameTruncation(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogs["dialog-1"] = &entity.Chat{ID: "dialog-1"}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	longName := strings.Repeat("x", 300)
	resp, err := svc.SetChatSession("user-1", &SetChatSessionRequest{
		DialogID: "dialog-1",
		Name:     longName,
		IsNew:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name == nil || len(*resp.Name) > 255 {
		t.Fatalf("expected name truncated to <=255, got len=%d", len(*resp.Name))
	}
}

// ===================================================================
// RemoveChatSessions tests
// ===================================================================

func TestRemoveChatSessions_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["conv-1"] = &entity.ChatSession{ID: "conv-1", DialogID: "dialog-1"}
	store.sessions["conv-2"] = &entity.ChatSession{ID: "conv-2", DialogID: "dialog-1"}
	store.dialogExists["user-1|dialog-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-1"}},
		pipeline:       &fakePipeline{},
	}

	err := svc.RemoveChatSessions("user-1", []string{"conv-1", "conv-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deleteByIDIDs) != 2 {
		t.Fatalf("expected 2 deletes, got %d", len(store.deleteByIDIDs))
	}
}

func TestRemoveChatSessions_SessionNotFound(t *testing.T) {
	store := newFakeSessionStore()
	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-1"}},
		pipeline:       &fakePipeline{},
	}

	err := svc.RemoveChatSessions("user-1", []string{"missing"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

func TestRemoveChatSessions_NotOwner(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions["conv-1"] = &entity.ChatSession{ID: "conv-1", DialogID: "dialog-1"}
	// No tenant matches — dialogExists stays false for all combinations

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{tenantIDs: []string{"tenant-other"}},
		pipeline:       &fakePipeline{},
	}

	err := svc.RemoveChatSessions("user-1", []string{"conv-1"})
	if err == nil || !strings.Contains(err.Error(), "Only owner") {
		t.Fatalf("expected 'Only owner' error, got %v", err)
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

func TestCompletionStream_EmptyMessages(t *testing.T) {
	svc := &ChatSessionService{
		chatSessionDAO: &fakeSessionStore{},
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	streamChan := make(chan string, 10)
	err := svc.CompletionStream(context.Background(), "user-1", "session-1", nil, "", nil, "msg-1", streamChan)
	if err == nil || err.Error() != "messages cannot be empty" {
		t.Fatalf("expected 'messages cannot be empty', got %v", err)
	}
}

func TestCompletionStream_LastMessageNotFromUser(t *testing.T) {
	svc := &ChatSessionService{
		chatSessionDAO: &fakeSessionStore{},
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	streamChan := make(chan string, 10)
	err := svc.CompletionStream(context.Background(), "user-1", "session-1", []map[string]interface{}{
		{"role": "assistant", "content": "hello"},
	}, "", nil, "msg-1", streamChan)
	if err == nil || !strings.Contains(err.Error(), "not from user") {
		t.Fatalf("expected 'not from user' error, got %v", err)
	}
}

func TestCompletionStream_ConversationNotFound(t *testing.T) {
	store := newFakeSessionStore()
	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	streamChan := make(chan string, 10)
	err := svc.CompletionStream(context.Background(), "user-1", "missing", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1", streamChan)
	if err == nil || err.Error() != "Conversation not found" {
		t.Fatalf("expected 'Conversation not found', got %v", err)
	}
}

func TestCompletionStream_DialogNotFound(t *testing.T) {
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

	streamChan := make(chan string, 10)
	err := svc.CompletionStream(context.Background(), "user-1", "session-1", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1", streamChan)
	if err == nil || err.Error() != "Dialog not found" {
		t.Fatalf("expected 'Dialog not found', got %v", err)
	}
}

func TestCompletionStream_PipelineError(t *testing.T) {
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

	streamChan := make(chan string, 10)
	err := svc.CompletionStream(context.Background(), "user-1", "session-1", []map[string]interface{}{
		{"role": "user", "content": "hi"},
	}, "", nil, "msg-1", streamChan)
	if err == nil || err.Error() != "model unavailable" {
		t.Fatalf("expected 'model unavailable' error, got %v", err)
	}
}
