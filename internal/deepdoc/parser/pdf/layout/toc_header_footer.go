package layout

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ---------------------------------------------------------------------------
// Tunable thresholds.
// ---------------------------------------------------------------------------

const (
	// tocMaxProseRunes is the per-box length (in runes) above which a box is
	// treated as a prose paragraph. A page with substantial body text has many
	// such boxes; a TOC page has at most a title or a long chapter heading.
	// This is the primary guard against deleting content: any page with more
	// than tocMaxLongBoxes long boxes is never classified as TOC.
	tocMaxProseRunes = 60
	// tocMaxLongBoxes is the maximum number of boxes longer than
	// tocMaxProseRunes a page may carry and still be classified as TOC.
	tocMaxLongBoxes = 2
	// tocMinShortBoxes is the minimum number of short boxes a TOC page carries.
	// A TOC is a list, so its entries usually stay separate boxes. A page whose
	// entries were merged into one long box is covered by tocMinMergedRuns, and
	// one merged without leader runs at all is covered by the outline signal.
	tocMinShortBoxes = 4
	// tocMinEntries is the minimum number of confirming entry markers (chapter
	// markers plus page numbers) a page must carry to be classified as TOC.
	tocMinEntries = 3
	// tocMinMergedRuns is the number of leader+page-number runs a single long
	// box must carry before that box is read as a whole TOC block rather than as
	// prose. A merged block confirms itself — three leader runs are three
	// entries — so it needs no separate marker count. Prose can accumulate the
	// occasional run, which is why the threshold is three and not one.
	tocMinMergedRuns = 3
	// tocMaxLeadPages bounds how many leading pages without body text may
	// precede the TOC. A cover, copyright page or frontispiece carries no body
	// text and is skipped, so a TOC that is not physically page one is still
	// reachable; the first page carrying body text ends the search.
	tocMaxLeadPages = 3
)

// ---------------------------------------------------------------------------
// Box-level patterns.
// ---------------------------------------------------------------------------

