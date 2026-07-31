// Package utility provides a Go port of the Python `markdown_to_json`
// library's markdown-outline → nested-dict pipeline, plus the
// list-to-key/value, merge, and tree-shaping steps that RAGFlow's
// MindMapExtractor layers on top.
//
// # Provenance
//
// This file reproduces two Python sources, kept byte-for-behavior where the
// port is faithful:
//
//  1. The `markdown_to_json` package (vendored in this repo at
//     .venv/lib/python3.13/site-packages/markdown_to_json/markdown_to_json.py):
//     - dictify            -> Dictify
//     - CMarkASTNester.nest / _dictify_blocks / _ensure_list_singleton
//     + dictify_list_by  -> dictifyBlocks
//     - Renderer.stringify_dict / _valuify / _render_block /
//     _render_generic_block / _render_List / _render_FencedCode
//     -> valuify / renderBlockText / renderBlockAny / renderList / toBlock
//
//  2. RAGFlow's MindMapExtractor
//     (rag/advanced_rag/knowlege_compile/mind_map_extractor.py):
//     - _todict (+ _list_to_kv)   -> Todict
//     - _merge                    -> MergeDicts
//     - _be_children              -> BeChildren
//     - _key                      -> StripStars
//     - __call__'s final shaping  -> ShapeTree
//     - re.sub(r"```[^\n]*", "")  -> StripFences
//
// The public entry points used by the knowledge-compiler mindmap component are
// Dictify, Todict, MergeDicts, ShapeTree, StripFences, and the Node / OMap
// types. The remaining helpers are unexported.
//
// # Deviations from the Python sources
//
// The port is behaviorally faithful for the common mindmap output shape, but
// the following intentional or incidental differences exist (all documented so
// the divergences are auditable against the Python golden behavior):
//
//  1. (Fixed here) Closed ATX headings: Python's CommonMark strips BOTH the
//     leading and the trailing '#' run ("## Title ##" -> "Title"). This port
//     now strips the trailing '#' too, in headingRawText.
//
//  2. Heading text is taken from the raw source line (goldmark's
//     Heading.Lines) instead of goldmark's inline-parsed text, so inline
//     emphasis markers (asterisks) survive exactly as Python's block.strings
//     would keep them.
//
//  3. A list that is NOT the first child of a heading is rendered as readable
//     flattened item text joined by "\n"; Python's _valuify falls back to
//     str() of the nested list (a Python list literal). This is a deliberate
//     divergence for the rare "list mid-children" case.
//
//  4. Fenced code blocks: goldmark keeps each source line's trailing newline,
//     so the rendered block carries one extra trailing "\n" compared with
//     Python's string_content. Minor.
//
//  5. Todict term keying types the preceding item as a string; if it is not a
//     string the key becomes "" and is later dropped by BeChildren. Python
//     would raise TypeError (unhashable) on a list term. The port is more
//     robust.
//
//  6. BeChildren skips non-string list items instead of raising on unhashable
//     list members (same defensive rationale as #5).
//
//  7. The fence-strip-before-parse behavior is inherited from Python
//     (re.sub(r"```[^\n]*","") runs before parsing), so any fenced code in an
//     LLM reply is stripped before it is parsed. Faithful to the original.
//
//  8. An empty merged dict degrades to a bare root node (ShapeTree returns
//     &Node{ID:"root"}); Python's __call__ would raise IndexError on
//     keys()[0]. The port keeps the pipeline alive.
package utility

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// kv is one ordered key/value pair; OMap mirrors Python's OrderedDict (key
// order is load-bearing for tree shape and merge order).
type kv struct {
	k string
	v any
}
type OMap []kv

// Node is one shaped mind-map node (Python's {"id", "children"} shape).
type Node struct {
	ID       string
	Children []*Node
}

type blockType int

const (
	headingBlock blockType = iota
	listBlock
	paraBlock
	fencedBlock
	otherBlock
)

// mdBlock is one markdown block in dictify terms: heading (level+text), list
// (flattened items), paragraph/fenced/other (text).
type mdBlock struct {
	typ   blockType
	level int
	text  string
	items []any
}

// fenceStripRE mirrors re.sub(r"```[^\n]*", "", response): whole fence marker
// lines (``` or ```lang) are removed before parsing.
var fenceStripRE = regexp.MustCompile("```[^\\n]*")

// StripFences removes markdown code-fence marker lines from an LLM reply.
func StripFences(s string) string {
	return fenceStripRE.ReplaceAllString(s, "")
}

