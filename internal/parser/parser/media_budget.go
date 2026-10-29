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
	"strconv"

	"ragflow/internal/parser/media"
)

type embeddedMediaBudget struct {
	budget        *media.Budget
	items         int
	nextSource    uint64
	pendingSource string
}

func newEmbeddedMediaBudget() *embeddedMediaBudget {
	return &embeddedMediaBudget{budget: media.NewBudget()}
}

// include reserves and charges one payload. The second result is false when
// callers should stop walking images because the run item limit was reached.
// Byte-limit omissions can keep walking so later text and media metadata
// remain available.
func (b *embeddedMediaBudget) include(data []byte) (bool, bool) {
	if !b.reserveItem() {
		return false, false
	}
	return b.includeReservedSize(len(data)), true
}

// reserveItem maps a sequential parser payload onto media.Budget's
// source-aware admission. Each attempt gets a fresh source, so the shared
// implementation enforces the count and byte caps without duplicate-source
// collapsing changing the parser output count.
func (b *embeddedMediaBudget) reserveItem() bool {
	if b == nil {
		return true
	}
	if b.budget == nil {
		b.budget = media.NewBudget()
	}
	source := strconv.FormatUint(b.nextSource, 10)
	b.nextSource++
	if !b.budget.Reserve(source) {
		return false
	}
	b.pendingSource = source
	b.items++
	return true
}

// includeReservedSize charges the payload reserved by the preceding
// reserveItem call through the same byte admission used by source-aware media.
func (b *embeddedMediaBudget) includeReservedSize(size int) bool {
	if b == nil {
		return true
	}
	if b.budget == nil || b.pendingSource == "" {
		return false
	}
	source := b.pendingSource
	b.pendingSource = ""
	return b.budget.Include(source, size)
}

func (b *embeddedMediaBudget) warnings() []string {
	if b == nil {
		return nil
	}
	return b.budget.Warnings()
}
