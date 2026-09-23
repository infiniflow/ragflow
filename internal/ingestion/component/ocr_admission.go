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

package component

import (
	"context"
	"sync"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
)

type ocrMediaAdmission struct {
	slots chan struct{}
}

func newOCRMediaAdmission(limit int) *ocrMediaAdmission {
	if limit < 1 {
		limit = 1
	}
	return &ocrMediaAdmission{slots: make(chan struct{}, limit)}
}

func (a *ocrMediaAdmission) acquire(ctx context.Context) (func(), error) {
	select {
	case a.slots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-a.slots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var (
	processOCRMediaAdmissionOnce sync.Once
	processOCRMediaAdmission     *ocrMediaAdmission
)

func sharedOCRMediaAdmission() *ocrMediaAdmission {
	processOCRMediaAdmissionOnce.Do(func() {
		processOCRMediaAdmission = newOCRMediaAdmission(deepdocpdf.DeepDocConcurrency())
	})
	return processOCRMediaAdmission
}