var (
	// chapterEntryPattern matches a box that opens like a TOC entry title: a
	// chapter/section/appendix marker (Chinese or Latin, digit or word form) or
	// a common front/back-matter heading.
	chapterEntryPattern = regexp.MustCompile(`(?i)^(第\s*[0-9０-９一二三四五六七八九十百千]+\s*[章节篇部册卷回讲目]|chapter\s+(\d+|[a-z]+|one|two|three|four|five|six|seven|eight|nine|ten)|appendix\s+[a-z0-9]+|section\s+\d+|part\s+(\d+|[ivxlcdm]+|[a-z]+)|前言|序言|引言|导论|绪论|后记|结语|附录|索引|参考文献|结论|致谢|跋|序|凡例|目录|acknowledgements?|acknowledgments?)`)
	// pageNumberPattern matches common page-number forms: pure digits, digits
	// with leading dots (..18), roman numerals (III, IV), or dash-wrapped
	// numbers (- 2 -). Full-width digits count as digits, matching the class
	// chapterEntryPattern uses for 第N章 numbering: the target corpus is largely
	// CJK, and a full-width page-number column still has to confirm a TOC page.
	pageNumberPattern = regexp.MustCompile(`^[.…]*[0-9０-９]+[.…]*$|^[.…]*[IVXLCDM]+[.…]*$|^\s*-\s*[0-9０-９]+\s*-\s*$`)
	// tocStartPattern matches an outline title that names the table of contents,
	// which is the only thing that starts a TOC page range. A heading that is
	// merely adjacent to a TOC — acknowledgements, references — does not: the
	// section-level detector this pass replaces accepted 致谢 here, so a book
	// without a 目录 bookmark had its acknowledgements page claimed as a TOC.
	tocStartPattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents)$`)
	// tocEndMarkerPattern matches the front/back-matter headings that are
	// neither a TOC nor an entry, so they cannot bound a TOC range. The
	// acknowledgements family is spelled out the way chapterEntryPattern spells
	// it (singular and plural, both spellings), because the pattern this
	// replaces only matched the bare word "acknowledge" and so never fired on
	// the usual "Acknowledgements" bookmark.
	tocEndMarkerPattern = regexp.MustCompile(`(?i)^(致谢|acknowledg(?:e|ements?|ments?))$`)
	// tocEntryRunPattern matches one TOC entry's tail: a leader run (two or more
	// dots, ellipses or middots, never wide spaces — those are too common in
	// prose ending in a number), a page number and an optional roman-numeral
	// fragment. Counting these runs tells a TOC block that upstream merged into
	// one long box apart from a paragraph. Digits accept the full-width forms
	// pageNumberPattern accepts, for the same reason.
	tocEntryRunPattern = regexp.MustCompile(`(\.{2,}|…{2,}|⋯{2,}|·{2,})\s*[0-9０-９]{1,4}\s*[IVXLCDMivxlcdm]{0,5}`)
)

// ---------------------------------------------------------------------------
// Outline signal.
// ---------------------------------------------------------------------------

// TOCPageRangeFromOutlines returns the 0-based page indices the PDF bookmarks
// identify as the table of contents: from the outline entry naming the TOC up
// to — excluding — the next entry at the same level. It returns nil when the
// document carries no usable bookmark.
//
// Only an entry that names the TOC itself starts a range (tocStartPattern).
// When a front/back-matter heading (tocEndMarkerPattern) is the next same-level
// entry the range is abandoned rather than closed, because such a heading does
// not say where the entries stop — it follows the TOC in some books and sits at
// the back of others — so the extent is left to the box shape signal instead of
// spanning the gap and deleting whatever lies inside it. Only the page the TOC
// starts on is claimed.
//
// Outline page numbers are 1-based — pdfium adds one to the 0-based destination
// index (internal/deepdoc/parser/pdf/pdfium/pdfium.go) — while
// TextBox.PageNumber is 0-based, because ParseRaw enumerates pages from zero.
// The conversion happens here, once, so no caller has to know about it.
func TOCPageRangeFromOutlines(outlines []pdf.Outline) map[int]bool {
	for i, o := range outlines {
		if !tocStartPattern.MatchString(outlineTitle(o.Title)) {
			continue
		}
		first := o.PageNumber - 1
		if first < 0 {
			return nil
		}
		for _, next := range outlines[i+1:] {
			if next.Level != o.Level {
				continue
			}
			if tocEndMarkerPattern.MatchString(outlineTitle(next.Title)) {
				break
			}
			pages := make(map[int]bool, 4)
			for pg := first; pg < next.PageNumber-1; pg++ {
				pages[pg] = true
			}
			if len(pages) == 0 {
				return nil
			}
			return pages
		}
		// No usable following entry at the same level: the TOC page is known
		// but its extent is not, so claim only that page and leave the rest to
		// the box shape signal.
		return map[int]bool{first: true}
	}
	return nil
}

// outlineTitle strips the "@@" anchor suffix pdfium appends to bookmark titles
// and lower-cases the result, matching how the section-level detector compared.
func outlineTitle(title string) string {
	title = strings.TrimSpace(title)
	if idx := strings.Index(title, "@@"); idx >= 0 {
		title = strings.TrimSpace(title[:idx])
	}
	return strings.ToLower(title)
}

// ---------------------------------------------------------------------------
// TOC removal.
// ---------------------------------------------------------------------------

// RemoveTOCBoxes drops whole pages that are detected as tables of contents.
//
// Detection works on signals that are destroyed by the later TextMerge /
// BoxesToSections passes (leader dots get folded into adjacent boxes, per-entry
// geometry collapses into a section), so this MUST run before TextMerge (see
// Parser.buildLayout).
//
// coversDocumentStart reports whether boxes cover the document's first page.
// The shape signal reads a TOC out of a leading run, so it is skipped when the
// parse began mid-document (Config.Pages): a TOC-shaped page at the start of a
// page range says nothing about the document, and reading it as a document
// prefix would delete content from the middle of the book. The outline signal
// carries absolute page numbers and is unaffected.
//
// Two signals select pages and the union is dropped:
//
//  1. outlinePages — the pages the PDF bookmarks identify as TOC (see
//     TOCPageRangeFromOutlines). Strongest, and the only one that still sees a
//     TOC whose entries were merged into one box with no leader runs left.
//  2. Box shape — the leading run of pages that read as a TOC list: a page
//     carrying at least tocMinShortBoxes short boxes and tocMinEntries
//     confirming markers (chapter markers or page numbers), or one long box
//     that is itself tocMinMergedRuns leader+page-number runs, preceded only by
//     pages without body text and spanning as many consecutive pages as keep
//     satisfying that shape. Kept for documents that carry no usable bookmark,
//     and only consulted when coversDocumentStart.
//
// Both signals pass through one guard: a page carrying body text (more than
// tocMaxLongBoxes boxes longer than tocMaxProseRunes) is never dropped, and a
// document consisting only of TOC pages is left untouched. The detector stays
// deliberately conservative — missing a TOC page costs noise chunks, deleting a
// content page loses text.
func RemoveTOCBoxes(boxes []pdf.TextBox, outlinePages map[int]bool, coversDocumentStart bool) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}

	perPage := make(map[int][]int, 64)
	for i := range boxes {
		p := boxes[i].PageNumber
		perPage[p] = append(perPage[p], i)
	}

	pages := make([]int, 0, len(perPage))
	for pg := range perPage {
		pages = append(pages, pg)
	}
	sort.Ints(pages)

	shapes := make(map[int]pageShape, len(pages))
	for _, pg := range pages {
		shapes[pg] = shapeOfPage(boxes, perPage[pg])
	}

	selected := make(map[int]bool, len(pages))
	for pg := range outlinePages {
		if _, ok := perPage[pg]; ok {
			selected[pg] = true
		}
	}

	// Heuristic fallback, for documents that carry no usable bookmark. A TOC is
	// a document prefix, so only a parse that covers the document's first page
	// is eligible, and then only the leading pages are candidates: pages without
	// body text (a cover, copyright page) may be skipped, and the run continues
	// while pages keep satisfying isTOC(). It ends on the first page that does
	// not — in practice the first page carrying body text, which is what keeps
	// per-chapter pages that happen to hold several short headings (the Daodejing
	// case) out of scope. The run is deliberately unbounded in length: a real
	// TOC can span more pages than any fixed cap would allow, and truncating it
	// silently keeps the remaining TOC pages in the output. Length is bounded
	// instead by isTOC() itself and by the all-TOC guard below.
	if coversDocumentStart {
		inTOC, lead := false, 0
		for _, pg := range pages {
			if inTOC {
				if !shapes[pg].isTOC() {
					break
				}
				selected[pg] = true
				continue
			}
			if shapes[pg].isTOC() {
				inTOC = true
				selected[pg] = true
				continue
			}
			if shapes[pg].carriesProse() || lead >= tocMaxLeadPages {
				break
			}
			lead++
		}
	}

	drop := make(map[int]struct{}, len(boxes))
	droppedPages := 0
	for _, pg := range pages {
		if !selected[pg] || shapes[pg].carriesProse() {
			continue
		}
		droppedPages++
		for _, i := range perPage[pg] {
			drop[i] = struct{}{}
		}
	}
	if droppedPages == 0 || droppedPages == len(pages) {
		return boxes
	}

	out := make([]pdf.TextBox, 0, len(boxes)-len(drop))
	for i := range boxes {
		if _, ok := drop[i]; ok {
			continue
		}
		out = append(out, boxes[i])
	}
	return out
}

// pageShape summarises the boxes of one page.
type pageShape struct {
	prose   int // boxes longer than tocMaxProseRunes
	short   int
	markers int  // short boxes opening with a chapter marker or holding a page number
	merged  bool // a long box carrying tocMinMergedRuns leader+page-number runs
}

func shapeOfPage(boxes []pdf.TextBox, indices []int) pageShape {
	var s pageShape
	for _, i := range indices {
		text := strings.TrimSpace(boxes[i].Text)
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > tocMaxProseRunes {
			s.prose++
			// A merged TOC block is long by definition, so the run count is the
			// only evidence it can carry. Cap the search: the answer is a
			// three-way comparison, not the exact number of runs.
			if len(tocEntryRunPattern.FindAllStringIndex(text, tocMinMergedRuns)) >= tocMinMergedRuns {
				s.merged = true
			}
			continue
		}
		s.short++
		if chapterEntryPattern.MatchString(text) || pageNumberPattern.MatchString(text) {
			s.markers++
		}
	}
	return s
}

// isTOC reports whether the page reads as a table of contents: a list of short
// boxes carrying entry markers, or one long box that is itself a run of
// entries, and no body text either way.
func (s pageShape) isTOC() bool {
	if s.carriesProse() {
		return false
	}
	return s.merged || (s.short >= tocMinShortBoxes && s.markers >= tocMinEntries)
}

// carriesProse reports whether the page carries body text. No signal may drop
// such a page: this is the single guard that bounds every removal.
func (s pageShape) carriesProse() bool { return s.prose > tocMaxLongBoxes }

// ---------------------------------------------------------------------------
// Running header / footer removal.
// ---------------------------------------------------------------------------

const (
	// headerZoneRatio / footerZoneRatio bound the top/bottom page zones where a
	// running header / footer lives (fractions of the page height).
	headerZoneRatio = 0.10
	footerZoneRatio = 0.90
	// headerMaxZoneRatio / footerMinZoneRatio define the expanded margin zones
	// when a clear whitespace gap separates the candidate from body content.
	headerMaxZoneRatio = 0.14
	footerMinZoneRatio = 0.86
	// minWhitespaceGapPt is the minimum vertical whitespace gap required to
	// justify expanding the header/footer zone beyond the base 10% ratio.
	minWhitespaceGapPt = 18.0
	// minHeaderFooterPages guards against cross-page false positives on short
	// documents. Deterministic page-number patterns still fire on shorter ones.
	minHeaderFooterPages = 3
	// minParityCoverageRatio is the minimum fraction of even or odd pages that
	// a running header/footer text must appear on to qualify under the parity track.
	//
	// In formal book typography (LaTeX book documentclass, InDesign book templates),
	// running headers are standardly suppressed on chapter opening pages (\thispagestyle{plain}),
	// blank verso pages before new chapters, and full-page illustrations/tables.
	// As a result, a running header in a real book often covers only 40%-48% of the
	// even or odd pages. A 0.40 threshold captures alternating bilateral book headers
	// while the evenPages >= 3 / oddPages >= 3 count guards against accidental matches.
	minParityCoverageRatio = 0.40
	// seqBandTopRatio / seqBandBottomRatio bound the wide bands in which the
	// sequence-backed page-number track looks for bare numeric / Roman-numeral
	// boxes. The bands are much looser than the header/footer zones because the
	// track's evidence is not geometric: only boxes that form an incrementing
	// run across consecutive pages at a stable Y are removed, which tight body
	// text cannot fake.
	seqBandTopRatio    = 0.20
	seqBandBottomRatio = 0.80
	// seqMinRun / seqMaxDy mirror the locality track: at least 3 consecutive
	// pages, total top-edge span across the run within 4pt.
	seqMinRun = 3
	seqMaxDy  = 4.0
	// promoCompanionMaxRunes bounds the length of a margin line that may be
	// learned as a running-header ad companion of a dropped site-promo box.
	// A piracy header is a short slogan, so anything longer is treated as body
	// text and never learned.
	promoCompanionMaxRunes = 24
	// minCompanionPages is how many distinct pages must carry the same band
	// peer of a dropped site-promo box before that peer's text is treated as a
	// running-header ad and propagated to every page. Requiring several pages
	// keeps a one-off line that merely shares a page with an ad intact.
	minCompanionPages = 3
)

var (
	// strictDecoratedPagePattern matches page numbers with clear formatting:
	// - Dash/dot/tilde-wrapped: - 1 -, — 1 —, – 1 –, · 1 ·, • 1 •, ~ 1 ~
	// - Bracket/paren-wrapped: [1], (1)
	// - "Page X", "Page X of Y", "p. X", "X / Y", "X of Y"
	// - Chinese: 第 1 页, 第 1 页 共 10 页, 第 1 页/共 10 页, 第一页, 第一页 共十页
	// - Roman with dashes: - iv -, — iv —, · iv ·
	strictDecoratedPagePattern = regexp.MustCompile(`(?i)^(\s*[-—–~·•]+\s*[0-9０-９]+\s*[-—–~·•]+|\s*\[\s*[0-9０-９]+\s*\]|\s*\(\s*[0-9０-９]+\s*\)|page\s+[0-9０-９]+(\s*(of|/)\s*[0-9０-９]+)?|p\.\s*[0-9０-９]+|[0-9０-９]+\s*/\s*[0-9０-９]+|[0-9０-９]+\s+of\s+[0-9０-９]+|第\s*[0-9０-９一二三四五六七八九十百]+\s*页(\s*(/\s*共|[,，]\s*共?|[/共])\s*[0-9０-９一二三四五六七八九十百]+\s*页?)?|\s*[-—–~·•]+\s*((l|xl|x{1,3})(ix|iv|v?i{1,3}|v)?|ix|iv|v?i{1,3}|v)\s*[-—–~·•]+)$`)

	// strictBareNumberPattern matches lone Arabic digits or Roman numerals: 1, 23, iv, VII.
	strictBareNumberPattern = regexp.MustCompile(`(?i)^([0-9０-９]{1,4}|((l|xl|x{1,3})(ix|iv|v?i{1,3}|v)?|ix|iv|v?i{1,3}|v))$`)

	// canonicalRomanPattern accepts the full canonical Roman-numeral grammar
	// (up to M) so the sequence track can parse page numbers beyond the XXX
	// range strictBareNumberPattern's header/footer idiom covers. Non-canonical
	// forms (IIII) are rejected by the pattern itself; romanValue only computes
	// the subtractive sum of a spelling this regex already accepted.
	canonicalRomanPattern = regexp.MustCompile(`(?i)^(m{0,3})(cm|cd|d?c{0,3})(xc|xl|l?x{0,3})(ix|iv|v?i{0,3})$`)

	// sitePromoPattern matches margin lines that advertise a download site —
	// a URL core with at most a short non-URL wrapper on either side. Such a
	// line is a running header/footer regardless of page count: piracy-supply
	// PDFs change the wrapper text on some pages ("更多的书籍免费下载 http://…",
	// "欢迎访问！http://…"), so those variants reach no recurrence track, while
	// the URL itself is deterministic evidence. Only whole-box, length-capped
	// boxes in the margin bands use it, so body prose containing a URL is never
	// touched.
	sitePromoPattern = regexp.MustCompile(`(?i)^.{0,20}(https?://\S+|www\.[\w-]+(\.[\w-]+)+|(forum|bbs|tieba|phpwind)\.[\w-]+\.[a-z]{2,}\S*|\S*fromuid=\S+).{0,20}$`)
)

