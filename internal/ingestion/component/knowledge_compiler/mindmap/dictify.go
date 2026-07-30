package mindmap

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Markdown-outline → mind-map-tree conversion, mirroring the Python pipeline:
// markdown_to_json.dictify (CommonMark nest + Renderer) → _todict/_list_to_kv
// → _merge (multi-batch) → final shaping + _be_children. Node ids keep the
// raw markdown text (asterisks included) until _key strips them, exactly like
// Python.

// kv is one ordered key/value pair; omap mirrors Python's OrderedDict (key
// order is load-bearing for tree shape and merge order).
type kv struct {
	k string
	v any
}
type omap []kv

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

// dictify mirrors markdown_to_json.dictify: nest top-level blocks by the
// MINIMUM top heading level; content before the first heading at each level
// is dropped; when a level has no headings the blocks become a list (the top
// level then renders as {"root": [...]}).
func dictify(md string) omap {
	src := []byte(md)
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))
	var blocks []mdBlock
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		blocks = append(blocks, toBlock(n, src))
	}
	if len(blocks) == 0 {
		return omap{}
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
	case omap:
		out := omap{}
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
		return omap{{"root", rendered}}
	}
	return omap{}
}

// dictifyBlocks mirrors CMarkASTNester._dictify_blocks: split blocks by
// headings at exactly `level`, recursing at level+1. Returns omap or the raw
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
	out := omap{}
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
	case omap:
		out := omap{}
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
// (Python str()'s the nested list — a pathological case we render readably).
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

// todict mirrors _todict + _list_to_kv: lists under dict keys convert to
// key→definition pairs ([prev scalar] → sublist[0]); a list with no
// definition sub-lists collapses to an empty dict (the bullets are dropped —
// faithful to Python). Recurses into nested dicts.
func todict(m omap) omap {
	for i := range m {
		switch val := m[i].v.(type) {
		case omap:
			m[i].v = todict(val)
		case []any:
			nv := omap{}
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

// mergeDicts mirrors _merge: d1 merges INTO d2 (ordered). Both dicts recurse;
// both lists concatenate d2's then d1's items; otherwise d1 wins. Keys absent
// from d2 append in d1's order.
func mergeDicts(d1, d2 omap) omap {
	for _, e := range d1 {
		idx := indexOMap(d2, e.k)
		if idx < 0 {
			d2 = append(d2, e)
			continue
		}
		dv := d2[idx].v
		d1Dict, d1IsDict := e.v.(omap)
		d2Dict, d2IsDict := dv.(omap)
		d1List, d1IsList := e.v.([]any)
		d2List, d2IsList := dv.([]any)
		switch {
		case d1IsDict && d2IsDict:
			d2[idx].v = mergeDicts(d1Dict, d2Dict)
		case d1IsList && d2IsList:
			d2[idx].v = append(d2List, d1List...)
		default:
			d2[idx].v = e.v
		}
	}
	return d2
}

// shapeTree mirrors __call__'s final shaping: >1 top-level keys → synthetic
// "root" node whose children are the dict-valued keys (with a keyset seeded
// by those keys); exactly 1 → that key becomes the root id. An empty merged
// dict degrades to a bare root (Python would raise IndexError on
// keys()[0]; the Go port keeps the pipeline alive).
func shapeTree(merged omap) *Node {
	if len(merged) > 1 {
		keyset := map[string]bool{}
		for _, e := range merged {
			if _, ok := e.v.(omap); !ok {
				continue
			}
			if k := stripStars(e.k); k != "" {
				keyset[k] = true
			}
		}
		var children []*Node
		for _, e := range merged {
			if _, ok := e.v.(omap); !ok {
				continue
			}
			k := stripStars(e.k)
			if k == "" {
				continue
			}
			children = append(children, &Node{ID: k, Children: beChildren(e.v, keyset)})
		}
		return &Node{ID: "root", Children: children}
	}
	if len(merged) == 1 {
		k := stripStars(merged[0].k)
		return &Node{ID: k, Children: beChildren(merged[0].v, map[string]bool{k: true})}
	}
	return &Node{ID: "root"}
}

// beChildren mirrors _be_children: strings/lists become leaf nodes; dict keys
// become nested nodes. keyset is SHARED across the whole subtree walk — a key
// already seen anywhere below is skipped (silent dedup, as in Python).
func beChildren(v any, keyset map[string]bool) []*Node {
	switch t := v.(type) {
	case string:
		keyset[t] = true
		if id := stripStars(t); id != "" {
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
			if id := stripStars(s); id != "" {
				out = append(out, &Node{ID: id})
			}
		}
		return out
	case omap:
		var out []*Node
		for _, e := range t {
			k := stripStars(e.k)
			if k == "" || keyset[k] {
				continue
			}
			keyset[k] = true
			out = append(out, &Node{ID: k, Children: beChildren(e.v, keyset)})
		}
		return out
	}
	return nil
}

// starsRE mirrors re.compile(r"\*+") used by _key.
var starsRE = regexp.MustCompile(`\*+`)

// stripStars mirrors _key (re.sub(r"\*+", "", k)).
func stripStars(s string) string {
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
// block.strings (goldmark's inline text would strip emphasis).
func headingRawText(n *ast.Heading, src []byte) string {
	raw := strings.TrimSpace(rawLines(n, src, "\n"))
	raw = strings.TrimLeft(raw, "#")
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
func setOMap(m omap, k string, v any) omap {
	if idx := indexOMap(m, k); idx >= 0 {
		m[idx].v = v
		return m
	}
	return append(m, kv{k, v})
}

func indexOMap(m omap, k string) int {
	for i, e := range m {
		if e.k == k {
			return i
		}
	}
	return -1
}
