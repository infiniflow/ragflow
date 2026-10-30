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

// Size is a declared or actual media payload size in bytes. UnknownSize lets a
// caller reserve an occurrence before it knows how much data it will read.
type Size int64

const (
	// UnknownSize defers the byte check until Reservation.Commit.
	UnknownSize Size = -1
)

// Admission explains why a source or occurrence was accepted or rejected.
type Admission uint8

const (
	// Admitted means the caller owns a reservation and may read the payload.
	Admitted Admission = iota
	// RejectedByCount means the run's distinct-image limit is exhausted.
	RejectedByCount
	// RejectedByBytes means the per-image or total-byte limit rejected it.
	RejectedByBytes
	// RejectedInvalid means the supplied size is invalid.
	RejectedInvalid
)

// Budget enforces one parser run's image count and byte limits. Stable sources
// are admitted once through Reserve; source-less payloads use
// ReserveOccurrence so each occurrence consumes one item.
type Budget struct {
	limits Limits

	sources         map[string]*budgetSource
	sourcesUsed     int
	totalBytes      int64
	oversizedImages int
	totalOverflows  int
	countExhausted  bool
}

type budgetSource struct {
	included bool
	charged  bool
}

// Reservation is one admitted source or occurrence. Commit applies the actual
// payload size after the caller reads or decodes it.
type Reservation struct {
	budget *Budget
	entry  *budgetSource
}

// NewBudget returns a run budget using the current configured limits.
func NewBudget() *Budget {
	return NewBudgetWithLimits(LoadLimits())
}

// NewBudgetWithLimits returns a run budget with an explicit limits snapshot.
func NewBudgetWithLimits(limits Limits) *Budget {
	return &Budget{
		limits:  limits,
		sources: make(map[string]*budgetSource),
	}
}

// Reserve admits a stable source before its bytes are read. Repeated references
// to an included source reuse the same reservation. A declared size rejected
// by a byte limit is memoized as charged without consuming a source slot.
func (b *Budget) Reserve(source string, declared Size) (*Reservation, Admission) {
	if declared != UnknownSize && declared < 0 {
		return nil, RejectedInvalid
	}
	if entry, ok := b.sources[source]; ok {
		reservation := &Reservation{budget: b, entry: entry}
		if entry.charged {
			return nil, RejectedByBytes
		}
		if entry.included {
			return reservation, Admitted
		}
		if declared != UnknownSize && !b.admitBytes(entry, declared) {
			return nil, RejectedByBytes
		}
		return reservation, Admitted
	}
	if declared != UnknownSize && !b.canAdmitBytes(declared) {
		b.sources[source] = &budgetSource{charged: true}
		b.noteByteRejection(declared)
		return nil, RejectedByBytes
	}
	if b.countExhausted || b.sourcesUsed >= b.limits.MaxImagesPerRun {
		b.countExhausted = true
		return nil, RejectedByCount
	}
	entry := &budgetSource{}
	b.sources[source] = entry
	b.sourcesUsed++
	return &Reservation{budget: b, entry: entry}, Admitted
}

// ReserveOccurrence admits a source-less payload. The occurrence consumes one
// item before its actual size is known, matching sequential Office and
// Markdown extraction where byte-limit omissions must not stop later text.
func (b *Budget) ReserveOccurrence(declared Size) (*Reservation, Admission) {
	if declared != UnknownSize && declared < 0 {
		return nil, RejectedInvalid
	}
	if b.countExhausted || b.sourcesUsed >= b.limits.MaxImagesPerRun {
		b.countExhausted = true
		return nil, RejectedByCount
	}
	entry := &budgetSource{}
	b.sourcesUsed++
	if declared != UnknownSize && !b.admitBytes(entry, declared) {
		return nil, RejectedByBytes
	}
	return &Reservation{budget: b, entry: entry}, Admitted
}

// Commit applies the actual payload size to a reservation. It is idempotent
// for an included source and keeps byte-rejected reservations rejected.
func (r *Reservation) Commit(actual Size) bool {
	if r == nil || r.budget == nil || r.entry == nil {
		return false
	}
	if r.entry.included {
		return true
	}
	if r.entry.charged || actual < 0 {
		return false
	}
	if !r.budget.admitBytes(r.entry, actual) {
		return false
	}
	r.entry.included = true
	r.budget.totalBytes += int64(actual)
	return true
}

// Count reports how many source or occurrence slots this run has reserved.
func (b *Budget) Count() int {
	return b.sourcesUsed
}

func (b *Budget) canAdmitBytes(size Size) bool {
	if int64(size) > b.limits.ImageMaxBytes {
		return false
	}
	return int64(size) <= b.limits.MaxBytesPerRun-b.totalBytes
}

func (b *Budget) admitBytes(entry *budgetSource, size Size) bool {
	if entry.charged {
		return false
	}
	if b.canAdmitBytes(size) {
		return true
	}
	entry.charged = true
	b.noteByteRejection(size)
	return false
}

func (b *Budget) noteByteRejection(size Size) {
	if int64(size) > b.limits.ImageMaxBytes {
		b.oversizedImages++
		return
	}
	b.totalOverflows++
}

// Warnings reports the budget omissions accumulated by this run.
func (b *Budget) Warnings() []string {
	var warnings []string
	if b.oversizedImages > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"omitted %d embedded image payload(s) larger than the %d-byte per-image limit",
			b.oversizedImages,
			b.limits.ImageMaxBytes,
		))
	}
	if b.totalOverflows > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"omitted %d embedded image payload(s) after reaching the %d-byte run image budget",
			b.totalOverflows,
			b.limits.MaxBytesPerRun,
		))
	}
	if b.countExhausted {
		warnings = append(warnings, fmt.Sprintf(
			"stopped extracting embedded images after the %d-item run limit",
			b.limits.MaxImagesPerRun,
		))
	}
	return warnings
}
