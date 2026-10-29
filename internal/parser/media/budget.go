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

package media

import "fmt"

// Budget admits source-identified images into one parser run. Unlike a plain
// item counter it dedupes by source: a document that repeats one icon a
// thousand times stays bounded without charging the document for a thousand
// images, while distinct sources keep the count and byte caps meaningful.
//
// Callers Reserve immediately before reading a source and Include after a
// successful read. A source that fails to read keeps its reserved item until
// the run ends, because the read attempt already cost the work the cap bounds.
// The count and byte limits are the same run-level limits enforced by the
// media cache; the parser applies them early so it does not decode or read
// payloads the cache would reject.
type Budget struct {
	maxSources    int
	maxImageBytes int
	maxTotalBytes int64

	sources         map[string]*budgetSource
	sourcesUsed     int
	totalBytes      int64
	oversizedImages int
	totalOverflows  int
	countExhausted  bool
}

// budgetSource records how one distinct source was charged. charged marks a
// source rejected by the per-image or total-byte limit so repeated references
// cannot inflate the warning counters.
type budgetSource struct {
	reserved bool
	included bool
	charged  bool
}

// NewBudget returns a run budget bounded by the same run-level count and byte
// limits as the media cache, plus the shared per-image limit.
func NewBudget() *Budget {
	return &Budget{
		maxSources:    MediaMaxImagesPerRun(),
		maxImageBytes: ImageMaxBytes(),
		maxTotalBytes: MediaMaxBytesPerRun(),
		sources:       make(map[string]*budgetSource),
	}
}

// Reserve admits one source before its bytes are read. A source already
// admitted by this run returns true without consuming another item; exhausted
// source counts set the warning state and reject every new source.
func (b *Budget) Reserve(source string) bool {
	if b == nil {
		return true
	}
	if entry, ok := b.sources[source]; ok && entry.reserved {
		return true
	}
	if b.countExhausted {
		return false
	}
	if b.sourcesUsed >= b.maxSources {
		b.countExhausted = true
		return false
	}
	if b.sources == nil {
		b.sources = make(map[string]*budgetSource)
	}
	b.sourcesUsed++
	b.sources[source] = &budgetSource{reserved: true}
	return true
}

// Include charges a successfully read source against the per-image and total
// byte limits. Repeating an included source adds no bytes; a source already
// rejected by a byte limit keeps returning false without being counted again.
func (b *Budget) Include(source string, size int) bool {
	if b == nil {
		return true
	}
	entry, ok := b.sources[source]
	if !ok {
		entry = &budgetSource{}
		if b.sources == nil {
			b.sources = make(map[string]*budgetSource)
		}
		b.sources[source] = entry
	}
	if entry.included {
		return true
	}
	if entry.charged {
		return false
	}
	if size > b.maxImageBytes {
		entry.charged = true
		b.oversizedImages++
		return false
	}
	if int64(size) > b.maxTotalBytes-b.totalBytes {
		entry.charged = true
		b.totalOverflows++
		return false
	}
	entry.included = true
	b.totalBytes += int64(size)
	return true
}

// ReserveBytes checks a declared size before the caller reads or decodes the
// source. It uses the same per-image and total-byte limits as Include, but
// charges nothing on success: Include still owns the actual bytes. A source
// rejected here is marked charged so repeated references do not read it again
// or inflate the warning counters. ZIP callers use this with the declared
// uncompressed size to avoid reading a large entry that the total budget will
// reject.
func (b *Budget) ReserveBytes(source string, size int) bool {
	if b == nil {
		return true
	}
	entry, ok := b.sources[source]
	if !ok {
		entry = &budgetSource{}
		if b.sources == nil {
			b.sources = make(map[string]*budgetSource)
		}
		b.sources[source] = entry
	}
	if entry.included {
		return true
	}
	if entry.charged {
		return false
	}
	if size > b.maxImageBytes {
		entry.charged = true
		b.oversizedImages++
		return false
	}
	if int64(size) > b.maxTotalBytes-b.totalBytes {
		entry.charged = true
		b.totalOverflows++
		return false
	}
	return true
}

// NoteOversized records a source rejected before reading (for example a ZIP
// entry whose declared size exceeds the per-image limit). It does not consume
// an item: nothing was read, and a document full of huge entries must not
// exhaust the image count.
func (b *Budget) NoteOversized() {
	if b == nil {
		return
	}
	b.oversizedImages++
}

// Warnings reports the budget omissions accumulated by this run.
func (b *Budget) Warnings() []string {
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
	if b.totalOverflows > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"omitted %d embedded image payload(s) after reaching the %d-byte run image budget",
			b.totalOverflows,
			b.maxTotalBytes,
		))
	}
	if b.countExhausted {
		warnings = append(warnings, fmt.Sprintf(
			"stopped extracting embedded images after the %d-item run limit",
			b.maxSources,
		))
	}
	return warnings
}
