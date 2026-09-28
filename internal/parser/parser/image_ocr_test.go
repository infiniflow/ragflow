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

package parser

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestImageOCRBudget_ExpiredDocumentBudgetWarns(t *testing.T) {
	budget := newImageOCRBudget(context.Background())
	budget.cancel()
	defer budget.close()

	if got := budget.recognize([]byte("image payload")); got != "" {
		t.Fatalf("recognize = %q, want no OCR text after document budget expires", got)
	}
	if warnings := strings.Join(budget.warnings(), "\n"); !strings.Contains(warnings, errImageOCRBudgetExpired.Error()) {
		t.Fatalf("warnings = %q, want document-budget expiration", warnings)
	}
}

func TestImageOCRBudget_AdmissionWaitHonorsContextDeadline(t *testing.T) {
	admission := sharedImageMediaAdmission()
	releases := make([]func(), 0, cap(admission.slots))
	for range cap(admission.slots) {
		release, err := admission.acquire(context.Background())
		if err != nil {
			t.Fatalf("fill OCR admission: %v", err)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	budget := &imageOCRBudget{parent: context.Background(), ctx: ctx, cancel: cancel}
	defer budget.close()

	if got := budget.recognize([]byte("image payload")); got != "" {
		t.Fatalf("recognize = %q, want no OCR text after admission deadline", got)
	}
	if warnings := strings.Join(budget.warnings(), "\n"); !strings.Contains(warnings, context.DeadlineExceeded.Error()) {
		t.Fatalf("warnings = %q, want admission deadline failure", warnings)
	}
}