// computePageGaps computes for candidate margin boxes on a page:
// - gapBelow: distance from b.Bottom to the nearest other box on the page with Top >= b.Bottom - 2.0.
// - gapAbove: distance from b.Top to the nearest other box on the page with Bottom <= b.Top + 2.0.
//
// Performance optimization: only boxes falling into the outer margin zones (top 14% or bottom 14%)
// ever query whitespace gaps. Skipping body-interior boxes reduces the outer loop from M to K (K <= 5),
// lowering complexity from O(M^2) to O(K*M) on dense multi-column pages.
func computePageGaps(boxes []pdf.TextBox, indices []int, pageHeight float64) (map[int]float64, map[int]float64) {
	gapBelow := make(map[int]float64, len(indices))
	gapAbove := make(map[int]float64, len(indices))

	for _, i := range indices {
		bi := boxes[i]
		if bi.Bottom > pageHeight*headerMaxZoneRatio && bi.Top < pageHeight*footerMinZoneRatio {
			continue
		}
		minDistBelow := pageHeight - bi.Bottom
		if minDistBelow < 0 {
			minDistBelow = 0
		}
		maxDistAbove := bi.Top
		if maxDistAbove < 0 {
			maxDistAbove = 0
		}

		for _, j := range indices {
			if i == j {
				continue
			}
			bj := boxes[j]
			if bj.Top >= bi.Bottom-2.0 {
				dist := bj.Top - bi.Bottom
				if dist < 0 {
					dist = 0
				}
				if dist < minDistBelow {
					minDistBelow = dist
				}
			}
			if bj.Bottom <= bi.Top+2.0 {
				dist := bi.Top - bj.Bottom
				if dist < 0 {
					dist = 0
				}
				if dist < maxDistAbove {
					maxDistAbove = dist
				}
			}
		}
		gapBelow[i] = minDistBelow
		gapAbove[i] = maxDistAbove
	}
	return gapBelow, gapAbove
}

