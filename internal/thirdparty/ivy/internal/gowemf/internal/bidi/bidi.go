// Copied from github.com/mgilbir/gowemf, file internal/bidi/bidi.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

// Package bidi implements the Unicode Bidirectional Algorithm (UAX #9) for one
// line of text: resolving embedding levels (rules P2–P3, X1–X10, W1–W7,
// N0–N2, I1–I2), resetting trailing whitespace (L1) and reordering (L2).
// Shaping, mirroring (L4) and the placement of combining marks (L3) are left
// to the renderer. It is written from UAX #9 revision 52 (Unicode 18.0.0) and
// checked against the Unicode conformance tests under make test-external.
package bidi

import "sort"

// Class is a Bidi_Class property value.
type Class uint8

const (
	L Class = iota
	R
	AL
	EN
	ES
	ET
	AN
	CS
	NSM
	BN
	B
	S
	WS
	ON
	LRE
	LRO
	RLE
	RLO
	PDF
	LRI
	RLI
	FSI
	PDI
)

var classNames = [...]string{"L", "R", "AL", "EN", "ES", "ET", "AN", "CS", "NSM", "BN", "B", "S", "WS", "ON", "LRE", "LRO", "RLE", "RLO", "PDF", "LRI", "RLI", "FSI", "PDI"}

func (c Class) String() string {
	if int(c) < len(classNames) {
		return classNames[c]
	}
	return "invalid"
}

// ParseClass returns the class with a short property value alias.
func ParseClass(s string) (Class, bool) {
	for i, n := range classNames {
		if n == s {
			return Class(i), true
		}
	}
	return 0, false
}

// Lookup returns the Bidi_Class of r; values outside Unicode are L.
func Lookup(r rune) Class {
	if r < 0 || r > 0x10ffff {
		return L
	}
	i := sort.Search(len(classStarts), func(i int) bool { return classStarts[i] > uint32(r) }) - 1
	return classValues[i]
}

// Bracket identifies a paired bracket for rule N0. Kind is BracketOpen or
// BracketClose; Key is the opening bracket of the pair, so an opening and a
// closing bracket pair up when their keys are equal.
type Bracket struct {
	Kind uint8
	Key  rune
}

const (
	BracketNone uint8 = iota
	BracketOpen
	BracketClose
)

// LookupBracket returns r's Bidi_Paired_Bracket_Type and pairing key. The
// canonically equivalent angle brackets U+2329/U+232A and U+3008/U+3009 share
// a key (UAX #9 BD16).
func LookupBracket(r rune) Bracket {
	canonical := func(r rune) rune {
		switch r {
		case 0x2329:
			return 0x3008
		case 0x232a:
			return 0x3009
		}
		return r
	}
	i := sort.Search(len(bracketPairs), func(i int) bool { return bracketPairs[i][0] >= r })
	if i < len(bracketPairs) && bracketPairs[i][0] == r {
		return Bracket{BracketOpen, canonical(r)}
	}
	i = sort.Search(len(closingBrackets), func(i int) bool { return closingBrackets[i][0] >= r })
	if i < len(closingBrackets) && closingBrackets[i][0] == r {
		return Bracket{BracketClose, canonical(closingBrackets[i][1])}
	}
	return Bracket{}
}

// Direction selects the paragraph embedding level.
type Direction uint8

const (
	LeftToRight Direction = iota // level 0
	RightToLeft                  // level 1
	Auto                         // rules P2 and P3, defaulting to level 0
)

// Removed is the level Resolve reports for characters removed by rule X9.
const Removed int8 = -1

const maxDepth = 125

func isolateInitiator(c Class) bool { return c == LRI || c == RLI || c == FSI }

func removedByX9(c Class) bool {
	switch c {
	case RLE, LRE, RLO, LRO, PDF, BN:
		return true
	}
	return false
}

// matchingPDIs returns, for each isolate initiator, the index of its matching
// PDI (BD9), or len(classes) when it has none; other entries are -1.
func matchingPDIs(classes []Class) []int {
	match := make([]int, len(classes))
	var open []int
	for i, c := range classes {
		match[i] = -1
		switch {
		case isolateInitiator(c):
			open = append(open, i)
		case c == PDI && len(open) > 0:
			match[open[len(open)-1]] = i
			open = open[:len(open)-1]
		}
	}
	for _, i := range open {
		match[i] = len(classes)
	}
	return match
}

