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
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// wantTierSubject is the subject the injected default resolver must route to:
// the single community queue.
const wantTierSubject = common.TaskSubject

// TestCreateForDocumentsPublishesTieredSubject is the core happy-path RED:
// a freshly created ingestion task must be published onto the tier subject
// (the default lane), not left unrouted.
func TestCreateForDocumentsPublishesTieredSubject(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	// Inject a deterministic default resolver so the routing assertion is
	// edition-independent; the community (!ee) and ee default lanes are covered
	// separately by the resolver tests.
	svc.tierResolver = (&fakeTierResolver{tier: common.TierDefault}).resolve

	resp, err := svc.CreateForDocuments(permission.WithIdentity(t.Context(), "user-1", ""), "kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	if publisher.subject != wantTierSubject {
		t.Fatalf("published subject = %q, want %q", publisher.subject, wantTierSubject)
	}
}

// TestCreateForDocumentsPublishesToDefaultSubject is the adversarial twin of the
// happy path: the created task must be published to the single community subject
// and never left empty.
func TestCreateForDocumentsPublishesToDefaultSubject(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	// Inject a deterministic default resolver so the routing assertion is
	// edition-independent; the community (!ee) and ee default lanes are covered
	// separately by the resolver tests.
	svc.tierResolver = (&fakeTierResolver{tier: common.TierDefault}).resolve

	if _, err := svc.CreateForDocuments(permission.WithIdentity(t.Context(), "user-1", ""), "kb-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if publisher.subject != common.TaskSubject || publisher.subject == "" {
		t.Fatalf("published subject = %q must be the single community subject, never empty", publisher.subject)
	}
}

// TestCreateForDocumentsMultipleDocsAllTiered covers the batch path: every
// document in one CreateForDocuments call must publish to a tier subject with
// no document falling back to the legacy subject (off-by-one / partial wiring).
func TestCreateForDocumentsMultipleDocsAllTiered(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 2, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestDoc(t, "doc-2", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	// Inject a deterministic default resolver so the routing assertions are
	// edition-independent; the community (!ee) and ee default lanes are covered
	// separately by the resolver tests.
	svc.tierResolver = (&fakeTierResolver{tier: common.TierDefault}).resolve

	resp, err := svc.CreateForDocuments(permission.WithIdentity(t.Context(), "user-1", ""), "kb-1", []string{"doc-1", "doc-2"})
	if err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resp))
	}
	if len(publisher.messages) != 2 {
		t.Fatalf("expected 2 published messages, got %d", len(publisher.messages))
	}
	if publisher.subject != wantTierSubject {
		t.Fatalf("last published subject = %q, want %q", publisher.subject, wantTierSubject)
	}
}

// TestCreateForDocumentsTieringKeepsTaskMessage guards against a regression
// where subject routing accidentally mutates the durable TaskMessage (TaskID /
// TaskType). This passes today and must keep passing after the change.
func TestCreateForDocumentsTieringKeepsTaskMessage(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	// Inject a deterministic default resolver so the routing assertions are
	// edition-independent; the community (!ee) and ee default lanes are covered
	// separately by the resolver tests.
	svc.tierResolver = (&fakeTierResolver{tier: common.TierDefault}).resolve

	if _, err := svc.CreateForDocuments(permission.WithIdentity(t.Context(), "user-1", ""), "kb-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.TaskType != common.TaskTypeIngestionTask {
		t.Fatalf("task type = %q, want %q", msg.TaskType, common.TaskTypeIngestionTask)
	}
	if msg.TaskID == "" {
		t.Fatal("task id must not be emptied by tier routing")
	}
}

// TestCreateForDocumentsPublishFailureStillFailsTask is the error-path guard:
// a broker publish error must still settle the task as FAILED (fail loudly)
// after subject tiering is introduced. Uses the existing recording publisher's
// error injection.
func TestCreateForDocumentsPublishFailureStillFailsTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{err: errors.New("broker unavailable")}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	// Inject a deterministic default resolver so the routing assertions are
	// edition-independent; the community (!ee) and ee default lanes are covered
	// separately by the resolver tests.
	svc.tierResolver = (&fakeTierResolver{tier: common.TierDefault}).resolve

	ctx := t.Context()
	// CreateForDocuments collects per-document failures into responses[].Result
	// and returns a nil outer error; the broker error is reported there.
	resp, err := svc.CreateForDocuments(permission.WithIdentity(ctx, "user-1", ""), "kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments returned unexpected outer error: %v", err)
	}
	if len(resp) != 1 || !strings.Contains(resp[0].Result, "broker unavailable") {
		t.Fatalf("expected per-document publish failure, got %+v", resp)
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("status = %q, want %q", task.Status, common.FAILED)
	}
}
