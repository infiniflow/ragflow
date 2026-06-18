//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestListVersions_Success verifies that ListVersions returns all versions
// for a canvas, ordered by update_time DESC.
func TestListVersions_Success(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	// Migrate tables needed for agent versions
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	now := time.Now()

	// Insert canvas owner
	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "owner@test.com"})

	// Insert canvas
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})

	// Insert 3 versions with staggered timestamps
	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v1",
		UserCanvasID: "canvas-1",
		Title:        sptr("v1_oldest"),
		BaseModel: entity.BaseModel{
			CreateTime: ptr(now.Add(-2 * time.Hour).UnixMilli()),
			UpdateTime: ptr(now.Add(-2 * time.Hour).UnixMilli()),
		},
	})
	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v2",
		UserCanvasID: "canvas-1",
		Title:        sptr("v2_middle"),
		BaseModel: entity.BaseModel{
			CreateTime: ptr(now.Add(-1 * time.Hour).UnixMilli()),
			UpdateTime: ptr(now.Add(-1 * time.Hour).UnixMilli()),
		},
	})
	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v3",
		UserCanvasID: "canvas-1",
		Title:        sptr("v3_newest"),
		BaseModel: entity.BaseModel{
			CreateTime: ptr(now.UnixMilli()),
			UpdateTime: ptr(now.UnixMilli()),
		},
	})

	svc := NewAgentService()
	versions, err := svc.ListVersions(context.Background(), "user-1", "canvas-1")
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// Verify DESC order
	if *versions[0].Title != "v3_newest" {
		t.Errorf("expected v3_newest first, got %s", *versions[0].Title)
	}
	if *versions[1].Title != "v2_middle" {
		t.Errorf("expected v2_middle second, got %s", *versions[1].Title)
	}
	if *versions[2].Title != "v1_oldest" {
		t.Errorf("expected v1_oldest last, got %s", *versions[2].Title)
	}
}

// TestListVersions_Empty verifies that ListVersions returns an empty slice
// when no versions exist.
func TestListVersions_Empty(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	// Insert canvas with no versions
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-empty",
		UserID: "user-1",
		Title:  sptr("Empty Agent"),
	})

	svc := NewAgentService()
	versions, err := svc.ListVersions(context.Background(), "user-1", "canvas-empty")
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

// TestGetVersion_Success verifies getting a specific version by ID.
func TestGetVersion_Success(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})

	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v1",
		UserCanvasID: "canvas-1",
		Title:        sptr("version-1"),
		DSL:          entity.JSONMap{"model": "gpt-4"},
	})

	svc := NewAgentService()
	v, err := svc.GetVersion(context.Background(), "user-1", "canvas-1", "v1")
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if *v.Title != "version-1" {
		t.Errorf("expected title 'version-1', got %s", *v.Title)
	}
}

// TestGetVersion_WrongCanvas verifies version belonging to another canvas returns error.
func TestGetVersion_WrongCanvas(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v1",
		UserCanvasID: "canvas-1",
		Title:        sptr("version-1"),
	})

	svc := NewAgentService()
	_, err := svc.GetVersion(context.Background(), "user-other", "canvas-other", "v1")
	if err == nil {
		t.Error("expected error for version belonging to another canvas")
	}
}

