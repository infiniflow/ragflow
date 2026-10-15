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

package parser

import (
	"encoding/json"

	"ragflow/internal/entity"
)

// DocItem is one structured piece of a parser's output, replacing the previous
// loosely-typed `map[string]any` payload. The parser package emits DocItems;
// the ingestion component and downstream consumers still read the wire form
// produced by ToMap, so the external JSON shape (key names, pipeline-only
// keys, the doc_type_kwd discriminator) is unchanged by this type.
//
// The design mirrors the existing typed chunk AST (schema.ChunkDoc) but lives
// in the parser package so the parser layer does not import ingestion. Ad-hoc
// pipeline keys (layout, page_number, _pdf_positions, sheet_index, media_order,
// parent_table_id, ...) that are not worth modelling as named fields are passed
// through verbatim via the Extra map, keeping ToMap a 1:1 projection of the
// historical map shape. A later pass may promote the well-known Extra keys to
// named fields; that is out of scope for the boundary-adapter change.
type DocItem interface {
	// DocType returns the item's doc_type_kwd discriminator (text/table/image).
	DocType() string
	// ToMap projects the item onto the historical wire shape. Callers must not
	// mutate the returned map's shared Extra contents.
	ToMap() map[string]any
}

// TextItem is a body/heading/list/code span. CKType optionally carries the
// legacy ck_type discriminator (heading/list/code/...); it is omitted from
// ToMap when empty. Extra carries layout/page_number/level/... passthrough.
type TextItem struct {
	Text   string
	CKType string
	Extra  map[string]any
}

func (t TextItem) DocType() string { return DocTypeText }

func (t TextItem) ToMap() map[string]any {
	m := map[string]any{
		"text":     t.Text,
		DocTypeKey: DocTypeText,
	}
	if t.CKType != "" {
		m["ck_type"] = t.CKType
	}
	return mergeExtra(m, t.Extra)
}

// TableItem carries a parsed TableData. CKType is the legacy ck_type
// discriminator (always "table" today, mirroring NewTableDataJSONItem); it is
// emitted for pipeline compatibility and will be dropped in a later pass.
type TableItem struct {
	Table     *entity.TableData
	Sheet     string
	Positions [][]float64
	CKType    string
	Extra     map[string]any
}

func (t TableItem) DocType() string { return DocTypeTable }

func (t TableItem) ToMap() map[string]any {
	m := map[string]any{
		"table":    t.Table,
		DocTypeKey: DocTypeTable,
		"ck_type":  firstNonEmptyStr(t.CKType, DocTypeTable),
	}
	if t.Sheet != "" {
		m["sheet"] = t.Sheet
	}
	if len(t.Positions) > 0 {
		m["positions"] = t.Positions
	}
	return mergeExtra(m, t.Extra)
}

// ImageItem is an embedded image. Text is the alt text, Image the data
// URL/base64 payload (possibly empty when omitted). CKType carries the legacy
// ck_type discriminator ("image" for xlsx/pptx/html/markdown, empty for docx);
// it is omitted from ToMap when empty.
type ImageItem struct {
	Text      string
	Image     string
	Sheet     string
	Positions [][]float64
	CKType    string
	Extra     map[string]any
}

func (i ImageItem) DocType() string { return DocTypeImage }

func (i ImageItem) ToMap() map[string]any {
	m := map[string]any{
		"text":     i.Text,
		"image":    i.Image,
		DocTypeKey: DocTypeImage,
	}
	if i.CKType != "" {
		m["ck_type"] = i.CKType
	}
	if i.Sheet != "" {
		m["sheet"] = i.Sheet
	}
	if len(i.Positions) > 0 {
		m["positions"] = i.Positions
	}
	return mergeExtra(m, i.Extra)
}

// RawItem is a passthrough for items the parser constructs as an opaque map
// (the email content item, or any parser still emitting map literals during
// the migration). ToMap returns the map unchanged, so the wire shape is
// preserved exactly. DocType is empty because the discriminator lives in the
// map under DocTypeKey.
type RawItem struct {
	M map[string]any
}

func (RawItem) DocType() string { return "" }

func (r RawItem) ToMap() map[string]any { return r.M }

// MarshalJSON projects each item onto its wire shape so that downstream
// consumers that round-trip the payload through encoding/json (the chunker
// decodes inputs["json"] via json.Marshal -> json.Unmarshal into []ChunkDoc)
// observe the historical map shape unchanged.
func (t TextItem) MarshalJSON() ([]byte, error)  { return json.Marshal(t.ToMap()) }
func (t TableItem) MarshalJSON() ([]byte, error) { return json.Marshal(t.ToMap()) }
func (i ImageItem) MarshalJSON() ([]byte, error) { return json.Marshal(i.ToMap()) }
func (r RawItem) MarshalJSON() ([]byte, error)   { return json.Marshal(r.M) }

// FromMap wraps an opaque payload map as a DocItem. Used by boundary readers
// that receive a []map[string]any and must hand it to DocItem-typed APIs.
func FromMap(m map[string]any) DocItem { return RawItem{M: m} }

// Raw wraps an opaque payload map as a DocItem. Parsers that still build items
// as map literals use it at the assignment site (e.g. `JSON: MapsToItems(items)`
// or `res.JSON = append(res.JSON, Raw(m))`), so their internal logic stays
// unchanged while the field gains the DocItem type.
func Raw(m map[string]any) DocItem { return RawItem{M: m} }