// classifyZone determines whether a box sits in a header or footer zone.
func classifyZone(b pdf.TextBox, pageHeight float64, gapAbove, gapBelow float64) string {
	if b.Bottom <= pageHeight*headerZoneRatio {
		return "header"
	}
	if b.Bottom <= pageHeight*headerMaxZoneRatio && gapBelow >= minWhitespaceGapPt {
		return "header"
	}
	if b.Top >= pageHeight*footerZoneRatio {
		return "footer"
	}
	if b.Top >= pageHeight*footerMinZoneRatio && gapAbove >= minWhitespaceGapPt {
		return "footer"
	}
	return ""
}

// isNonTextLayout reports whether a layout type represents non-text elements
// like tables or figures that must not be treated as running headers/footers.
func isNonTextLayout(lt string) bool {
	lt = strings.TrimSpace(lt)
	return lt == "table" || lt == "figure" || lt == "equation" || lt == "image"
}

// parseDecimalValue parses a string of lone ASCII or full-width digits.
func parseDecimalValue(t string) (int, bool) {
	if t == "" {
		return 0, false
	}
	val := 0
	for _, r := range t {
		switch {
		case r >= '0' && r <= '9':
			val = val*10 + int(r-'0')
		case r >= '０' && r <= '９':
			val = val*10 + int(r-'０')
		default:
			return 0, false
		}
	}
	return val, true
}

// romanValue parses a canonical Roman numeral (I..MMMCMXCIX) and returns its
// value. The canonical pattern admits only canonical spellings, so non-canonical
// forms such as "IIII" are rejected before the subtractive sum is computed.
func romanValue(t string) (int, bool) {
	if !canonicalRomanPattern.MatchString(t) {
		return 0, false
	}
	values := map[rune]int{'m': 1000, 'd': 500, 'c': 100, 'l': 50, 'x': 10, 'v': 5, 'i': 1}
	val := 0
	prev := 0
	for _, r := range strings.ToLower(t) {
		v := values[r]
		if v == 0 {
			return 0, false
		}
		if v > prev {
			val += v - 2*prev
		} else {
			val += v
		}
		prev = v
	}
	if val <= 0 {
		return 0, false
	}
	return val, true
}