// firstStrong applies rules P2 and P3 to classes[start:end]: 1 if the first
// strong character outside isolates is R or AL, otherwise 0.
func firstStrong(classes []Class, match []int, start, end int) int8 {
	for i := start; i < end; i++ {
		switch c := classes[i]; {
		case c == L:
			return 0
		case c == R || c == AL:
			return 1
		case isolateInitiator(c):
			i = match[i] // skip to the matching PDI; the loop steps past it
		}
	}
	return 0
}

// Resolve returns the embedding level of each character of one paragraph
// (which may end with a paragraph separator), after rule L1 for a paragraph
// displayed on one line, and the paragraph level. Characters removed by rule
// X9 get Removed. brackets may be nil, in which case no character is a paired
// bracket.
func Resolve(classes []Class, brackets []Bracket, dir Direction) ([]int8, int8) {
	n := len(classes)
	match := matchingPDIs(classes)
	var para int8
	switch dir {
	case RightToLeft:
		para = 1
	case Auto:
		para = firstStrong(classes, match, 0, n)
	}
	types := append([]Class(nil), classes...)
	levels := make([]int8, n)

	// X1–X8: explicit levels and directions.
	type entry struct {
		level    int8
		override Class // ON when neutral, otherwise L or R
		isolate  bool
	}
	stack := []entry{{para, ON, false}}
	overflowIsolates, overflowEmbeddings, validIsolates := 0, 0, 0
	next := func(odd bool) int8 {
		l := stack[len(stack)-1].level + 1
		if (l%2 == 1) != odd {
			l++
		}
		return l
	}
	for i, c := range classes {
		top := stack[len(stack)-1]
		switch c {
		case RLE, LRE, RLO, LRO:
			levels[i] = top.level
			l := next(c == RLE || c == RLO)
			if l <= maxDepth && overflowIsolates == 0 && overflowEmbeddings == 0 {
				o := ON
				if c == RLO {
					o = R
				} else if c == LRO {
					o = L
				}
				stack = append(stack, entry{l, o, false})
			} else if overflowIsolates == 0 {
				overflowEmbeddings++
			}
		case RLI, LRI, FSI:
			levels[i] = top.level
			if top.override != ON {
				types[i] = top.override
			}
			rtl := c == RLI
			if c == FSI {
				rtl = firstStrong(classes, match, i+1, match[i]) == 1
			}
			l := next(rtl)
			if l <= maxDepth && overflowIsolates == 0 && overflowEmbeddings == 0 {
				validIsolates++
				stack = append(stack, entry{l, ON, true})
			} else {
				overflowIsolates++
			}
		case PDI:
			if overflowIsolates > 0 {
				overflowIsolates--
			} else if validIsolates > 0 {
				overflowEmbeddings = 0
				for !stack[len(stack)-1].isolate {
					stack = stack[:len(stack)-1]
				}
				stack = stack[:len(stack)-1]
				validIsolates--
			}
			top = stack[len(stack)-1]
			levels[i] = top.level
			if top.override != ON {
				types[i] = top.override
			}
		case PDF:
			levels[i] = top.level
			if overflowIsolates > 0 {
			} else if overflowEmbeddings > 0 {
				overflowEmbeddings--
			} else if !top.isolate && len(stack) >= 2 {
				stack = stack[:len(stack)-1]
			}
		case B:
			levels[i] = para // X8
		case BN:
			levels[i] = top.level
		default: // X6
			levels[i] = top.level
			if top.override != ON {
				types[i] = top.override
			}
		}
	}

	// X9: the remaining rules skip removed characters.
	var kept []int
	for i, c := range classes {
		if !removedByX9(c) {
			kept = append(kept, i)
		}
	}

	// X10 and BD13: level runs, chained across isolates into isolating run
	// sequences.
	var runs [][]int
	runOf := make([]int, n)
	for k, i := range kept {
		if k == 0 || levels[i] != levels[kept[k-1]] {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], i)
		runOf[i] = len(runs) - 1
	}
	prevKept, nextKept := make([]int, n), make([]int, n)
	for i, last := 0, -1; i < n; i++ {
		prevKept[i] = last
		if !removedByX9(classes[i]) {
			last = i
		}
	}
	for i, after := n-1, n; i >= 0; i-- {
		nextKept[i] = after
		if !removedByX9(classes[i]) {
			after = i
		}
	}
	// chain[r] is the run continuing run r across an isolate: the run that
	// starts with the matching PDI of the isolate initiator ending r.
	chain := make([]int, len(runs))
	continues := make([]bool, len(runs))
	for r, run := range runs {
		chain[r] = -1
		last := run[len(run)-1]
		if m := match[last]; isolateInitiator(classes[last]) && m < n && runs[runOf[m]][0] == m {
			chain[r] = runOf[m]
			continues[runOf[m]] = true
		}
	}
	// Sequence boundaries use the explicit levels, before rules I1 and I2
	// change any of them.
	explicit := append([]int8(nil), levels...)
	for r := range runs {
		if continues[r] {
			continue
		}
		var seq []int
		for ; r >= 0; r = chain[r] {
			seq = append(seq, runs[r]...)
		}
		resolveSequence(seq, classes, types, brackets, explicit, levels, para, prevKept, nextKept)
	}

	// L1: separators, and whitespace and isolate formatting characters
	// before them or at the end of the line, take the paragraph level.
	trailing := true
	for i := n - 1; i >= 0; i-- {
		switch c := classes[i]; {
		case c == S || c == B:
			levels[i] = para
			trailing = true
		case c == WS || isolateInitiator(c) || c == PDI:
			if trailing {
				levels[i] = para
			}
		case removedByX9(c):
		default:
			trailing = false
		}
	}
	for i, c := range classes {
		if removedByX9(c) {
			levels[i] = Removed
		}
	}
	return levels, para
}

