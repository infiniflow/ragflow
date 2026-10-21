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

import (
	"encoding/json"

	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

// PayloadFormat is the discriminator shared by parser/chunker/tokenizer
// wire payloads.
type PayloadFormat string

const (
	PayloadFormatJSON     PayloadFormat = "json"
	PayloadFormatMarkdown PayloadFormat = "markdown"
	PayloadFormatText     PayloadFormat = "text"
	PayloadFormatHTML     PayloadFormat = "html"
	PayloadFormatChunks   PayloadFormat = "chunks"
)

func (f PayloadFormat) isKnown() bool {
	switch f {
	case "", PayloadFormatJSON, PayloadFormatMarkdown, PayloadFormatText, PayloadFormatHTML, PayloadFormatChunks:
		return true
	default:
		return false
	}
}

// ChunkerFileMeta is the common file descriptor shape carried through
// parser/chunker/tokenizer boundaries. Known fields are modeled
// explicitly; any parser-specific enrichments stay in Extra.
type ChunkerFileMeta struct {
	Name      string                     `json:"name,omitempty"`
	Path      string                     `json:"path,omitempty"`
	Bucket    string                     `json:"bucket,omitempty"`
	Binary    string                     `json:"binary,omitempty"`
	Blob      string                     `json:"blob,omitempty"`
	ID        string                     `json:"id,omitempty"`
	PageCount *int                       `json:"page_count,omitempty"`
	Extra     map[string]json.RawMessage `json:"-"`
}

func (m *ChunkerFileMeta) UnmarshalJSON(data []byte) error {
	type alias ChunkerFileMeta
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	delete(raw, "name")
	delete(raw, "path")
	delete(raw, "bucket")
	delete(raw, "binary")
	delete(raw, "blob")
	delete(raw, "id")
	delete(raw, "page_count")
	*m = ChunkerFileMeta(decoded)
	if len(raw) > 0 {
		m.Extra = raw
	}
	return nil
}

func (m ChunkerFileMeta) MarshalJSON() ([]byte, error) {
	type alias ChunkerFileMeta
	base, err := json.Marshal(alias(m))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(base, &raw); err != nil {
		return nil, err
	}
	for k, v := range m.Extra {
		if _, exists := raw[k]; !exists {
			raw[k] = v
		}
	}
	return json.Marshal(raw)
}

// ChunkDoc is the typed chunk item shared by chunker/tokenizer
// boundaries. Common fields are explicit; dynamic enrichments are
// preserved in Extra for forward compatibility.
type ChunkDoc struct {
	Text          string          `json:"text,omitempty"`
	DocType       string          `json:"doc_type_kwd,omitempty"`
	TKNums        *int            `json:"tk_nums,omitempty"`
	Mom           string          `json:"mom,omitempty"`
	ImgID         string          `json:"img_id,omitempty"`
	ID            string          `json:"id,omitempty"`
	Layout        string          `json:"layout,omitempty"`
	LayoutType    string          `json:"layout_type,omitempty"`
	LayoutNo      string          `json:"layoutno,omitempty"`
	Image         string          `json:"image,omitempty"`
	ContextAbove  string          `json:"context_above,omitempty"`
	ContextBelow  string          `json:"context_below,omitempty"`
	Questions     string          `json:"questions,omitempty"`
	Keywords      string          `json:"keywords,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	ChunkOrderInt *int            `json:"chunk_order_int,omitempty"`
	TitleTks      string          `json:"title_tks,omitempty"`
	TitleSmTks    string          `json:"title_sm_tks,omitempty"`
	ContentLtks   string          `json:"content_ltks,omitempty"`
	ContentSmLtks string          `json:"content_sm_ltks,omitempty"`
	PageNumber    *int            `json:"page_number,omitempty"`
	TopInt        []int           `json:"top_int,omitempty"`
	PDFPositions  json.RawMessage `json:"_pdf_positions,omitempty"`
	Positions     json.RawMessage `json:"positions,omitempty"`
	Sheet         string          `json:"sheet,omitempty"`
	SheetIndex    *int            `json:"sheet_index,omitempty"`
	// PreserveOrder declares the input sequence authoritative; geometry sorting
	// must retain it. PaddleOCR sets this for its ordered layout blocks.
	PreserveOrder bool `json:"_preserve_order,omitempty"`
	// TableData is the structured table contract. When a chunk carries it
	// the table content is consumed directly from here instead of being
	// re-parsed from Text markup. It must stay out of Extra (see the
	// deletion list in UnmarshalJSON).
	TableData *entity.TableData          `json:"table,omitempty"`
	Extra     map[string]json.RawMessage `json:"-"`
}

func (d *ChunkDoc) UnmarshalJSON(data []byte) error {
	type alias ChunkDoc
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// Known ChunkDoc fields plus tag_kwd, which has no ChunkDoc field: Go's
	// tagger emits tag_feas and the vocabulary comes from a tag source file,
	// so no chunk carries tag_kwd. Deleted rather than left in raw so it
	// cannot fall through to Extra and be re-emitted by MarshalJSON.
	// ck_type is a retired parser key: it has no field, but must stay in this
	// list so an old payload cannot leak it back through Extra.
	for _, key := range []string{
		"text", "doc_type_kwd", "mom", "img_id",
		"ck_type", "tk_nums", "layout", "layout_type", "layoutno", "image",
		"context_above", "context_below", "questions", "keywords", "summary",
		"chunk_order_int", "title_tks", "title_sm_tks", "content_ltks",
		"content_sm_ltks", "tag_kwd", "page_number", "top_int", "_pdf_positions", "positions",
		"sheet", "sheet_index", "table", "_preserve_order",
	} {
		delete(raw, key)
	}
	*d = ChunkDoc(decoded)
	if len(raw) > 0 {
		d.Extra = raw
	}
	return nil
}

func (d ChunkDoc) MarshalJSON() ([]byte, error) {
	type alias ChunkDoc
	base, err := json.Marshal(alias(d))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(base, &raw); err != nil {
		return nil, err
	}
	for k, v := range d.Extra {
		// ck_type is retired; a stray Extra key must not resurrect it.
		if k == "ck_type" {
			continue
		}
		if _, exists := raw[k]; !exists {
			raw[k] = v
		}
	}
	return json.Marshal(raw)
}

// ChunkDocFromMap decodes a free-form map into a typed ChunkDoc.
func ChunkDocFromMap(in map[string]any) (ChunkDoc, error) {
	if in == nil {
		return ChunkDoc{}, nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return ChunkDoc{}, err
	}
	var doc ChunkDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return ChunkDoc{}, err
	}
	return doc, nil
}

// ChunkDocsFromAny converts either []ChunkDoc, []map[string]any, or []any
// into a typed chunk slice.
func ChunkDocsFromAny(in any) ([]ChunkDoc, bool, error) {
	switch t := in.(type) {
	case []ChunkDoc:
		return t, true, nil
	case []map[string]any:
		out := make([]ChunkDoc, 0, len(t))
		for _, item := range t {
			doc, err := ChunkDocFromMap(item)
			if err != nil {
				return nil, true, err
			}
			out = append(out, doc)
		}
		return out, true, nil
	case []any:
		out := make([]ChunkDoc, 0, len(t))
		for _, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			doc, err := ChunkDocFromMap(m)
			if err != nil {
				return nil, true, err
			}
			out = append(out, doc)
		}
		return out, true, nil
	case []parser.DocItem:
		return ChunkDocsFromDocItem(t)
	default:
		return nil, false, nil
	}
}

// ChunkDocsFromDocItem converts a parser-produced []DocItem directly into a
// typed chunk slice by projecting each item through its canonical DocItem.ToMap
// wire shape and decoding the result via ChunkDocFromMap (see chunkDocFromDocItem).
// The resulting []ChunkDoc is therefore identical to what the historical
// ChunkDocsFromAny([]DocItem) path produced, which round-tripped through the same
// ToMap projection.
// This is the consumer-side entry the chunker uses once parsers emit DocItems
// directly; other callers that still pass []map[string]any / []any / []ChunkDoc
// keep using ChunkDocsFromAny.
func ChunkDocsFromDocItem(items []parser.DocItem) ([]ChunkDoc, bool, error) {
	if items == nil {
		return nil, false, nil
	}
	out := make([]ChunkDoc, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		doc, err := chunkDocFromDocItem(it)
		if err != nil {
			return nil, true, err
		}
		out = append(out, doc)
	}
	return out, true, nil
}

// chunkDocFromDocItem converts a parser DocItem directly into a typed chunk by
// projecting it through its canonical ToMap wire shape and decoding that via
// ChunkDocFromMap. Using ToMap — the parser package's single canonical
// projection, with mergeExtra semantics — instead of a second, divergent wire
// map builder keeps the conversion identical to the historical []map path and
// prevents an Extra key from shadowing an empty canonical field (e.g. a stray
// Extra["text"] must never override TextItem.Text == "").
func chunkDocFromDocItem(it parser.DocItem) (ChunkDoc, error) {
	return ChunkDocFromMap(it.ToMap())
}

// ToMap converts a typed chunk back to the generic map form used by
// runtime.Component boundaries.
func (d ChunkDoc) ToMap() map[string]any {
	b, err := json.Marshal(d)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{}
	}
	for k, raw := range d.Extra {
		if k == "ck_type" {
			continue
		}
		out[k] = decodeExtraValue(raw)
	}
	// Every pre-index chunk has one canonical text field. Keep the key even
	// for media-only chunks so chunk identity and image upload use the same
	// wire contract; contextual text remains explicit retrieval metadata.
	out["text"] = d.Text
	delete(out, "content_with_weight")
	if len(d.PDFPositions) > 0 {
		out["_pdf_positions"] = decodeStructuredValue(d.PDFPositions)
	}
	if len(d.Positions) > 0 {
		out["positions"] = decodeStructuredValue(d.Positions)
	}
	return out
}

// ChunkDocsToMaps converts a typed chunk slice to the generic map form.
func ChunkDocsToMaps(in []ChunkDoc) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, doc := range in {
		out = append(out, doc.ToMap())
	}
	return out
}

// ContextualText joins a chunk's body with its surrounding media context in
// the order Python materializes it (rag/flow/chunker/token_chunker.py:343):
// context_above, text, context_below, concatenated as they are. No separator
// is inserted: Python's finalize concatenates directly, and the context
// collectors keep their own boundary punctuation and newlines.
func ContextualText(d ChunkDoc) string {
	return d.ContextAbove + d.Text + d.ContextBelow
}

func (d *ChunkDoc) SetExtraValue(key string, value any) error {
	if d.Extra == nil {
		d.Extra = make(map[string]json.RawMessage)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	d.Extra[key] = raw
	return nil
}

func (d *ChunkDoc) GetExtraString(key string) (string, bool) {
	if d.Extra == nil {
		return "", false
	}
	raw, ok := d.Extra[key]
	if !ok {
		return "", false
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil || out == "" {
		return "", false
	}
	return out, true
}

func (d *ChunkDoc) GetExtraStringSlice(key string) ([]string, bool) {
	if d.Extra == nil {
		return nil, false
	}
	raw, ok := d.Extra[key]
	if !ok {
		return nil, false
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return nil, false
	}
	return out, true
}

func decodeExtraValue(raw json.RawMessage) any {
	var floats []float64
	if err := json.Unmarshal(raw, &floats); err == nil {
		return floats
	}
	var stringsOut []string
	if err := json.Unmarshal(raw, &stringsOut); err == nil {
		return stringsOut
	}
	var stringOut string
	if err := json.Unmarshal(raw, &stringOut); err == nil {
		return stringOut
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err == nil {
		return generic
	}
	return nil
}

func decodeStructuredValue(raw json.RawMessage) any {
	var matrix [][]float64
	if err := json.Unmarshal(raw, &matrix); err == nil {
		return matrix
	}
	var mapsOut []map[string]any
	if err := json.Unmarshal(raw, &mapsOut); err == nil {
		return mapsOut
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err == nil {
		return generic
	}
	return nil
}