// parseBareNumberValue reports the numeric value of a box whose entire text is
// one bare number: lone digits (ASCII or full-width) or a canonical Roman
// numeral. "12", "４５", "iv", "XIV" parse; "12.", "- 3 -", "2024." do not.
func parseBareNumberValue(text string) (int, bool) {
	t := strings.TrimSpace(text)
	if v, ok := parseDecimalValue(t); ok {
		return v, true
	}
	return romanValue(t)
}

// pageNumberCeiling bounds which bare numeric values can be page numbers: a
// document of N pages never carries page number > N+5, and the floor of 20
// keeps short documents (years like 2024 on a 2-page footer) protected.
func pageNumberCeiling(numPages int) int {
	maxAllowed := numPages + 5
	if maxAllowed < 20 {
		maxAllowed = 20
	}
	return maxAllowed
}

// isSitePromo reports whether the whole box text advertises a download site.
func isSitePromo(text string) bool {
	t := strings.Join(strings.Fields(text), " ")
	if t == "" || utf8.RuneCountInString(t) > 60 {
		return false
	}
	return sitePromoPattern.MatchString(t)
}

// bandOf reports which expanded margin band ("header"/"footer") a box sits in,
// or "" when it is outside both. It uses the same zone bounds the site-promo
// tier applies, so a box learned as an ad companion and a box later matched
// against it are judged by one rule.
func bandOf(b pdf.TextBox, pageHeight float64) string {
	switch {
	case b.Bottom <= pageHeight*headerMaxZoneRatio:
		return "header"
	case b.Top >= pageHeight*footerMinZoneRatio:
		return "footer"
	}
	return ""
}

// promoSpot is the page and band where a site-promo box was dropped, recorded
// so the companion pass can harvest its in-band peers as running-header ads.
type promoSpot struct {
	page int
	band string
}

// promoCompanionDrops returns indices of margin-band boxes that are the slogan
// half of an advertisement already seen in the same band next to a dropped
// site-promo box. It harvests the short, non-title, in-band peers of every
// promo spot, then flags every band box whose collapsed text was harvested on
// at least minCompanionPages distinct pages — so a piracy header whose URL line
// is omitted or split on some pages still goes. The peers are keyed on
// collapseText (whitespace + case only, digits preserved) so two lines that
// differ only by a number are never merged. `skip` holds indices already doomed
// by an earlier tier and is consulted, never mutated.
func promoCompanionDrops(boxes []pdf.TextBox, perPage map[int][]int, pageHeights map[int]float64, skip map[int]struct{}, promoSpots []promoSpot) []int {
	companionPages := make(map[string]map[int]bool)
	for _, sp := range promoSpots {
		h := pageHeights[sp.page]
		for _, j := range perPage[sp.page] {
			if _, dropped := skip[j]; dropped {
				continue
			}
			b := boxes[j]
			if isNonTextLayout(b.LayoutType) || strings.TrimSpace(b.LayoutType) == "title" {
				continue
			}
			if bandOf(b, h) != sp.band {
				continue
			}
			key := collapseText(b.Text)
			if key == "" || utf8.RuneCountInString(key) > promoCompanionMaxRunes {
				continue
			}
			if companionPages[key] == nil {
				companionPages[key] = make(map[int]bool)
			}
			companionPages[key][sp.page] = true
		}
	}
	var out []int
	for key, pages := range companionPages {
		if len(pages) < minCompanionPages {
			continue
		}
		for i := range boxes {
			if _, dropped := skip[i]; dropped {
				continue
			}
			b := boxes[i]
			if isNonTextLayout(b.LayoutType) || strings.TrimSpace(b.LayoutType) == "title" {
				continue
			}
			h, ok := pageHeights[b.PageNumber]
			if !ok || h <= 0 {
				continue
			}
			if bandOf(b, h) != "" && collapseText(b.Text) == key {
				out = append(out, i)
			}
		}
	}
	return out
}

// isDeterministicPageNumber reports whether text is unambiguously a page number.
func isDeterministicPageNumber(text string, zone string, gapAbove, gapBelow float64, numPages int) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	if strictDecoratedPagePattern.MatchString(t) {
		return true
	}
	if strictBareNumberPattern.MatchString(t) {
		// Bare numbers must have clear vertical whitespace separation:
		// - in header: gapBelow >= 18pt to avoid dropping section headings/chapter numbers.
		// - in footer: gapAbove >= 18pt to avoid dropping footnote numbers, table cells, or trailing text.
		if zone == "header" && gapBelow < minWhitespaceGapPt {
			return false
		}
		if zone == "footer" && gapAbove < minWhitespaceGapPt {
			return false
		}
		// For bare numeric digits, verify the value is within a reasonable range of the document page count.
		// Protects isolated years (e.g. "2024"), IDs, or footnote indexes on short/medium documents.
		if val, ok := parseDecimalValue(t); ok {
			if val > pageNumberCeiling(numPages) {
				return false
			}
		}
		return true
	}
	return false
}

type boxMeta struct {
	idx  int
	page int
	top  float64
}

type zoneKey struct {
	zone string
	text string
}

// pageNumCand is one bare-number box inside a wide margin band, a candidate
// for the sequence-backed page-number track.
type pageNumCand struct {
	idx   int
	page  int
	value int
	top   float64
	band  string
}

