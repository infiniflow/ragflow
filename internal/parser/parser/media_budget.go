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
	"fmt"

	"ragflow/internal/parser/media"
)

const (
	maxEmbeddedDocumentImageBytes = 64 << 20
	maxEmbeddedMediaItems         = 256
)

type embeddedMediaBudget struct {
	maxImageBytes int
	maxTotalBytes int
	maxItems      int

	items             int
	totalBytes        int
	oversizedImages   int
	documentOverflows int
	countExhausted    bool
}

func newEmbeddedMediaBudget() *embeddedMediaBudget {
	return &embeddedMediaBudget{
		maxImageBytes: media.ImageMaxBytes(),
		maxTotalBytes: maxEmbeddedDocumentImageBytes,
		maxItems:      maxEmbeddedMediaItems,
	}
}

// include reserves budget for one image payload. The second result is false
// when callers should stop walking images because the document item limit has
// been reached. Byte-limit omissions can keep walking so later text and media
// metadata remain available.
func (b *embeddedMediaBudget) include(data []byte) (bool, bool) {
	if !b.reserveItem() {
		return false, false
	}
	return b.includeReservedSize(len(data)), true
}

// reserveItem caps the image fetch attempts for one document. Callers reserve
// immediately before the I/O; skips that read no bytes are not charged.
func (b *embeddedMediaBudget) reserveItem() bool {
	if b.countExhausted {
		return false
	}
	if b.items >= b.maxItems {
		b.countExhausted = true
		return false
	}
	b.items++
	return true
}

// includeReservedSize charges a payload against the per-image and document
// byte limits without retaining or decoding it just to measure its size.
func (b *embeddedMediaBudget) includeReservedSize(size int) bool {
	if size > b.maxImageBytes {
		b.oversizedImages++
		return false
	}
	if size > b.maxTotalBytes-b.totalBytes {
		b.documentOverflows++
		return false
	}
	b.totalBytes += size
	return true
}

func (b *embeddedMediaBudget) warnings() []string {
	if b == nil {
		return nil
	}
	var warnings []string
	if b.oversizedImages > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"omitted %d embedded image payload(s) larger than the %d-byte per-image limit",
			b.oversizedImages,
			b.maxImageBytes,
		))
	}
	if b.documentOverflows > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"omitted %d embedded image payload(s) after reaching the %d-byte document image budget",
			b.documentOverflows,
			b.maxTotalBytes,
		))
	}
	if b.countExhausted {
		warnings = append(warnings, fmt.Sprintf(
			"stopped extracting embedded images after the %d-item document limit",
			b.maxItems,
		))
	}
	return warnings
}