// Dictify mirrors markdown_to_json.dictify: nest top-level blocks by the
// MINIMUM top heading level; content before the first heading at each level
// is dropped; when a level has no headings the blocks become a list (the top
// level then renders as {"root": [...]}).
func Dictify(md string) OMap {
	src := []byte(md)
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))
	var blocks []mdBlock
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		blocks = append(blocks, toBlock(n, src))
	}
	if len(blocks) == 0 {
		return OMap{}
	}
	min := 100000
	for _, b := range blocks {
		lvl := 100000
		if b.typ == headingBlock {
			lvl = b.level
		}
		if lvl < min {
			min = lvl
		}
	}
	switch t := dictifyBlocks(blocks, min).(type) {
	case OMap:
		out := OMap{}
		for _, e := range t {
			out = append(out, kv{e.k, valuify(e.v)})
		}
		return out
	case []mdBlock:
		// No headings anywhere: {"root": [rendered blocks]} (Renderer's
		// non-dict branch).
		rendered := make([]any, 0, len(t))
		for _, b := range t {
			rendered = append(rendered, renderBlockAny(b))
		}
		return OMap{{"root", rendered}}
	}
	return OMap{}
}

// dictifyBlocks mirrors CMarkASTNester._dictify_blocks: split blocks by
// headings at exactly `level`, recursing at level+1. Returns OMap or the raw
// []mdBlock (no headings at this level).
func dictifyBlocks(blocks []mdBlock, level int) any {
	has := false
	for _, b := range blocks {
		if b.typ == headingBlock && b.level == level {
			has = true
			break
		}
	}
	if !has {
		return blocks
	}
	out := OMap{}
	var cur string
	var children []mdBlock
	started := false
	for _, b := range blocks {
		if b.typ == headingBlock && b.level == level {
			if started {
				out = setOMap(out, cur, children)
			}
			cur = b.text
			children = nil
			started = true
			continue
		}
		if started {
			children = append(children, b)
		}
	}
	if started {
		out = setOMap(out, cur, children)
	}
	for i := range out {
		out[i].v = dictifyBlocks(out[i].v.([]mdBlock), level+1)
	}
	return out
}