// MapsToItems wraps a slice of payload maps as typed DocItems (one
// TextItem / TableItem / ImageItem / RawItem each, detected via doc_type_kwd).
// Parsers that accumulate results as []map[string]any use it at the assignment
// site (`JSON: MapsToItems(items)`) so their internal logic is unchanged while
// the field gains the DocItem type and the concrete TextItem/TableItem/
// ImageItem constructors become the emission path. Opaque maps without a known
// discriminator stay RawItem, preserving the wire shape exactly.
func MapsToItems(maps []map[string]any) []DocItem {
	return DocItemsFromMaps(maps)
}

// DocItemFromMap detects the item's doc_type_kwd and builds the matching typed
// DocItem, extracting the canonical wire fields into the struct and leaving
// every other key in Extra. For an opaque map without a known discriminator it
// falls back to RawItem, preserving the map exactly. This is the producer-side
// counterpart to ItemsToMaps: parsers can emit typed DocItems without altering
// the wire shape downstream consumers (ToMap) rely on.
func DocItemFromMap(m map[string]any) DocItem {
	if m == nil {
		return RawItem{}
	}
	switch docType, _ := m[DocTypeKey].(string); docType {
	case DocTypeText:
		// Only build a TextItem when the map actually carries a text field.
		// A text-typed map without a text key (e.g. an email metadata item
		// whose body is gated by fields) must round-trip through ToMap
		// unchanged; TextItem.ToMap always writes "text", so forcing it here
		// would invent a text key the original map lacked.
		if _, ok := m["text"]; ok {
			return textItemFromMap(m)
		}
		return RawItem{M: m}
	case DocTypeImage:
		if _, ok := m["image"]; ok {
			return imageItemFromMap(m)
		}
		return RawItem{M: m}
	case DocTypeTable:
		if _, ok := m["table"]; ok {
			return tableItemFromMap(m)
		}
		return RawItem{M: m}
	default:
		return RawItem{M: m}
	}
}

func textItemFromMap(m map[string]any) DocItem {
	return TextItem{
		Text:   stringField(m, "text"),
		CKType: stringField(m, "ck_type"),
		Extra:  extraFromMap(m, "text", DocTypeKey, "ck_type"),
	}
}

func imageItemFromMap(m map[string]any) DocItem {
	return ImageItem{
		Text:      stringField(m, "text"),
		Image:     stringField(m, "image"),
		CKType:    stringField(m, "ck_type"),
		Sheet:     stringField(m, "sheet"),
		Positions: positionsField(m),
		// "positions" is intentionally NOT stripped: when the map carries
		// positions as [][]any (e.g. PDF crop regions from normalizePDFPositions)
		// the typed field cannot capture them, and stripping would drop the key
		// from ToMap. Leaving it in Extra preserves the wire shape exactly.
		Extra: extraFromMap(m, "text", "image", DocTypeKey, "ck_type", "sheet"),
	}
}

func tableItemFromMap(m map[string]any) DocItem {
	td, _ := m["table"].(*entity.TableData)
	return TableItem{
		Table:     td,
		Sheet:     stringField(m, "sheet"),
		Positions: positionsField(m),
		CKType:    stringField(m, "ck_type"),
		// Same as imageItemFromMap: keep "positions" in Extra so non-[][]float64
		// position matrices survive the typed conversion unchanged.
		Extra: extraFromMap(m, "table", DocTypeKey, "ck_type", "sheet"),
	}
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func positionsField(m map[string]any) [][]float64 {
	p, _ := m["positions"].([][]float64)
	return p
}

// extraFromMap returns every key of m that is not a canonical field for the
// item type, so it lands in the item's Extra passthrough (matching mergeExtra
// semantics downstream). A nil result means "no extra keys".
func extraFromMap(m map[string]any, canonical ...string) map[string]any {
	skip := make(map[string]struct{}, len(canonical))
	for _, c := range canonical {
		skip[c] = struct{}{}
	}
	extra := make(map[string]any)
	for k, v := range m {
		if _, ok := skip[k]; ok {
			continue
		}
		extra[k] = v
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// DocItemsFromMaps maps a []map[string]any to typed DocItems, preserving order,
// via DocItemFromMap. Parsers that accumulate results as []map[string]any use
// it at the assignment site (`JSON: DocItemsFromMaps(items)`) so their internal
// logic is unchanged while the field gains the DocItem type and the concrete
// constructors become the emission path.
func DocItemsFromMaps(maps []map[string]any) []DocItem {
	if maps == nil {
		return nil
	}
	out := make([]DocItem, 0, len(maps))
	for _, m := range maps {
		out = append(out, DocItemFromMap(m))
	}
	return out
}

// ItemsToMaps projects a slice of DocItems onto the historical wire form. The
// ingestion boundary calls this once so downstream consumers keep reading
// []map[string]any unchanged.
func ItemsToMaps(items []DocItem) []map[string]any {
	if items == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, it.ToMap())
	}
	return out
}

// mergeExtra copies Extra entries onto m without clobbering canonical keys
// that were already set. Canonical typed fields always win; ad-hoc pipeline
// keys (layout, page_number, level, ...) are added only when not already
// present, so a stray Extra key can never shadow text/table/image/doc_type_kwd.
func mergeExtra(m, extra map[string]any) map[string]any {
	for k, v := range extra {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return m
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
