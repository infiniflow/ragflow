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

// FlattenLegacyParserSetups normalizes a Parser param map saved by the
// Python-era frontend. That shape nests the per-family setups under a
// "setups" key ({outputs, setups: {pdf: {...}}}), while the Go backend —
// the component runtime and the parser_config storage alike — uses the flat
// shape with file families as top-level keys. The nested families are
// lifted to the top level; when both shapes carry the same family the
// entries are field-merged with the top-level (Go-native) fields winning,
// mirroring the shallow per-field overlay NewParserComponent applies over
// its defaults. Maps without a nested "setups" object are returned
// unchanged.
func FlattenLegacyParserSetups(params map[string]any) map[string]any {
	nested, ok := params["setups"].(map[string]any)
	if !ok {
		return params
	}
	flat := make(map[string]any, len(params)+len(nested))
	for family, cfg := range nested {
		flat[family] = cfg
	}
	for k, v := range params {
		if k == "setups" {
			continue
		}
		flat[k] = mergeSetupEntry(flat[k], v)
	}
	return flat
}

// mergeSetupEntry field-merges two same-family setup maps, with the
// top-level entry winning on conflicting keys. Non-map values (or a
// non-map on either side) replace outright.
func mergeSetupEntry(nested, top any) any {
	nestedMap, ok1 := nested.(map[string]any)
	topMap, ok2 := top.(map[string]any)
	if !ok1 || !ok2 {
		return top
	}
	merged := make(map[string]any, len(nestedMap)+len(topMap))
	for k, v := range nestedMap {
		merged[k] = v
	}
	for k, v := range topMap {
		merged[k] = v
	}
	return merged
}

// ParserOutputs is the result of invoking the Parser component. The
// wire format is "json", with structured JSON items as the only payload.
type ParserOutputs struct {
	// Name is the resolved source filename.
	Name string `json:"name"`

	// FileType is the canonical extension used for parser dispatch.
	FileType string `json:"file_type"`

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