// resolveSequence applies rules W1–W7, N0–N2 and I1–I2 to one isolating run
// sequence, given as character indexes in logical order.
func resolveSequence(seq []int, classes, types []Class, brackets []Bracket, explicit, levels []int8, para int8, prevKept, nextKept []int) {
	n := len(classes)
	level := explicit[seq[0]]
	direction := func(l int8) Class {
		if l%2 == 1 {
			return R
		}
		return L
	}
	before := para
	if p := prevKept[seq[0]]; p >= 0 {
		before = explicit[p]
	}
	last := seq[len(seq)-1]
	after := para
	if nx := nextKept[last]; nx < n && !isolateInitiator(classes[last]) {
		after = explicit[nx]
	}
	sos, eos := direction(max(before, level)), direction(max(after, explicit[last]))
	e := direction(level)
	t := make([]Class, len(seq))
	for k, i := range seq {
		t[k] = types[i]
	}

	// W1: nonspacing marks take the type of the preceding character.
	for k := range t {
		if t[k] != NSM {
			continue
		}
		switch {
		case k == 0:
			t[k] = sos
		case isolateInitiator(t[k-1]) || t[k-1] == PDI:
			t[k] = ON
		default:
			t[k] = t[k-1]
		}
	}
	// W2: European numbers after Arabic letters are Arabic numbers.
	strong := sos
	for k, c := range t {
		switch c {
		case L, R, AL:
			strong = c
		case EN:
			if strong == AL {
				t[k] = AN
			}
		}
	}
	// W3.
	for k, c := range t {
		if c == AL {
			t[k] = R
		}
	}
	// W4: single separators between numbers.
	for k := 1; k+1 < len(t); k++ {
		switch {
		case t[k] == ES && t[k-1] == EN && t[k+1] == EN:
			t[k] = EN
		case t[k] == CS && t[k-1] == EN && t[k+1] == EN:
			t[k] = EN
		case t[k] == CS && t[k-1] == AN && t[k+1] == AN:
			t[k] = AN
		}
	}
	// W5: terminators adjacent to European numbers.
	for k := 0; k < len(t); {
		if t[k] != ET {
			k++
			continue
		}
		end := k
		for end < len(t) && t[end] == ET {
			end++
		}
		if (k > 0 && t[k-1] == EN) || (end < len(t) && t[end] == EN) {
			for j := k; j < end; j++ {
				t[j] = EN
			}
		}
		k = end
	}
	// W6.
	for k, c := range t {
		if c == ES || c == ET || c == CS {
			t[k] = ON
		}
	}
	// W7: European numbers after left-to-right text are L.
	strong = sos
	for k, c := range t {
		switch c {
		case L, R:
			strong = c
		case EN:
			if strong == L {
				t[k] = L
			}
		}
	}

	// N0: paired brackets.
	if brackets != nil {
		resolveBrackets(seq, classes, brackets, t, sos, e)
	}

	// N1 and N2: neutrals and isolate formatting characters.
	ni := func(c Class) bool {
		switch c {
		case B, S, WS, ON, LRI, RLI, FSI, PDI:
			return true
		}
		return false
	}
	strongOf := func(c Class) Class {
		if c == EN || c == AN {
			return R
		}
		return c
	}
	for k := 0; k < len(t); {
		if !ni(t[k]) {
			k++
			continue
		}
		end := k
		for end < len(t) && ni(t[end]) {
			end++
		}
		lead, trail := sos, eos
		if k > 0 {
			lead = strongOf(t[k-1])
		}
		if end < len(t) {
			trail = strongOf(t[end])
		}
		d := e
		if lead == trail {
			d = lead
		}
		for j := k; j < end; j++ {
			t[j] = d
		}
		k = end
	}

	// I1 and I2.
	for k, i := range seq {
		switch c := t[k]; {
		case level%2 == 0 && c == R:
			levels[i] = level + 1
		case level%2 == 0 && (c == AN || c == EN):
			levels[i] = level + 2
		case level%2 == 1 && (c == L || c == EN || c == AN):
			levels[i] = level + 1
		default:
			levels[i] = level
		}
	}
}