// TestGetVersion_NotFound verifies error for non-existent version.
func TestGetVersion_NotFound(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	svc := NewAgentService()
	_, err := svc.GetVersion(context.Background(), "user-1", "canvas-1", "non-existent")
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

// TestRunAgent_VersionBelongsToOtherCanvas pins the v3.5.2 IDOR
// fix: when the caller supplies an explicit version id, RunAgent
// must verify that the row belongs to the canvas being run, not
// to some other canvas whose id the caller happens to know. The
// previous code did `versionRow, _ = s.versionDAO.GetByID(version)`
// and used whatever row came back, letting any caller run any
// canvas's DSL against their own canvas.
//
// We set up two canvases owned by the same user (user-1 owns
// canvas-1 and canvas-2), create a version v1 on canvas-2, and
// try to run canvas-1 with version=v1. RunAgent must refuse with
// dao.ErrUserCanvasVersionNotFound.
func TestRunAgent_VersionBelongsToOtherCanvas(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "owner@test.com"})
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Canvas 1"),
	})
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-2",
		UserID: "user-1",
		Title:  sptr("Canvas 2"),
	})
	// Version row that belongs to canvas-2, NOT canvas-1.
	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v-on-canvas-2",
		UserCanvasID: "canvas-2",
		Title:        sptr("foreign version"),
		DSL:          entity.JSONMap{"components": map[string]any{}},
	})

	svc := NewAgentService()
	_, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-1",      // we're running canvas-1…
		"",              // session ID auto-generated
		"v-on-canvas-2", // …with a version that belongs to canvas-2
		"hi",
	)
	if err == nil {
		t.Fatal("expected error when version belongs to a different canvas (IDOR guard)")
	}
	if !errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
		t.Errorf("expected ErrUserCanvasVersionNotFound, got %v", err)
	}
}

// TestRunAgent_VersionNotFound pins the v3.5.2 DAO-error
// visibility fix: when the caller supplies an explicit version id
// and the row does not exist, RunAgent must return the DAO error
// (not swallow it as the V1 placeholder). The previous code
// silenced the error and proceeded with a "no version published"
// placeholder answer, hiding real storage/DAO failures from the
// caller.
func TestRunAgent_VersionNotFound(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "owner@test.com"})
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Canvas 1"),
	})

	svc := NewAgentService()
	_, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-1",
		"",
		"does-not-exist",
		"hi",
	)
	if err == nil {
		t.Fatal("expected error when explicit version id does not exist")
	}
	if !errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
		t.Errorf("expected ErrUserCanvasVersionNotFound, got %v", err)
	}
}

// TestRunAgent_NoVersionPublishedPlaceholder pins the legitimate
// "no version published" branch: when version is empty AND
// GetLatest returns ErrUserCanvasVersionNotFound (no rows), the
// run proceeds with the V1 echo placeholder answer rather than
// failing the whole RunAgent call. This is intentional — the SSE
// surface still flows and the caller sees a "no published version"
// message.
//
// (Distinct from TestRunAgent_VersionNotFound above, which tests
// the explicit-version case where the error must surface.)
//
// v3.5.2 hardening: the v3.5.2 review noted the prior version of
// this test only asserted "non-nil channel, no immediate error"
// — a spec that would pass for many wrong implementations
// (channel with only a done event, channel with malformed events,
// etc.). The hardened test now drains the channel synchronously
// and asserts at least one MessageEvent carries an Answer whose
// payload contains the canonical placeholder text.
func TestRunAgent_NoVersionPublishedPlaceholder(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "owner@test.com"})
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-empty",
		UserID: "user-1",
		Title:  sptr("Canvas With No Version"),
	})

	svc := NewAgentService()
	events, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-empty",
		"test-session",
		"", // no explicit version → use GetLatest, which returns ErrUserCanvasVersionNotFound
		"hi",
	)
	if err != nil {
		t.Fatalf("RunAgent should proceed with placeholder when no version published: %v", err)
	}
	if events == nil {
		t.Fatal("RunAgent returned nil event channel for legitimate 'no version published'")
	}

	// Drain the channel synchronously and assert the placeholder
	// answer text is present. The driver emits at least one
	// orchestrator (canvas.Runner) RunEvent with Type=="message" whose Data is a
	// JSON-encoded MessageEvent with the placeholder Answer, plus
	// a terminator RunEvent with Type=="done".
	var (
		gotAnswer       string
		gotMessageEvent bool
		gotDoneEvent    bool
	)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if !gotMessageEvent {
					t.Fatal("placeholder channel closed before any MessageEvent was received")
				}
				if gotAnswer == "" {
					t.Fatal("placeholder MessageEvent had empty Answer")
				}
				if !strings.Contains(gotAnswer, "canvas-empty") {
					t.Errorf("placeholder answer %q does not mention canvas ID", gotAnswer)
				}
				if !strings.Contains(gotAnswer, "No published version") {
					t.Errorf("placeholder answer %q does not contain 'No published version'", gotAnswer)
				}
				if !gotDoneEvent {
					t.Error("placeholder channel closed without emitting a DoneEvent")
				}
				return
			}
			switch ev.Type {
			case "message":
				gotMessageEvent = true
				var msg canvas.MessageEvent
				if err := json.Unmarshal([]byte(ev.Data), &msg); err != nil {
					t.Fatalf("message RunEvent had un-decodable Data %q: %v", ev.Data, err)
				}
				gotAnswer = msg.Answer
			case "done":
				gotDoneEvent = true
			}
		case <-deadline:
			t.Fatal("placeholder channel did not close within 5s — driver deadlocked?")
		}
	}
}

