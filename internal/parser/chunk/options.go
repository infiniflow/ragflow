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

// Typed options for the internal chunk execution path.

package chunk

import (
	"fmt"
)

// ChunkOptions is the typed configuration for production callers.
// It intentionally models only the option subset used by the current
// production call sites.
type ChunkOptions struct {
	// Preprocess flags. Any combination of the three is honoured; all
	// false means "no preprocess stage" (the engine skips the stage
	// rather than running an identity preprocess).
	NormalizeNewlines bool
	StripWhitespace   bool
	RemoveEmptyLines  bool

	// Split configuration. Callers must select a strategy; an unset
	// strategy degrades to "sentence" inside the operator.
	SplitStrategy string

	// Postprocess configuration. Zero values mean "do not run that
	// step"; non-zero values enable it.
	MergeTargetSize int

	// FilterMinLength > 0 drops chunks shorter than that (rune count).
	FilterMinLength int
}

// validate ensures the typed option set is internally consistent.
// The check is cheap; an option set that fails validation will not
// produce meaningful results at run time.
func (o ChunkOptions) validate() error {
	if o.MergeTargetSize < 0 {
		return fmt.Errorf("chunk: MergeTargetSize must be >= 0 (got %d)", o.MergeTargetSize)
	}
	if o.FilterMinLength < 0 {
		return fmt.Errorf("chunk: FilterMinLength must be >= 0 (got %d)", o.FilterMinLength)
	}
	return nil
}
