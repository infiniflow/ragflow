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

import "fmt"

const (
	maxEmbeddedImageBytes         = 32 << 20
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
		maxImageBytes: maxEmbeddedImageBytes,
		maxTotalBytes: maxEmbeddedDocumentImageBytes,
		maxItems:      maxEmbeddedMediaItems,
	}
}

// include reserves budget for one image payload. The second result is false
// when callers should stop walking images because the document item limit has
// been reached. Byte-limit omissions can keep walking so later text and media
// metadata remain available.
func (b *embeddedMediaBudget) include(data []byte) (bool, bool) {
	if b.countExhausted {
		return false, false
	}
	if b.items >= b.maxItems {
		b.countExhausted = true
		return false, false
	}
	b.items++
	if len(data) > b.maxImageBytes {
		b.oversizedImages++
		return false, true
	}
	if len(data) > b.maxTotalBytes-b.totalBytes {
		b.documentOverflows++
		return false, true
	}
	b.totalBytes += len(data)
	return true, true
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