// TestRunAgent_StorageErrorFromCanvasAccess pins the v3.5.2
// follow-up: when loadCanvasForUser's underlying DAO surfaces a
// real DB error (NOT the ErrUserCanvasNotFound sentinel), RunAgent
// must return an error wrapped with ErrAgentStorageError so the
// handler maps it to CodeServerError (500) with a sanitized
// message — the raw DAO string (DSN, table name, etc.) MUST NOT
// reach the client.
//
// We simulate a real DB failure by closing the underlying sql.DB
// connection mid-test. The next query against the test SQLite DB
// returns a "sql: database is closed" error, which is the
// closest thing to a real production DB outage we can stage
// without monkey-patching the DAO.
//
// This complements TestMapAgentError's wrapped-storage case (which
// pins the HTTP-layer mapping) by pinning the service-layer
// wrapping contract itself.
func TestRunAgent_StorageErrorFromCanvasAccess(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "owner@test.com"})
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Canvas"),
	})

	// Close the underlying sql.DB to make the next DAO query fail
	// with a real storage error. We restore dao.DB after the test
	// via t.Cleanup above so the close doesn't leak across tests.
	sqlDB, sErr := testDB.DB()
	if sErr != nil {
		t.Fatalf("failed to obtain sql.DB: %v", sErr)
	}
	if cErr := sqlDB.Close(); cErr != nil {
		t.Fatalf("failed to close sql.DB: %v", cErr)
	}

	svc := NewAgentService()
	_, err := svc.RunAgent(
		context.Background(),
		"user-1",
		"canvas-1",
		"",
		"",
		"hi",
	)
	if err == nil {
		t.Fatal("expected storage error from closed DB")
	}
	if !errors.Is(err, ErrAgentStorageError) {
		t.Errorf("expected ErrAgentStorageError in chain, got %v", err)
	}
	// The raw "sql: database is closed" / gorm error text must
	// not be in the user-facing message — the handler's
	// mapAgentError returns a sanitized message, but we pin the
	// contract at the service layer too: the error wraps the
	// sentinel, and a wrapping chain that includes
	// ErrAgentStorageError means the handler will sanitize.
}

