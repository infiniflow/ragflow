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
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// tierResolver injection seam: tests replace the resolver to drive error
// outcomes deterministically without a live billing service.
type fakeTierResolver struct {
	tier      common.Tier
	err       error
	gotTenant string
	calls     int
}

func (f *fakeTierResolver) resolve(_ context.Context, tenantID string) (common.Tier, error) {
	f.calls++
	f.gotTenant = tenantID
	return f.tier, f.err
}

func newTierTestSvc(t *testing.T, resolver *fakeTierResolver, publisher *recordingTaskPublisher) *IngestionTaskService {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	if resolver != nil {
		svc.tierResolver = resolver.resolve
	}
	return svc
}

// --- SubjectForTier: mapping + adversarial unknown-tier ---

func TestSubjectForTierMapsDefaultTier(t *testing.T) {
	got, err := common.SubjectForTier(common.TierDefault)
	if err != nil {
		t.Fatalf("SubjectForTier(%q) returned error: %v", common.TierDefault, err)
	}
	if got != "tasks.RAGFLOW" {
		t.Fatalf("SubjectForTier(%q) = %q, want %q", common.TierDefault, got, "tasks.RAGFLOW")
	}
}

// Adversarial: an unknown/empty/case-mangled tier must return an error and an
// empty subject, never silently fall back to a default lane. A wrong impl that
// defaults to a tier would pass the happy-path test but fail here.
func TestSubjectForTierRejectsUnknownTier(t *testing.T) {
	for _, bad := range []common.Tier{"", "DEFAULT", "default ", "urgent", "default\n", common.Tier("\x00")} {
		got, err := common.SubjectForTier(bad)
		if err == nil {
			t.Fatalf("SubjectForTier(%q) must error, got subject %q", bad, got)
		}
		if got != "" {
			t.Fatalf("SubjectForTier(%q) must return empty subject on error, got %q", bad, got)
		}
	}
}

// --- resolveTier: tenant extraction + fail-loudly on missing KB ---

// Adversarial: the resolver must receive the dataset's owning tenant, not the
// acting user, the dataset id, or an empty string.
func TestResolveTierPassesDatasetTenantToResolver(t *testing.T) {
	fake := &fakeTierResolver{tier: common.TierDefault}
	svc := newTierTestSvc(t, fake, &recordingTaskPublisher{})
	ctx := t.Context()
	if _, err := svc.CreateForDocuments(permission.WithIdentity(ctx, "user-1", ""), "kb-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("resolver called %d times, want 1", fake.calls)
	}
	if fake.gotTenant != "tenant-1" {
		t.Fatalf("resolver got tenant %q, want %q (dataset owner)", fake.gotTenant, "tenant-1")
	}
}

// Adversarial: a missing knowledgebase must fail loudly (error), never pick a
// silent default tier.
func TestResolveTierMissingKnowledgebaseReturnsError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	// deliberately no insertTestKB
	svc := NewIngestionTaskService()
	if _, err := svc.resolveTier(t.Context(), "kb-does-not-exist", nil); err == nil {
		t.Fatal("resolveTier must fail when the knowledgebase is missing")
	}
}

// --- enqueueTask: default routing + resolver/unknown-tier error handling ---

// Adversarial twin of the default happy path: a tenant resolved to the default
// tier must be published to the default subject, not a legacy bare queue.
func TestEnqueueTenantPublishesDefaultSubject(t *testing.T) {
	fake := &fakeTierResolver{tier: common.TierDefault}
	publisher := &recordingTaskPublisher{}
	svc := newTierTestSvc(t, fake, publisher)
	if _, err := svc.CreateForDocuments(permission.WithIdentity(t.Context(), "user-1", ""), "kb-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments: %v", err)
	}
	if publisher.subject != "tasks.RAGFLOW" {
		t.Fatalf("default tenant published to %q, want tasks.RAGFLOW (community default lane)", publisher.subject)
	}
}

// Adversarial: a resolver error must settle the task FAILED and publish nothing
// (fail loudly). A wrong impl that ignores the error and publishes to a default
// would be caught here.
func TestEnqueueResolverErrorFailsTaskAndPublishesNothing(t *testing.T) {
	fake := &fakeTierResolver{err: errors.New("subscription lookup failed")}
	publisher := &recordingTaskPublisher{}
	svc := newTierTestSvc(t, fake, publisher)
	ctx := t.Context()
	resp, err := svc.CreateForDocuments(permission.WithIdentity(ctx, "user-1", ""), "kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments returned unexpected outer error: %v", err)
	}
	if len(resp) != 1 || resp[0].Result == "" {
		t.Fatalf("expected a per-document failure result, got %+v", resp)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("resolver error must not publish, got %d messages", len(publisher.messages))
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, dao.DB, "doc-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("status = %q, want %q (fail loudly)", task.Status, common.FAILED)
	}
}

// Adversarial: an unknown tier returned by the resolver must propagate through
// SubjectForTier as an error and settle the task FAILED, not silently coerce.
func TestEnqueueUnknownTierFailsTaskAndPublishesNothing(t *testing.T) {
	fake := &fakeTierResolver{tier: common.Tier("bogus-tier")}
	publisher := &recordingTaskPublisher{}
	svc := newTierTestSvc(t, fake, publisher)
	ctx := t.Context()
	resp, err := svc.CreateForDocuments(permission.WithIdentity(ctx, "user-1", ""), "kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments returned unexpected outer error: %v", err)
	}
	if len(resp) != 1 || resp[0].Result == "" {
		t.Fatalf("expected a per-document failure result, got %+v", resp)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("unknown tier must not publish, got %d messages", len(publisher.messages))
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, dao.DB, "doc-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("status = %q, want %q", task.Status, common.FAILED)
	}
}
