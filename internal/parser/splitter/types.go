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

package splitter

import "errors"

// ErrSplitterNotSupported indicates CGO/pdfium is not available in the current build.
var ErrSplitterNotSupported = errors.New("splitter: pdf slicing requires cgo and pdfium")

// ChapterPlan defines one adaptive chapter slice to be parsed.
type ChapterPlan struct {
	Index     int    `json:"index"`      // 0-indexed sequential chapter number
	Title     string `json:"title"`      // Derived chapter title or label
	StartPage int    `json:"start_page"` // 0-indexed inclusive start page
	EndPage   int    `json:"end_page"`   // 0-indexed inclusive end page
	PageCount int    `json:"page_count"` // EndPage - StartPage + 1
}

// SplitOptions controls the adaptive chapter splitting behavior.
type SplitOptions struct {
	// MinPages is the minimum page threshold for a chapter before coalescing
	// with adjacent chapters (default 30).
	MinPages int `json:"min_pages"`

	// MaxPages is the target upper page boundary for a single chapter (default 120).
	MaxPages int `json:"max_pages"`

	// ThresholdPages is the minimum total pages of a document to trigger
	// adaptive chapter splitting (default 200).
	ThresholdPages int `json:"threshold_pages"`

	// MaxConcurrency bounds concurrent OCR worker parsing requests (default 2).
	MaxConcurrency int `json:"max_concurrency"`
}

const (
	DefaultMinPages       = 30
	DefaultMaxPages       = 120
	DefaultThresholdPages = 200
	DefaultMaxConcurrency = 2
)

// DefaultSplitOptions returns standard industrial adaptive splitting settings.
func DefaultSplitOptions() SplitOptions {
	return SplitOptions{
		MinPages:       DefaultMinPages,
		MaxPages:       DefaultMaxPages,
		ThresholdPages: DefaultThresholdPages,
		MaxConcurrency: DefaultMaxConcurrency,
	}
}

// Normalize ensures reasonable bounds on split options.
func (opts SplitOptions) Normalize() SplitOptions {
	if opts.MinPages <= 0 {
		opts.MinPages = DefaultMinPages
	}
	if opts.MaxPages <= opts.MinPages {
		opts.MaxPages = DefaultMaxPages
	}
	if opts.ThresholdPages <= 0 {
		opts.ThresholdPages = DefaultThresholdPages
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = DefaultMaxConcurrency
	}
	return opts
}
