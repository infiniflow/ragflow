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

import "encoding/json"

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
	Text              string                     `json:"text,omitempty"`
	ContentWithWeight string                     `json:"content_with_weight,omitempty"`
	DocType           string                     `json:"doc_type_kwd,omitempty"`
	CKType            string                     `json:"ck_type,omitempty"`
	TKNums            *int                       `json:"tk_nums,omitempty"`
	Mom               string                     `json:"mom,omitempty"`
	ImgID             string                     `json:"img_id,omitempty"`
	Layout            string                     `json:"layout,omitempty"`
	LayoutType        string                     `json:"layout_type,omitempty"`
	LayoutNo          string                     `json:"layoutno,omitempty"`
	Image             string                     `json:"image,omitempty"`
	ContextAbove      string                     `json:"context_above,omitempty"`
	ContextBelow      string                     `json:"context_below,omitempty"`
	Questions         string                     `json:"questions,omitempty"`
	Keywords          string                     `json:"keywords,omitempty"`
	Summary           string                     `json:"summary,omitempty"`
	ChunkOrderInt     *int                       `json:"chunk_order_int,omitempty"`
	TitleTks          string                     `json:"title_tks,omitempty"`
	TitleSmTks        string                     `json:"title_sm_tks,omitempty"`
	ContentLtks       string                     `json:"content_ltks,omitempty"`
	ContentSmLtks     string                     `json:"content_sm_ltks,omitempty"`
	PageNumber        *int                       `json:"page_number,omitempty"`
	PDFPositions      json.RawMessage            `json:"_pdf_positions,omitempty"`
	Positions         json.RawMessage            `json:"positions,omitempty"`
	Extra             map[string]json.RawMessage `json:"-"`
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
	for _, key := range []string{
		"text", "content_with_weight", "doc_type_kwd", "mom", "img_id",
		"ck_type", "tk_nums", "layout", "layout_type", "layoutno", "image",
		"context_above", "context_below", "questions", "keywords", "summary",
		"chunk_order_int", "title_tks", "title_sm_tks", "content_ltks",
		"content_sm_ltks", "page_number", "_pdf_positions", "positions",
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
	default:
		return nil, false, nil
	}
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
		out[k] = decodeExtraValue(raw)
	}
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
