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
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"

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
	versions, err := svc.ListVersions("canvas-1")
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
	versions, err := svc.ListVersions("canvas-empty")
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

// TestCheckCanvasAccess_Owner verifies that the canvas owner gets access.
func TestCheckCanvasAccess_Owner(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "a@b.com"})
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Title: sptr("My Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-1", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if !ok {
		t.Error("expected owner to have access")
	}
}

// TestCheckCanvasAccess_NotOwner verifies that a tenant member can access
// a team-level canvas.
func TestCheckCanvasAccess_NotOwner(t *testing.T) {
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

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "a@b.com"})
	testDB.Create(&entity.User{ID: "user-2", Nickname: "member", Email: "c@d.com"})
	// user-2 is a member of user-1's tenant (status "1" = active)
	testDB.Create(&entity.UserTenant{ID: "ut-1", UserID: "user-2", TenantID: "user-1", Role: "member", Status: sptr("1")})
	// Canvas has team-level permission
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Permission: "team", Title: sptr("Team Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-2", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if !ok {
		t.Error("expected tenant member to have access to team canvas")
	}
}

// TestCheckCanvasAccess_PrivateCanvas_Denied verifies that a tenant member
// cannot access a private (default "me") canvas.
func TestCheckCanvasAccess_PrivateCanvas_Denied(t *testing.T) {
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

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "a@b.com"})
	testDB.Create(&entity.User{ID: "user-2", Nickname: "member", Email: "c@d.com"})
	// user-2 is a tenant member (status "1" = active)
	testDB.Create(&entity.UserTenant{ID: "ut-1", UserID: "user-2", TenantID: "user-1", Role: "member", Status: sptr("1")})
	// Canvas has default "me" permission (private)
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Title: sptr("Private Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-2", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if ok {
		t.Error("expected tenant member to be denied access to private canvas")
	}
}

// TestCheckCanvasAccess_NotFound verifies behavior for non-existent canvas.
func TestCheckCanvasAccess_NotFound(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "tester", Email: "a@b.com"})

	svc := NewAgentService()
	_, err := svc.CheckCanvasAccess("user-1", "non-existent")
	if err == nil {
		t.Error("expected error for non-existent canvas")
	}
}

// TestGetVersion_Success verifies getting a specific version by ID.
func TestGetVersion_Success(t *testing.T) {
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
		DSL:          entity.JSONMap{"model": "gpt-4"},
	})

	svc := NewAgentService()
	v, err := svc.GetVersion("canvas-1", "v1")
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
	_, err := svc.GetVersion("canvas-other", "v1")
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
	_, err := svc.GetVersion("canvas-1", "non-existent")
	if err == nil {
		t.Error("expected error for non-existent version")
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
