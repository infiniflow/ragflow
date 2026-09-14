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

package schema

// ParserSetup is a per-filetype parser configuration block. The keys are heterogeneous (e.g.,
// `parse_method`, `lang`, `output_format`, `suffix`, `fields`, `vlm`),
// so a free-form map best mirrors the Python dict literal.
type ParserSetup map[string]any

// ParserOutputs is the result of invoking the Parser component. The
// wire format is "json", with structured JSON items as the only payload.
type ParserOutputs struct {
	// Name is the resolved source filename.
	Name string `json:"name"`

	// OutputFormat is always "json". Downstream components consume structured JSON items.
	OutputFormat string `json:"output_format,omitempty"`

	// JSON holds the list of structured sections (primary payload).
	JSON []map[string]any `json:"json"`

	// Lang is the language used by downstream tokenization and model calls.
	Lang string `json:"lang,omitempty"`

	// File is backend-produced file metadata (for example page_count,
	// outline, format, or sheets). When present, it replaces rather than
	// merges with the upstream file descriptor.
	File map[string]any `json:"file,omitempty"`

	// DocID, Bucket, and Path let downstream chunkers reacquire the source PDF
	// for preview cropping without carrying its binary in the payload.
	DocID  string `json:"doc_id,omitempty"`
	Bucket string `json:"bucket,omitempty"`
	Path   string `json:"path,omitempty"`
}
