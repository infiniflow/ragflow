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
	// ToMap projects the item onto the historical wire shape. Callers must not
	// mutate the returned map's shared Extra contents.
	ToMap() map[string]any
}

// TextItem is a body/heading/list/code span. Extra carries
// layout/page_number/level/... passthrough.
type TextItem struct {
	Text  string
	Extra map[string]any
}

func (t TextItem) ToMap() map[string]any {
	m := map[string]any{
		"text":     t.Text,
		DocTypeKey: DocTypeText,
	}
	return mergeExtra(m, t.Extra)
}

// TableItem carries a parsed TableData.
type TableItem struct {
	Table     *entity.TableData
	Sheet     string
	Positions Positions
	Extra     map[string]any
}

func (t TableItem) ToMap() map[string]any {
	m := map[string]any{
		"table":    t.Table,
		DocTypeKey: DocTypeTable,
	}
	if t.Sheet != "" {
		m["sheet"] = t.Sheet
	}
	if len(t.Positions) > 0 {
		m["positions"] = positionsToWire(t.Positions)
	}
	return mergeExtra(m, t.Extra)
}

// ImageItem is an embedded image. Text is the alt text, Image the data
// URL/base64 payload (possibly empty when omitted).
type ImageItem struct {
	Text      string
	Image     string
	Sheet     string
	Positions Positions
	Extra     map[string]any
}

func (i ImageItem) ToMap() map[string]any {
	m := map[string]any{
		"text":     i.Text,
		"image":    i.Image,
		DocTypeKey: DocTypeImage,
	}
	if i.Sheet != "" {
		m["sheet"] = i.Sheet
	}
	if len(i.Positions) > 0 {
		m["positions"] = positionsToWire(i.Positions)
	}
	return mergeExtra(m, i.Extra)
}

// RawItem is a passthrough for items the parser constructs as an opaque map
// (the email content item, or any parser still emitting map literals during
// the migration). ToMap returns the map unchanged, so the wire shape is
// preserved exactly. The discriminator lives in the map under DocTypeKey.
type RawItem struct {
	M map[string]any
}

func (r RawItem) ToMap() map[string]any { return r.M }

// MarshalJSON projects each item onto its wire shape so that downstream
// consumers that round-trip the payload through encoding/json (the chunker
// decodes inputs["json"] via json.Marshal -> json.Unmarshal into []ChunkDoc)
// observe the historical map shape unchanged.
func (t TextItem) MarshalJSON() ([]byte, error)  { return json.Marshal(t.ToMap()) }
func (t TableItem) MarshalJSON() ([]byte, error) { return json.Marshal(t.ToMap()) }
func (i ImageItem) MarshalJSON() ([]byte, error) { return json.Marshal(i.ToMap()) }
func (r RawItem) MarshalJSON() ([]byte, error)   { return json.Marshal(r.M) }

// MapsToItems wraps a slice of payload maps as typed DocItems (one
// TextItem / TableItem / ImageItem / RawItem each, detected via doc_type_kwd).
// Parsers that accumulate results as []map[string]any use it at the assignment
// site (`JSON: MapsToItems(items)`) so their internal logic is unchanged while
// the field gains the DocItem type and the concrete TextItem/TableItem/
// ImageItem constructors become the emission path. Opaque maps without a known
// discriminator stay RawItem, preserving the wire shape exactly.
func MapsToItems(maps []map[string]any) []DocItem {
	out := make([]DocItem, 0, len(maps))
	for _, m := range maps {
		out = append(out, DocItemFromMap(m))
	}
	return out
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
		Text:  stringField(m, "text"),
		Extra: extraFromMap(m, "text", DocTypeKey, "ck_type"),
	}
}

func imageItemFromMap(m map[string]any) DocItem {
	pos := wireToPositions(m["positions"], hasSheetIndex(m))
	skip := []string{"text", "image", DocTypeKey, "ck_type", "sheet"}
	if pos != nil {
		// Parsed into the typed field: it owns "positions", so strip it from
		// Extra. Both [][]float64 (office/xlsx) and [][]any (PDF crop regions
		// from normalizeDeepDocPositions) parse here, so the old [][]any-only
		// Extra fallback is gone.
		skip = append(skip, "positions")
	}
	return ImageItem{
		Text:      stringField(m, "text"),
		Image:     stringField(m, "image"),
		Sheet:     stringField(m, "sheet"),
		Positions: pos,
		Extra:     extraFromMap(m, skip...),
	}
}

