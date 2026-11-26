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

package admin

import (
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"strings"
	"testing"
)

func TestDeleteUserPreservesAdminRestrictions(t *testing.T) {
	for _, tt := range []struct {
		name, active, want string
		admin              bool
	}{
		{name: "active", active: "1", want: "deactivate"},
		{name: "admin", active: "0", admin: true, want: "admin account"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t, &entity.User{})
			t.Cleanup(testutil.ReplaceDBForTest(t, db))
			user := &entity.User{ID: "actor", Email: "actor@example.test", Nickname: "Actor", IsActive: tt.active, IsAuthenticated: "1", IsAnonymous: "0", IsSuperuser: new(tt.admin)}
			if err := db.Create(user).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := NewService().DeleteUser(t.Context(), user.Email); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v", err)
			}
			if _, err := dao.NewUserDAO().GetByID(t.Context(), db, user.ID); err != nil {
				t.Fatalf("user removed: %v", err)
			}
		})
	}
}

func TestDeleteUserCleanupWarningsPrecedeDone(t *testing.T) {
	db := testutil.SetupTestDB(t,
		&entity.User{}, &entity.Tenant{}, &entity.UserTenant{},
		&entity.Knowledgebase{}, &entity.Document{}, &entity.Task{}, &entity.File{}, &entity.File2Document{},
		&entity.Chat{}, &entity.ChatSession{}, &entity.ConversationMessage{}, &entity.ConversationReference{},
		&entity.API4Conversation{}, &entity.API4ConversationMessage{}, &entity.API4ConversationReference{},
		&entity.ChatChannel{}, &entity.UserCanvas{}, &entity.UserCanvasVersion{},
		&entity.Search{}, &entity.Memory{}, &entity.MemoryTask{}, &entity.MCPServer{},
		&entity.SkillSearchConfig{},
		&entity.CompilationTemplate{}, &entity.CompilationTemplateGroup{},
		&entity.Connector{}, &entity.Connector2Kb{}, &entity.SyncLogs{},
		&entity.KnowledgeCompileDataset{}, &entity.WikiDocumentDirty{}, &entity.PipelineOperationLog{},
		&entity.IngestionTask{}, &entity.IngestionTaskLog{}, &entity.FileCommit{}, &entity.FileCommitItem{},
		&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{},
		&entity.TenantLangfuse{}, &entity.APIToken{}, &entity.InvitationCode{},
	)
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	user := &entity.User{ID: "victim", Email: "victim@example.test", Nickname: "Victim", IsActive: "0", IsAuthenticated: "1", IsAnonymous: "0"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	// An owned tenant makes loadUserDeletionData produce external work. With no
	// engine/storage initialised in a unit test, cleanup fails and emits warnings.
	if err := db.Create(&entity.UserTenant{ID: "membership", UserID: user.ID, TenantID: "owned-tenant", Role: "owner"}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := NewService().DeleteUser(t.Context(), user.Email)
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	warningIndex, doneIndex := -1, -1
	for i, detail := range result.DeletedDetails {
		if strings.HasPrefix(detail, "- Warning:") && warningIndex < 0 {
			warningIndex = i
		}
		if detail == "Delete done!" && doneIndex < 0 {
			doneIndex = i
		}
	}
	if warningIndex < 0 {
		t.Fatalf("expected a cleanup warning in details: %v", result.DeletedDetails)
	}
	if doneIndex < 0 {
		t.Fatalf("expected a completion marker in details: %v", result.DeletedDetails)
	}
	if warningIndex > doneIndex {
		t.Fatalf("cleanup warning must precede the completion marker: %v", result.DeletedDetails)
	}
}