// TestLoadCanvasForUser_StorageErrorWrap pins the same wrapping
// contract at the loadCanvasForUser level (not just through
// RunAgent). loadCanvasForUser is shared by GetAgent, UpdateAgent,
// DeleteAgent, PublishAgent, ListVersions, GetVersion, and
// CancelAgent — sanitising its DAO errors closes the leak in all
// eight call sites.
func TestLoadCanvasForUser_StorageErrorWrap(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	// Close the underlying sql.DB before any query — this is the
	// cleanest way to force a non-sentinel DAO error out of
	// userTenantDAO.GetTenantIDsByUserID.
	sqlDB, sErr := testDB.DB()
	if sErr != nil {
		t.Fatalf("failed to obtain sql.DB: %v", sErr)
	}
	if cErr := sqlDB.Close(); cErr != nil {
		t.Fatalf("failed to close sql.DB: %v", cErr)
	}

	svc := NewAgentService()
	_, err := svc.loadCanvasForUser(context.Background(), "user-1", "canvas-1")
	if err == nil {
		t.Fatal("expected storage error from closed DB")
	}
	if !errors.Is(err, ErrAgentStorageError) {
		t.Errorf("expected ErrAgentStorageError in chain, got %v", err)
	}
	// Also assert the sentinel ErrUserCanvasNotFound did NOT
	// leak — it must not appear in the chain because the error
	// is a real storage failure, not a permission miss.
	if errors.Is(err, dao.ErrUserCanvasNotFound) {
		t.Errorf("storage error chain should not include ErrUserCanvasNotFound; got %v", err)
	}
}

func setupAgentSessionServiceTest(t *testing.T) {
	t.Helper()

	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserTenant{},
		&entity.API4Conversation{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })
}

func createAgentSessionTestCanvas(t *testing.T, id, userID string) {
	t.Helper()
	if err := dao.DB.Create(&entity.UserCanvas{
		ID:             id,
		UserID:         userID,
		Title:          sptr("Test Agent"),
		CanvasCategory: "agent_canvas",
	}).Error; err != nil {
		t.Fatalf("failed to create canvas %s: %v", id, err)
	}
}

func createAgentSessionTestConversation(t *testing.T, id, agentID, userID string, updateTime int64) {
	t.Helper()
	updateDate := time.UnixMilli(updateTime)
	if err := dao.DB.Create(&entity.API4Conversation{
		ID:        id,
		DialogID:  agentID,
		UserID:    userID,
		Message:   json.RawMessage(`[{"role":"assistant","content":"hello","prompt":"hidden"}]`),
		Reference: json.RawMessage(`[]`),
		BaseModel: entity.BaseModel{
			CreateTime: ptr(updateTime),
			CreateDate: &updateDate,
			UpdateTime: ptr(updateTime),
			UpdateDate: &updateDate,
		},
	}).Error; err != nil {
		t.Fatalf("failed to create session %s: %v", id, err)
	}
}

func TestListAgentSessionsServiceSuccess(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")
	createAgentSessionTestConversation(t, "session-old", "canvas-1", "user-1", 1000)
	createAgentSessionTestConversation(t, "session-new", "canvas-1", "user-1", 3000)
	createAgentSessionTestConversation(t, "session-other-agent", "canvas-other", "user-1", 9999)

	resp, code, err := NewAgentService().ListAgentSessions("user-1", "user-1", "canvas-1", ListAgentSessionsRequest{
		Page:       1,
		PageSize:   10,
		OrderBy:    "update_time",
		Desc:       true,
		IncludeDSL: false,
	})
	if err != nil {
		t.Fatalf("ListAgentSessions failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %d, got %d", common.CodeSuccess, code)
	}
	if resp.Total != 2 {
		t.Fatalf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Data))
	}
	if resp.Data[0]["id"] != "session-new" {
		t.Fatalf("expected newest session first, got %v", resp.Data[0]["id"])
	}
	if resp.Data[0]["agent_id"] != "canvas-1" {
		t.Fatalf("expected agent_id canvas-1, got %v", resp.Data[0]["agent_id"])
	}
	messages, ok := resp.Data[0]["message"].([]map[string]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("expected normalized message slice, got %T %v", resp.Data[0]["message"], resp.Data[0]["message"])
	}
	if _, ok := messages[0]["prompt"]; ok {
		t.Fatal("expected prompt to be removed from normalized session message")
	}
}