func tableItemFromMap(m map[string]any) DocItem {
	td, ok := m["table"].(*entity.TableData)
	// A table-typed map whose "table" value is not a *entity.TableData (e.g. a
	// JSON-decoded map[string]any) cannot be coerced into a TableItem without
	// silently dropping the data — TableItem.ToMap would emit "table": null.
	// Fall back to RawItem, preserving the map verbatim, exactly like the
	// text/image branches do when their canonical field is absent.
	if !ok {
		return RawItem{M: m}
	}
	pos := wireToPositions(m["positions"], hasSheetIndex(m))
	skip := []string{"table", DocTypeKey, "ck_type", "sheet"}
	if pos != nil {
		skip = append(skip, "positions")
	}
	return TableItem{
		Table:     td,
		Sheet:     stringField(m, "sheet"),
		Positions: pos,
		Extra:     extraFromMap(m, skip...),
	}
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
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
		// ck_type is retired; a stray Extra key must not resurrect it.
		if k == "ck_type" {
			continue
		}
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Position: strong-typed replacement for the loosely-typed `positions` wire
// field. The single wire shape is a 5-tuple array, but it carries TWO
// vocabularies that share the same five slots:
//   - AbsoluteBox:  [page, left, right, top, bottom]  (PDF layout bbox)
//   - RowColumnBox: [sheet, rowStart, rowEnd, colStart, colEnd] (spreadsheet)
// `Kind()` is a method on the concrete type (not a stored field) so it can
// never disagree with the type. The only consumer-visible seam is Wire(),
// which renders the historical tuple; ToMap projects through it so the JSON
// wire shape is unchanged.
// ---------------------------------------------------------------------------

type PositionKind int

const (
	// KindAbsolute marks a [page, left, right, top, bottom] bounding box.
	KindAbsolute PositionKind = iota
	// KindRowColumn marks a [sheet, rowStart, rowEnd, colStart, colEnd] range.
	KindRowColumn
)

// Position is one structured position entry. Implementations are concrete
// (AbsoluteBox / RowColumnBox); the interface exists only to give ToMap a
// single projection seam (Wire) and a type-safe discriminator (Kind).
type Position interface {
	// Wire renders the historical 5-tuple. AbsoluteBox keeps the page as an
	// int; RowColumnBox emits all ints.
	Wire() []any
	Kind() PositionKind
}

// AbsoluteBox is a PDF-style absolute bounding box: page number plus the box
// edges in page coordinates.
type AbsoluteBox struct {
	PageNumber int
	Left       float64
	Right      float64
	Top        float64
	Bottom     float64
}

func (AbsoluteBox) Kind() PositionKind { return KindAbsolute }

func (b AbsoluteBox) Wire() []any {
	return []any{b.PageNumber, b.Left, b.Right, b.Top, b.Bottom}
}

// RowColumnBox is a spreadsheet-style cell range: the 1-indexed sheet ordinal
// plus the row/column span. Sheet is the Nth sheet in the workbook, NOT the
// Nth table (one sheet emits one table unit; sheet_index is shared).
type RowColumnBox struct {
	Sheet int
	R0    int
	R1    int
	C0    int
	C1    int
}

func (RowColumnBox) Kind() PositionKind { return KindRowColumn }

func (b RowColumnBox) Wire() []any {
	return []any{b.Sheet, b.R0, b.R1, b.C0, b.C1}
}

// Positions is the structured position list carried by an ImageItem/TableItem.
type Positions []Position

// SpreadsheetPositions converts producer-side spreadsheet tuples
// ([sheet, r0, r1, c0, c1] as [][]float64) into RowColumnBoxes. Producers call
// this explicitly so the vocabulary is set at the source, never inferred.
func SpreadsheetPositions(tuples [][]float64) Positions {
	return rowColumnBoxesFromTyped(tuples)
}

// hasSheetIndex reports whether a payload map carries the spreadsheet
// discriminator. wireToPositions uses it to pick the vocabulary without
// guessing from the tuple shape (PDF and spreadsheet tuples are both five
// numbers). It mirrors the downstream isSpreadsheetChunk (task/indexdoc)
// exactly — sheet_index present and non-nil, or a non-empty "sheet" string —
// so the parser's classification and the indexer's spreadsheet/PDF decision
// use identical criteria and can never diverge. A present-but-nil sheet_index
// is not a spreadsheet signal.
func hasSheetIndex(m map[string]any) bool {
	if v, ok := m["sheet_index"]; ok && v != nil {
		return true
	}
	sheet, ok := m["sheet"].(string)
	return ok && sheet != ""
}

// wireToPositions is the in-bound seam: it converts a raw `positions` value
// into structured Positions. A nil / non-5-tuple input returns nil so the
// caller leaves the original value in Extra verbatim (positions-dropping
// regression guard). It never panics.
func wireToPositions(raw any, hasSheet bool) Positions {
	if hasSheet {
		return rowColumnBoxesFrom(raw)
	}
	return absoluteBoxesFrom(raw)
}

// positionsToWire is the out-bound seam: it renders structured Positions back
// into the historical wire form for ToMap. The wire container type is preserved
// per vocabulary so the parser output is byte-for-byte unchanged:
//   - AbsoluteBox (PDF)  -> [][]any  (int page + float64 coords), matching the
//     shape normalizeDeepDocPositions already emitted and keeping the page a
//     genuine int (a [][]float64 container would coerce it to float64).
//   - RowColumnBox (xlsx/csv) -> [][]float64, matching the historical
//     spreadsheet wire the ingestion indexer consumes.
//
// Preserving each producer's historical container keeps the wire unchanged and
// avoids churning the existing [][]float64 assertions; it is NOT a correctness
// requirement. Downstream, the chunker boundary stores positions as
// json.RawMessage and ChunkDoc.ToMap re-renders them via decodeStructuredValue,
// which unmarshals any all-numeric array into [][]float64 — so by the time the
// indexer runs, both vocabularies are already [][]float64 regardless of the
// container chosen here.
//
// A mixed slice (first element decides) cannot occur — a single item is either
// PDF or spreadsheet — so the [][]any fallback below is purely defensive and is
// never reached in practice.
func positionsToWire(p Positions) any {
	if len(p) == 0 {
		return nil
	}
	if p[0].Kind() == KindRowColumn {
		out := make([][]float64, 0, len(p))
		for _, pos := range p {
			b, ok := pos.(RowColumnBox)
			if !ok {
				return positionsToWireAny(p)
			}
			out = append(out, []float64{float64(b.Sheet), float64(b.R0), float64(b.R1), float64(b.C0), float64(b.C1)})
		}
		return out
	}
	return positionsToWireAny(p)
}

func positionsToWireAny(p Positions) []any {
	out := make([]any, 0, len(p))
	for _, pos := range p {
		out = append(out, pos.Wire())
	}
	return out
}

func absoluteBoxesFrom(raw any) Positions {
	switch v := raw.(type) {
	case [][]float64:
		return absoluteBoxesFromTyped(v)
	case [][]any:
		return absoluteBoxesFromAny(v)
	default:
		return nil
	}
}

func rowColumnBoxesFrom(raw any) Positions {
	switch v := raw.(type) {
	case [][]float64:
		return rowColumnBoxesFromTyped(v)
	case [][]any:
		return rowColumnBoxesFromAny(v)
	default:
		return nil
	}
}

func absoluteBoxesFromTyped(v [][]float64) Positions {
	out := make(Positions, 0, len(v))
	for _, t := range v {
		if len(t) != 5 {
			return nil
		}
		out = append(out, AbsoluteBox{
			PageNumber: int(t[0]),
			Left:       t[1],
			Right:      t[2],
			Top:        t[3],
			Bottom:     t[4],
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func absoluteBoxesFromAny(v [][]any) Positions {
	out := make(Positions, 0, len(v))
	for _, t := range v {
		if len(t) != 5 {
			return nil
		}
		page, ok := t[0].(int)
		if !ok {
			return nil
		}
		l, lok := positionFloat(t[1])
		r, rok := positionFloat(t[2])
		tp, tok := positionFloat(t[3])
		b, bok := positionFloat(t[4])
		if !lok || !rok || !tok || !bok {
			return nil
		}
		out = append(out, AbsoluteBox{PageNumber: page, Left: l, Right: r, Top: tp, Bottom: b})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func rowColumnBoxesFromTyped(v [][]float64) Positions {
	out := make(Positions, 0, len(v))
	for _, t := range v {
		if len(t) != 5 {
			return nil
		}
		out = append(out, RowColumnBox{
			Sheet: int(t[0]),
			R0:    int(t[1]),
			R1:    int(t[2]),
			C0:    int(t[3]),
			C1:    int(t[4]),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func rowColumnBoxesFromAny(v [][]any) Positions {
	out := make(Positions, 0, len(v))
	for _, t := range v {
		if len(t) != 5 {
			return nil
		}
		s, ok := toInt(t[0])
		if !ok {
			return nil
		}
		r0, ok0 := toInt(t[1])
		r1, ok1 := toInt(t[2])
		c0, ok2 := toInt(t[3])
		c1, ok3 := toInt(t[4])
		if !ok0 || !ok1 || !ok2 || !ok3 {
			return nil
		}
		out = append(out, RowColumnBox{Sheet: s, R0: r0, R1: r1, C0: c0, C1: c1})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func positionFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