// collectPageNumberCandidates gathers boxes whose entire text is one bare
// number (digits or Roman) sitting in the wide margin bands, bounded by the
// page-count ceiling. It walks EVERY box, including ones earlier tiers already
// dropped: the +1 chain must stay contiguous when the gap-backed tiers took
// the roomy pages and only the tight ones remain to this track.
func collectPageNumberCandidates(boxes []pdf.TextBox, pageHeights map[int]float64, ceiling int) []pageNumCand {
	var cands []pageNumCand
	for i := range boxes {
		b := boxes[i]
		if isNonTextLayout(b.LayoutType) {
			continue
		}
		h, ok := pageHeights[b.PageNumber]
		if !ok || h <= 0 {
			continue
		}
		band := ""
		switch {
		case b.Top >= h*seqBandBottomRatio:
			band = "footer"
		case b.Bottom <= h*seqBandTopRatio:
			band = "header"
		default:
			continue
		}
		val, ok := parseBareNumberValue(b.Text)
		if !ok || val > ceiling {
			continue
		}
		cands = append(cands, pageNumCand{idx: i, page: b.PageNumber, value: val, top: b.Top, band: band})
	}
	return cands
}

// findPageNumberSequenceDrops returns the indices of candidates that take part
// in a run of at least seqMinRun consecutive pages whose bare numbers step by
// exactly one (up or down) while the run's total top-edge span stays within
// seqMaxDy — the same full-window criterion the locality track applies, so a
// per-hop check never lets drift accumulate past the threshold. Such a run is
// a page-number series on sequence evidence alone — enough to remove tight
// footers the geometric zone gate rejected; isolated or non-stepping numbers
// are left alone.
func findPageNumberSequenceDrops(cands []pageNumCand) []int {
	byBand := make(map[string][]pageNumCand, 2)
	for _, c := range cands {
		byBand[c.band] = append(byBand[c.band], c)
	}
	dropped := make(map[int]bool, len(cands))
	for _, group := range byBand {
		if len(group) < seqMinRun {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].page != group[j].page {
				return group[i].page < group[j].page
			}
			return group[i].top < group[j].top
		})
		for _, dir := range []int{1, -1} {
			n := len(group)
			dp := make([]int, n)
			prev := make([]int, n)
			minTop := make([]float64, n)
			maxTop := make([]float64, n)
			for i := range group {
				dp[i], prev[i] = 1, -1
				minTop[i], maxTop[i] = group[i].top, group[i].top
			}
			for i := 0; i < n; i++ {
				for j := 0; j < i; j++ {
					if group[j].page != group[i].page-1 || group[j].value != group[i].value-dir {
						continue
					}
					newMin, newMax := minTop[j], maxTop[j]
					if group[i].top < newMin {
						newMin = group[i].top
					}
					if group[i].top > newMax {
						newMax = group[i].top
					}
					if newMax-newMin > seqMaxDy {
						continue
					}
					// Among equal-length chains prefer the tighter Y span, so a
					// wide early detour cannot block a later legal extension.
					if dp[j]+1 > dp[i] || (dp[j]+1 == dp[i] && newMax-newMin < maxTop[i]-minTop[i]) {
						dp[i] = dp[j] + 1
						prev[i] = j
						minTop[i], maxTop[i] = newMin, newMax
					}
				}
			}
			for i := 0; i < n; i++ {
				if dp[i] < seqMinRun {
					continue
				}
				for k := i; k >= 0; k = prev[k] {
					dropped[group[k].idx] = true
				}
			}
		}
	}
	out := make([]int, 0, len(dropped))
	for idx := range dropped {
		out = append(out, idx)
	}
	return out
}