func TestListAgentSessionsServiceDenied(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-2")

	resp, code, err := NewAgentService().ListAgentSessions("user-1", "user-1", "canvas-1", ListAgentSessionsRequest{})
	if err == nil {
		t.Fatal("expected permission error")
	}
	if code != common.CodeOperatingError {
		t.Fatalf("expected code %d, got %d", common.CodeOperatingError, code)
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
}

func TestGetAgentSessionServiceSuccess(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")
	createAgentSessionTestConversation(t, "session-1", "canvas-1", "user-1", 1000)

	session, code, err := NewAgentService().GetAgentSession("user-1", "canvas-1", "session-1")
	if err != nil {
		t.Fatalf("GetAgentSession failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %d, got %d", common.CodeSuccess, code)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.ID != "session-1" {
		t.Fatalf("expected session-1, got %s", session.ID)
	}
	if session.DialogID != "canvas-1" {
		t.Fatalf("expected dialog_id canvas-1, got %s", session.DialogID)
	}
}

func TestGetAgentSessionServiceNotFoundWhenSessionBelongsToAnotherAgent(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")
	createAgentSessionTestConversation(t, "session-other", "canvas-other", "user-1", 1000)

	session, code, err := NewAgentService().GetAgentSession("user-1", "canvas-1", "session-other")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if code != common.CodeNotFound {
		t.Fatalf("expected code %d, got %d", common.CodeNotFound, code)
	}
	if session != nil {
		t.Fatalf("expected nil session, got %+v", session)
	}
}

func TestDeleteAgentSessionItemServiceDeletesMatchingSession(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")
	createAgentSessionTestConversation(t, "session-1", "canvas-1", "user-1", 1000)
	createAgentSessionTestConversation(t, "session-other", "canvas-other", "user-1", 2000)

	deleted, code, err := NewAgentService().DeleteAgentSessionItem("user-1", "canvas-1", "session-1")
	if err != nil {
		t.Fatalf("DeleteAgentSessionItem failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %d, got %d", common.CodeSuccess, code)
	}
	if !deleted {
		t.Fatal("expected session to be deleted")
	}

	var count int64
	if err := dao.DB.Model(&entity.API4Conversation{}).Where("id = ?", "session-1").Count(&count).Error; err != nil {
		t.Fatalf("failed to count deleted session: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected session-1 to be deleted, count=%d", count)
	}

	if err := dao.DB.Model(&entity.API4Conversation{}).Where("id = ?", "session-other").Count(&count).Error; err != nil {
		t.Fatalf("failed to count other session: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected other agent session to remain, count=%d", count)
	}
}

func TestDeleteAgentSessionItemServiceNoopForSessionFromAnotherAgent(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")
	createAgentSessionTestConversation(t, "session-other", "canvas-other", "user-1", 1000)

	deleted, code, err := NewAgentService().DeleteAgentSessionItem("user-1", "canvas-1", "session-other")
	if err != nil {
		t.Fatalf("DeleteAgentSessionItem failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %d, got %d", common.CodeSuccess, code)
	}
	if deleted {
		t.Fatal("expected cross-agent session delete to be a noop")
	}

	var count int64
	if err := dao.DB.Model(&entity.API4Conversation{}).Where("id = ?", "session-other").Count(&count).Error; err != nil {
		t.Fatalf("failed to count other session: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected cross-agent session to remain, count=%d", count)
	}
}

func TestDeleteAgentSessionsServiceDeleteAll(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")
	createAgentSessionTestConversation(t, "session-1", "canvas-1", "user-1", 1000)
	createAgentSessionTestConversation(t, "session-2", "canvas-1", "user-1", 2000)
	createAgentSessionTestConversation(t, "session-other", "canvas-other", "user-1", 3000)

	result, code, err := NewAgentService().DeleteAgentSessions("user-1", "canvas-1", nil, true)
	if err != nil {
		t.Fatalf("DeleteAgentSessions failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %d, got %d", common.CodeSuccess, code)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Data != nil {
		t.Fatalf("expected no partial data on full success, got %+v", result.Data)
	}

	var ownCount int64
	if err := dao.DB.Model(&entity.API4Conversation{}).Where("dialog_id = ?", "canvas-1").Count(&ownCount).Error; err != nil {
		t.Fatalf("failed to count own sessions: %v", err)
	}
	if ownCount != 0 {
		t.Fatalf("expected all canvas-1 sessions to be deleted, count=%d", ownCount)
	}

	var otherCount int64
	if err := dao.DB.Model(&entity.API4Conversation{}).Where("id = ?", "session-other").Count(&otherCount).Error; err != nil {
		t.Fatalf("failed to count other session: %v", err)
	}
	if otherCount != 1 {
		t.Fatalf("expected other agent session to remain, count=%d", otherCount)
	}
}

func TestDeleteAgentSessionsServiceDuplicateIDsPartial(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")
	createAgentSessionTestConversation(t, "session-1", "canvas-1", "user-1", 1000)

	result, code, err := NewAgentService().DeleteAgentSessions("user-1", "canvas-1", []string{"session-1", "session-1"}, false)
	if err != nil {
		t.Fatalf("DeleteAgentSessions failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %d, got %d", common.CodeSuccess, code)
	}
	if result == nil || result.Data == nil {
		t.Fatalf("expected partial result data, got %+v", result)
	}
	if result.Data.SuccessCount != 1 {
		t.Fatalf("expected success_count 1, got %d", result.Data.SuccessCount)
	}
	if len(result.Data.Errors) != 1 || result.Data.Errors[0] != "Duplicate session ids: session-1" {
		t.Fatalf("unexpected duplicate errors: %v", result.Data.Errors)
	}

	var count int64
	if err := dao.DB.Model(&entity.API4Conversation{}).Where("id = ?", "session-1").Count(&count).Error; err != nil {
		t.Fatalf("failed to count deleted session: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected session-1 to be deleted, count=%d", count)
	}
}

func TestDeleteAgentSessionsServiceMissingSessionError(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")

	result, code, err := NewAgentService().DeleteAgentSessions("user-1", "canvas-1", []string{"missing-session"}, false)
	if err == nil {
		t.Fatal("expected missing session error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected code %d, got %d", common.CodeDataError, code)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
	if !strings.Contains(err.Error(), "The agent doesn't own the session missing-session") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteAgentSessionsServiceRequiresOwner(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-2")
	createAgentSessionTestConversation(t, "session-1", "canvas-1", "user-1", 1000)

	result, code, err := NewAgentService().DeleteAgentSessions("user-1", "canvas-1", []string{"session-1"}, false)
	if err == nil {
		t.Fatal("expected owner error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected code %d, got %d", common.CodeDataError, code)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}

	var count int64
	if err := dao.DB.Model(&entity.API4Conversation{}).Where("id = ?", "session-1").Count(&count).Error; err != nil {
		t.Fatalf("failed to count session: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected session to remain, count=%d", count)
	}
}

func TestUpdateAgentTagsServiceSuccess(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")

	ok, code, err := NewAgentService().UpdateAgentTags("user-1", "canvas-1", []interface{}{"alpha", "beta", "alpha", "with,comma"})
	if err != nil {
		t.Fatalf("UpdateAgentTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %d, got %d", common.CodeSuccess, code)
	}
	if !ok {
		t.Fatal("expected update to succeed")
	}

	canvas, err := dao.NewUserCanvasDAO().GetByID("canvas-1")
	if err != nil {
		t.Fatalf("failed to get canvas: %v", err)
	}
	if canvas.Tags != "alpha,beta,with comma" {
		t.Fatalf("expected normalized tags, got %q", canvas.Tags)
	}
}

func TestUpdateAgentTagsServiceInvalidPayload(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-1")

	ok, code, err := NewAgentService().UpdateAgentTags("user-1", "canvas-1", map[string]string{"tag": "alpha"})
	if err == nil {
		t.Fatal("expected invalid tags error")
	}
	if code != common.CodeBadRequest {
		t.Fatalf("expected code %d, got %d", common.CodeBadRequest, code)
	}
	if ok {
		t.Fatal("expected update to fail")
	}

	canvas, err := dao.NewUserCanvasDAO().GetByID("canvas-1")
	if err != nil {
		t.Fatalf("failed to get canvas: %v", err)
	}
	if canvas.Tags != "" {
		t.Fatalf("expected tags to remain unchanged, got %q", canvas.Tags)
	}
}

func TestUpdateAgentTagsServiceNoPermission(t *testing.T) {
	setupAgentSessionServiceTest(t)

	createAgentSessionTestCanvas(t, "canvas-1", "user-2")

	ok, code, err := NewAgentService().UpdateAgentTags("user-1", "canvas-1", []string{"alpha"})
	if err == nil {
		t.Fatal("expected permission error")
	}
	if code != common.CodeOperatingError {
		t.Fatalf("expected code %d, got %d", common.CodeOperatingError, code)
	}
	if ok {
		t.Fatal("expected update to fail")
	}

	canvas, err := dao.NewUserCanvasDAO().GetByID("canvas-1")
	if err != nil {
		t.Fatalf("failed to get canvas: %v", err)
	}
	if canvas.Tags != "" {
		t.Fatalf("expected tags to remain unchanged, got %q", canvas.Tags)
	}
}

func TestIsPublicAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{name: "public IPv4", addr: "8.8.8.8", want: true},
		{name: "loopback", addr: "127.0.0.1", want: false},
		{name: "private", addr: "192.168.1.1", want: false},
		{name: "carrier NAT", addr: "100.64.0.1", want: false},
		{name: "documentation", addr: "203.0.113.1", want: false},
		{name: "IPv4 mapped loopback", addr: "::ffff:127.0.0.1", want: false},
		{name: "IPv6 documentation", addr: "2001:db8::1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPublicAddr(netip.MustParseAddr(tt.addr))
			if got != tt.want {
				t.Fatalf("isPublicAddr(%s): expected %v, got %v", tt.addr, tt.want, got)
			}
		})
	}
}

func TestAssertHostIsSafeRejectsLocalhost(t *testing.T) {
	_, err := AssertHostIsSafe("localhost")
	if err == nil {
		t.Fatal("expected localhost to be rejected")
	}
	if !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected non-public address error, got %v", err)
	}
}

func TestTestDBConnectionMissingFields(t *testing.T) {
	code, err := NewAgentService().TestDBConnection("user-1", &TestDBConnectionRequest{DBType: "mysql"})
	if err == nil {
		t.Fatal("expected missing field error")
	}
	if code != common.CodeArgumentError {
		t.Fatalf("expected code %d, got %d", common.CodeArgumentError, code)
	}
	want := "required argument are missing: database,username,host,port,password; "
	if err.Error() != want {
		t.Fatalf("expected %q, got %q", want, err.Error())
	}
}

func TestTestDBConnectionUnsupportedDatabaseType(t *testing.T) {
	code, err := NewAgentService().TestDBConnection("user-1", &TestDBConnectionRequest{
		DBType:   "postgres",
		Database: "rag_flow",
		Username: "root",
		Host:     "8.8.8.8",
		Port:     5432,
		Password: "password",
	})
	if err == nil {
		t.Fatal("expected unsupported database type error")
	}
	if code != common.CodeExceptionError {
		t.Fatalf("expected code %d, got %d", common.CodeExceptionError, code)
	}
	if err.Error() != "Unsupported database type." {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDBConnectionPortAcceptsStringAndNumber(t *testing.T) {
	if got := dbConnectionPort("3306"); got != "3306" {
		t.Fatalf("expected string port 3306, got %q", got)
	}
	if got := dbConnectionPort(float64(3306)); got != "3306" {
		t.Fatalf("expected numeric port 3306, got %q", got)
	}
}

// ptr returns a pointer to the given int64.
func ptr(v int64) *int64 { return &v }