// valuify mirrors Renderer._valuify: dict → recurse; empty → ""; first child
// a list → its items; otherwise the blocks joined with "\n\n".
func valuify(v any) any {
	switch t := v.(type) {
	case OMap:
		out := OMap{}
		for _, e := range t {
			out = append(out, kv{e.k, valuify(e.v)})
		}
		return out
	case []mdBlock:
		if len(t) == 0 {
			return ""
		}
		if t[0].typ == listBlock {
			return t[0].items
		}
		parts := make([]string, 0, len(t))
		for _, b := range t {
			parts = append(parts, renderBlockText(b))
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

// renderBlockText renders one block to a string (Renderer._render_*).
// A list appearing mid-children is rendered as its flattened item texts
// (Python str()'s the nested list — a pathological case we render readably;
// see deviation #3).
func renderBlockText(b mdBlock) string {
	switch b.typ {
	case fencedBlock:
		return "```\n" + b.text + "```"
	case listBlock:
		var parts []string
		for _, item := range b.items {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return b.text
	}
}

func renderBlockAny(b mdBlock) any {
	if b.typ == listBlock {
		return b.items
	}
	return renderBlockText(b)
}

// Todict mirrors _todict + _list_to_kv: lists under dict keys convert to
// key→definition pairs ([prev scalar] → sublist[0]); a list with no
// definition sub-lists collapses to an empty dict (the bullets are dropped —
// faithful to Python). Recurses into nested dicts.
func Todict(m OMap) OMap {
	for i := range m {
		switch val := m[i].v.(type) {
		case OMap:
			m[i].v = Todict(val)
		case []any:
			nv := OMap{}
			for j, item := range val {
				lst, isList := item.([]any)
				if !isList || j == 0 {
					continue
				}
				key, _ := val[j-1].(string)
				var first any
				if len(lst) > 0 {
					first = lst[0]
				}
				nv = append(nv, kv{key, first})
			}
			m[i].v = nv
		}
	}
	return m
}

// MergeDicts mirrors _merge: d1 merges INTO d2 (ordered). Both dicts recurse;
// both lists concatenate d2's then d1's items; otherwise d1 wins. Keys absent
// from d2 append in d1's order.
func MergeDicts(d1, d2 OMap) OMap {
	for _, e := range d1 {
		idx := indexOMap(d2, e.k)
		if idx < 0 {
			d2 = append(d2, e)
			continue
		}
		dv := d2[idx].v
		d1Dict, d1IsDict := e.v.(OMap)
		d2Dict, d2IsDict := dv.(OMap)
		d1List, d1IsList := e.v.([]any)
		d2List, d2IsList := dv.([]any)
		switch {
		case d1IsDict && d2IsDict:
			d2[idx].v = MergeDicts(d1Dict, d2Dict)
		case d1IsList && d2IsList:
			d2[idx].v = append(d2List, d1List...)
		default:
			d2[idx].v = e.v
		}
	}
	return d2
}

// ShapeTree mirrors __call__'s final shaping: >1 top-level keys → synthetic
// "root" node whose children are the dict-valued keys (with a keyset seeded
// by those keys); exactly 1 → that key becomes the root id. An empty merged
// dict degrades to a bare root (Python would raise IndexError on
// keys()[0]; the Go port keeps the pipeline alive — deviation #8).
func ShapeTree(merged OMap) *Node {
	if len(merged) > 1 {
		keyset := map[string]bool{}
		for _, e := range merged {
			if _, ok := e.v.(OMap); !ok {
				continue
			}
			if k := StripStars(e.k); k != "" {
				keyset[k] = true
			}
		}
		var children []*Node
		for _, e := range merged {
			if _, ok := e.v.(OMap); !ok {
				continue
			}
			k := StripStars(e.k)
			if k == "" {
				continue
			}
			children = append(children, &Node{ID: k, Children: BeChildren(e.v, keyset)})
		}
		return &Node{ID: "root", Children: children}
	}
	if len(merged) == 1 {
		k := StripStars(merged[0].k)
		return &Node{ID: k, Children: BeChildren(merged[0].v, map[string]bool{k: true})}
	}
	return &Node{ID: "root"}
}

// BeChildren mirrors _be_children: strings/lists become leaf nodes; dict keys
// become nested nodes. keyset is SHARED across the whole subtree walk — a key
// already seen anywhere below is skipped (silent dedup, as in Python).
// Non-string list items are skipped (deviation #6) rather than raising.
func BeChildren(v any, keyset map[string]bool) []*Node {
	switch t := v.(type) {
	case string:
		keyset[t] = true
		if id := StripStars(t); id != "" {
			return []*Node{{ID: id}}
		}
		return nil
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok {
				keyset[s] = true
			}
		}
		var out []*Node
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				continue
			}
			if id := StripStars(s); id != "" {
				out = append(out, &Node{ID: id})
			}
		}
		return out
	case OMap:
		var out []*Node
		for _, e := range t {
			k := StripStars(e.k)
			if k == "" || keyset[k] {
				continue
			}
			keyset[k] = true
			out = append(out, &Node{ID: k, Children: BeChildren(e.v, keyset)})
		}
		return out
	}
	return nil
}

// starsRE mirrors re.compile(r"\*+") used by _key.
var starsRE = regexp.MustCompile(`\*+`)

// StripStars mirrors _key (re.sub(r"\*+", "", k)).
func StripStars(s string) string {
	return starsRE.ReplaceAllString(s, "")
}

// ---- goldmark AST → mdBlock ----

func toBlock(n ast.Node, src []byte) mdBlock {
	switch node := n.(type) {
	case *ast.Heading:
		return mdBlock{typ: headingBlock, level: node.Level, text: headingRawText(node, src)}
	case *ast.List:
		return mdBlock{typ: listBlock, items: renderList(node, src)}
	case *ast.FencedCodeBlock:
		return mdBlock{typ: fencedBlock, text: rawLines(n, src, "")}
	case *ast.Paragraph:
		return mdBlock{typ: paraBlock, text: rawLines(n, src, "\n")}
	default:
		return mdBlock{typ: otherBlock, text: rawLines(n, src, "\n")}
	}
}

// headingRawText renders the heading's raw source line without the ATX
// markers, keeping inline markdown (e.g. asterisks) intact like CommonMark's
// block.strings (goldmark's inline text would strip emphasis). Both leading
// and trailing '#' runs are removed (deviation #1 fixed: Python's CommonMark
// strips the closing '#' sequence too).
func headingRawText(n *ast.Heading, src []byte) string {
	raw := strings.TrimSpace(rawLines(n, src, "\n"))
	raw = strings.TrimLeft(raw, "#")
	raw = strings.TrimRight(raw, "#")
	return strings.TrimSpace(raw)
}

// rawLines joins a node's source lines (sep between lines). For fenced code
// the lines already carry their newlines (sep="").
func rawLines(n ast.Node, src []byte, sep string) string {
	lines := n.Lines()
	parts := make([]string, 0, lines.Len())
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		v := string(seg.Value(src))
		if sep != "" {
			v = strings.TrimRight(v, "\n")
		}
		parts = append(parts, v)
	}
	return strings.Join(parts, sep)
}

// renderList mirrors Renderer._render_List: each ListItem's children render
// (paragraph text / nested list) and the results flatten one level, so an
// item contributes its text and (if any) a nested item list.
func renderList(list *ast.List, src []byte) []any {
	var out []any
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			switch n := c.(type) {
			case *ast.List:
				out = append(out, renderList(n, src))
			default:
				out = append(out, rawLines(n, src, "\n"))
			}
		}
	}
	return out
}

// setOMap sets key k, replacing the value in place when the key repeats
// (OrderedDict semantics: position kept), else appending.
func setOMap(m OMap, k string, v any) OMap {
	if idx := indexOMap(m, k); idx >= 0 {
		m[idx].v = v
		return m
	}
	return append(m, kv{k, v})
}

func indexOMap(m OMap, k string) int {
	for i, e := range m {
		if e.k == k {
			return i
		}
	}
	return -1
}