// RemoveHeaderFooterBoxes drops boxes that are running headers or footers.
//
// It uses a 3D Cascaded Fusion scheme:
//  1. Tier 1: Deterministic Pattern & Semantic Tagging. Explicit DLA header/footer
//     tags in margin zones, unambiguously formatted page numbers, and margin
//     lines advertising a download site are removed immediately (even on 1- or
//     2-page documents).
//  2. Tier 2: Dual-Track Recurrence Engine. Evaluates recurrence across:
//     - Global half-pages threshold ((numPages + 1) / 2).
//     - Parity partition (even/odd) to eliminate bilateral alternating headers.
//     - Locality track (consecutive pages with stable Y) to remove chapter-varying headers.
//  3. Tier 3: Adaptive Whitespace Gap. Automatically extends the candidate zone
//     from 10% to up to 14% when separated from body content by >= 18pt of blank
//     space.
//  4. Sequence track: bare numeric / Roman-numeral boxes in wide margin bands
//     are removed when they step by one across >= 3 consecutive pages at a
//     stable Y, catching page numbers that sit too tight under body text for
//     the geometric gate in any tier above.
func RemoveHeaderFooterBoxes(boxes []pdf.TextBox, pageHeights map[int]float64) []pdf.TextBox {
	if len(pageHeights) == 0 || len(boxes) == 0 {
		return boxes
	}

	perPage := make(map[int][]int, len(pageHeights))
	for i := range boxes {
		perPage[boxes[i].PageNumber] = append(perPage[boxes[i].PageNumber], i)
	}

	allGapBelow := make(map[int]float64, len(boxes))
	allGapAbove := make(map[int]float64, len(boxes))
	for p, idxs := range perPage {
		h, ok := pageHeights[p]
		if !ok || h <= 0 {
			continue
		}
		gb, ga := computePageGaps(boxes, idxs, h)
		for idx, g := range gb {
			allGapBelow[idx] = g
		}
		for idx, g := range ga {
			allGapAbove[idx] = g
		}
	}

	numPages := len(pageHeights)
	drop := make(map[int]struct{}, len(boxes))
	// promoSpots records where a site-promo box was dropped in Tier 1 so the
	// companion pass can learn its band peers' text as running-header ads.
	var promoSpots []promoSpot

	// Tier 1: Deterministic page numbers & DLA semantic tags.
	for i := range boxes {
		b := boxes[i]
		if isNonTextLayout(b.LayoutType) {
			continue
		}
		h, ok := pageHeights[b.PageNumber]
		if !ok || h <= 0 {
			continue
		}

		// A URL ad in either expanded margin zone (top 14% / bottom 14%,
		// narrower than the sequence track's 20% bands) is an advertising
		// header/footer on sight — no whitespace gap and no recurrence required,
		// and this also fires on documents too short for the recurrence tracks.
		if isSitePromo(b.Text) && (b.Bottom <= h*headerMaxZoneRatio || b.Top >= h*footerMinZoneRatio) {
			common.Debug("header_footer: dropped by site promo",
				zap.Int("page", b.PageNumber), zap.Int("textLen", utf8.RuneCountInString(b.Text)))
			drop[i] = struct{}{}
			if b.Bottom <= h*headerMaxZoneRatio {
				promoSpots = append(promoSpots, promoSpot{page: b.PageNumber, band: "header"})
			} else {
				promoSpots = append(promoSpots, promoSpot{page: b.PageNumber, band: "footer"})
			}
			continue
		}

		zone := classifyZone(b, h, allGapAbove[i], allGapBelow[i])
		if zone == "" {
			continue
		}

		lt := strings.TrimSpace(b.LayoutType)
		if (zone == "header" && lt == "header") || (zone == "footer" && lt == "footer") {
			common.Debug("header_footer: dropped by DLA label",
				zap.Int("page", b.PageNumber), zap.String("zone", zone),
				zap.String("layoutType", lt), zap.Int("textLen", utf8.RuneCountInString(b.Text)))
			drop[i] = struct{}{}
			continue
		}

		if isDeterministicPageNumber(b.Text, zone, allGapAbove[i], allGapBelow[i], numPages) {
			common.Debug("header_footer: dropped by page-number pattern",
				zap.Int("page", b.PageNumber), zap.String("zone", zone),
				zap.Int("textLen", utf8.RuneCountInString(b.Text)))
			drop[i] = struct{}{}
			continue
		}
	}

	if numPages < minHeaderFooterPages {
		if len(drop) == 0 {
			return boxes
		}
		common.Debug("header_footer: removal completed (short document)",
			zap.Int("total_boxes", len(boxes)), zap.Int("dropped_boxes", len(drop)))
		return applyDrop(boxes, drop)
	}

	// Promo-companion propagation. Runs after Tier 1 and before the recurrence
	// engine, so the boxes it removes never enter the recurrence statistics
	// (which already skip dropped indices) and cannot perturb any "#"-masked count.
	for _, idx := range promoCompanionDrops(boxes, perPage, pageHeights, drop, promoSpots) {
		common.Debug("header_footer: dropped by promo companion",
			zap.Int("page", boxes[idx].PageNumber), zap.String("zone", bandOf(boxes[idx], pageHeights[boxes[idx].PageNumber])), zap.Int("textLen", utf8.RuneCountInString(boxes[idx].Text)))
		drop[idx] = struct{}{}
	}

	// Tier 2: Dual-Track Recurrence Engine.
	keyMetas := make(map[zoneKey][]boxMeta)
	for i := range boxes {
		if _, ok := drop[i]; ok {
			continue
		}
		b := boxes[i]
		if isNonTextLayout(b.LayoutType) {
			continue
		}
		h, ok := pageHeights[b.PageNumber]
		if !ok || h <= 0 {
			continue
		}
		zone := classifyZone(b, h, allGapAbove[i], allGapBelow[i])
		if zone == "" {
			continue
		}
		norm := normalizeRunningText(b.Text)
		if norm == "" {
			continue
		}
		key := zoneKey{zone: zone, text: norm}
		keyMetas[key] = append(keyMetas[key], boxMeta{idx: i, page: b.PageNumber, top: b.Top})
	}

	evenTotal, oddTotal := 0, 0
	for p := range pageHeights {
		if p%2 == 0 {
			evenTotal++
		} else {
			oddTotal++
		}
	}

	minGlobalPages := (numPages + 1) / 2
	if minGlobalPages < 2 {
		minGlobalPages = 2
	}

	for key, metas := range keyMetas {
		pageSet := make(map[int]bool, len(metas))
		evenPages, oddPages := 0, 0
		for _, m := range metas {
			if !pageSet[m.page] {
				pageSet[m.page] = true
				if m.page%2 == 0 {
					evenPages++
				} else {
					oddPages++
				}
			}
		}

		distinctPages := len(pageSet)
		var reason string

		// 1. Global coverage: at least half the pages (rounded up).
		if distinctPages >= minGlobalPages {
			reason = "global_frequency"
		} else if evenTotal >= 3 && evenPages >= 3 && float64(evenPages)/float64(evenTotal) >= minParityCoverageRatio {
			// 2. Parity partition: at least minParityCoverageRatio of even pages.
			reason = "parity_even"
		} else if oddTotal >= 3 && oddPages >= 3 && float64(oddPages)/float64(oddTotal) >= minParityCoverageRatio {
			// 2. Parity partition: at least minParityCoverageRatio of odd pages.
			reason = "parity_odd"
		}

		if reason != "" {
			common.Debug("header_footer: dropped by recurrence",
				zap.String("zone", key.zone), zap.Int("textLen", utf8.RuneCountInString(key.text)),
				zap.String("reason", reason), zap.Int("pages", distinctPages))
			for _, m := range metas {
				drop[m.idx] = struct{}{}
			}
			continue
		}

		// 3. Locality track: stable consecutive runs of at least 3 pages with stable Y.
		// Only drop occurrences that actually belong to qualifying consecutive runs,
		// preventing distant isolated occurrences of the same text from being deleted.
		if len(metas) >= 3 && utf8.RuneCountInString(key.text) <= 60 {
			runIndices := findStableConsecutiveRunIndices(metas, 3, 4.0)
			if len(runIndices) > 0 {
				common.Debug("header_footer: dropped by recurrence",
					zap.String("zone", key.zone), zap.Int("textLen", utf8.RuneCountInString(key.text)),
					zap.String("reason", "local_consecutive_run"), zap.Int("boxes", len(runIndices)))
				for _, idx := range runIndices {
					drop[idx] = struct{}{}
				}
			}
		}
	}

	// Sequence-backed page numbers. Word-derived books sit their page numbers
	// a few points under body text — below the 90% footer line AND below the
	// expanded band's whitespace requirement — so the geometric gate rejects
	// them before any tier can see a pattern or count a recurrence. A run of
	// bare numbers stepping by one across consecutive pages at a stable Y is a
	// page-number series on sequence evidence alone. This runs after the
	// recurrence engine so it only adds drops: the recurrence counts, which
	// include these boxes under their masked key, stay untouched.
	if seqDrops := findPageNumberSequenceDrops(collectPageNumberCandidates(boxes, pageHeights, pageNumberCeiling(numPages))); len(seqDrops) > 0 {
		common.Debug("header_footer: dropped by page-number sequence", zap.Int("boxes", len(seqDrops)))
		for _, idx := range seqDrops {
			drop[idx] = struct{}{}
		}
	}

	if len(drop) == 0 {
		return boxes
	}
	common.Debug("header_footer: removal completed",
		zap.Int("total_boxes", len(boxes)), zap.Int("dropped_boxes", len(drop)))
	return applyDrop(boxes, drop)
}