// resolveBrackets applies rule N0 to the types t of an isolating run sequence.
func resolveBrackets(seq []int, classes []Class, brackets []Bracket, t []Class, sos, e Class) {
	// BD16: pair brackets with a stack of at most 63 openers.
	type opener struct {
		key rune
		pos int
	}
	var stack []opener
	var pairs [][2]int
	for k, i := range seq {
		b := brackets[i]
		if t[k] != ON || b.Kind == BracketNone {
			continue
		}
		if b.Kind == BracketOpen {
			if len(stack) == 63 {
				return
			}
			stack = append(stack, opener{b.Key, k})
			continue
		}
		for s := len(stack) - 1; s >= 0; s-- {
			if stack[s].key == b.Key {
				pairs = append(pairs, [2]int{stack[s].pos, k})
				stack = stack[:s]
				break
			}
		}
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a][0] < pairs[b][0] })
	strongOf := func(c Class) Class {
		if c == EN || c == AN {
			return R
		}
		return c
	}
	opposite := L
	if e == L {
		opposite = R
	}
	for _, p := range pairs {
		var foundE, foundOpposite bool
		for k := p[0] + 1; k < p[1]; k++ {
			switch strongOf(t[k]) {
			case e:
				foundE = true
			case opposite:
				foundOpposite = true
			}
		}
		var d Class
		switch {
		case foundE:
			d = e
		case foundOpposite:
			context := sos
			for k := p[0] - 1; k >= 0; k-- {
				if c := strongOf(t[k]); c == L || c == R {
					context = c
					break
				}
			}
			d = e
			if context == opposite {
				d = opposite
			}
		default:
			continue
		}
		for _, k := range p {
			t[k] = d
			// Original nonspacing marks after a resolved bracket follow it.
			for j := k + 1; j < len(t) && classes[seq[j]] == NSM; j++ {
				t[j] = d
			}
		}
	}
}

// Reorder applies rule L2 to the levels of one line and returns the indexes of
// the characters from left to right. Characters at level Removed are omitted.
func Reorder(levels []int8) []int {
	var order []int
	var highest, lowestOdd int8 = 0, maxDepth + 2
	for i, l := range levels {
		if l == Removed {
			continue
		}
		order = append(order, i)
		highest = max(highest, l)
		lowestOdd = min(lowestOdd, l|1)
	}
	for l := highest; l >= lowestOdd; l-- {
		for k := 0; k < len(order); {
			if levels[order[k]] < l {
				k++
				continue
			}
			end := k
			for end < len(order) && levels[order[end]] >= l {
				end++
			}
			for a, b := k, end-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			k = end
		}
	}
	return order
}