// findStableConsecutiveRunIndices finds all box indices that belong to any qualifying
// consecutive run of at least minRun pages where the top variation is within maxDy.
// Restricting removal to these specific indices ensures isolated occurrences outside
// the run (e.g. on later pages) are not accidentally dropped.
func findStableConsecutiveRunIndices(metas []boxMeta, minRun int, maxDy float64) []int {
	if len(metas) < minRun {
		return nil
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].page != metas[j].page {
			return metas[i].page < metas[j].page
		}
		return metas[i].top < metas[j].top
	})

	type pageOccurrence struct {
		page    int
		top     float64
		indices []int
	}
	var distinct []pageOccurrence
	for _, m := range metas {
		if len(distinct) == 0 || distinct[len(distinct)-1].page != m.page {
			distinct = append(distinct, pageOccurrence{page: m.page, top: m.top, indices: []int{m.idx}})
		} else {
			distinct[len(distinct)-1].indices = append(distinct[len(distinct)-1].indices, m.idx)
		}
	}

	if len(distinct) < minRun {
		return nil
	}

	qualifyingPages := make(map[int]bool)

	left := 0
	for right := 0; right < len(distinct); right++ {
		// Reset window if consecutive page sequence is broken
		if right > 0 && distinct[right].page != distinct[right-1].page+1 {
			left = right
		}
		// Check all qualifying consecutive windows ending at right
		for (right - left + 1) >= minRun {
			curMin := distinct[left].top
			curMax := distinct[left].top
			for k := left + 1; k <= right; k++ {
				if distinct[k].top < curMin {
					curMin = distinct[k].top
				}
				if distinct[k].top > curMax {
					curMax = distinct[k].top
				}
			}
			if (curMax - curMin) <= maxDy {
				for k := left; k <= right; k++ {
					qualifyingPages[distinct[k].page] = true
				}
				break
			}
			left++
		}
	}

	if len(qualifyingPages) == 0 {
		return nil
	}

	var result []int
	for _, p := range distinct {
		if qualifyingPages[p.page] {
			result = append(result, p.indices...)
		}
	}
	return result
}

// applyDrop filters out the dropped boxes and returns the survivors.
func applyDrop(boxes []pdf.TextBox, drop map[int]struct{}) []pdf.TextBox {
	if len(drop) == 0 {
		return boxes
	}
	out := make([]pdf.TextBox, 0, len(boxes)-len(drop))
	for i := range boxes {
		if _, ok := drop[i]; ok {
			continue
		}
		out = append(out, boxes[i])
	}
	return out
}

// collapseText lower-cases and whitespace-collapses text without masking
// digits, for comparison keys that must not treat two differently-numbered
// lines as identical (unlike normalizeRunningText, which is for running
// headers/footers whose per-page number must be folded away).
func collapseText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// normalizeRunningText collapses whitespace, replaces digit runs with a single
// '#' and lower-cases the result, so per-page variants of the same running
// header/footer ("- 2 -", "- 3 -", "Page 4 of 9", "第 １ 页") share one
// comparison key.
func normalizeRunningText(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	out := make([]rune, 0, len(t))
	inDigit := false
	for _, r := range t {
		if r == '—' || r == '–' || r == '－' {
			r = '-'
		}
		if isMaskableDigit(r) {
			inDigit = true
			continue
		}
		if inDigit {
			out = append(out, '#')
			inDigit = false
		}
		out = append(out, r)
	}
	if inDigit {
		out = append(out, '#')
	}
	return strings.ToLower(strings.TrimSpace(string(out)))
}

// isMaskableDigit reports whether r is a digit whose per-page value must be
// masked before running headers/footers are compared: ASCII and full-width
// decimal digits. Full width is required for the same reason pageNumberPattern
// accepts it — otherwise every page of "第 １ 页" / "第 ２ 页" gets its own key
// and the footer never reaches minPages.
func isMaskableDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= '０' && r <= '９')
}
