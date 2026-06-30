package post

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
)

// ── Config ─────────────────────────────────────────────────────────────

// Config keys for PipelineConfig.
const (
	ConfigKeyPageWidth          = "page_width"
	ConfigKeyZoom               = "zoom"
	ConfigKeyOutlines           = "outlines"
	ConfigKeyFlattenMediaToText = "flatten_media_to_text"
	ConfigKeyTenantID           = "tenant_id"
	ConfigKeyVLMLLMID           = "vlm_llm_id"
	ConfigKeyRemoveTOC          = "remove_toc"
)

// PipelineConfig is a key-value map that post-processing reads
// to obtain its parameters.
type PipelineConfig map[string]interface{}

// Float64 returns the float64 value for key, or default_ if absent or wrong type.
func (c PipelineConfig) Float64(key string, default_ float64) float64 {
	if c == nil {
		return default_
	}
	v, ok := c[key]
	if !ok {
		return default_
	}
	f, ok := v.(float64)
	if !ok {
		return default_
	}
	return f
}

// Bool returns the bool value for key. Returns false if absent or wrong type.
func (c PipelineConfig) Bool(key string) bool {
	if c == nil {
		return false
	}
	v, ok := c[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// Outlines returns the []pdftype.Outline value for ConfigKeyOutlines.
func (c PipelineConfig) Outlines() []pdftype.Outline {
	if c == nil {
		return nil
	}
	v, ok := c[ConfigKeyOutlines]
	if !ok {
		return nil
	}
	o, ok := v.([]pdftype.Outline)
	if !ok {
		return nil
	}
	return o
}

// String returns the string value for key. Returns "" if absent or wrong type.
func (c PipelineConfig) String(key string) string {
	if c == nil {
		return ""
	}
	v, ok := c[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// ── Patterns ───────────────────────────────────────────────────────────

// headerFooterPattern matches layout types that should be treated as
// page furniture (Python: r"(header|footer|number)" in parser.py:637).
var headerFooterPattern = regexp.MustCompile(`(header|footer|number|reference)`)

// tocTitlePattern matches outline titles that mark a table-of-contents page.
// Python: r"(contents|目录|目次|table of contents|致谢|acknowledge)$"
var tocTitlePattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents|致谢|acknowledge)$`)

// ── PostProcess ────────────────────────────────────────────────────────

// PostProcess applies PDF post-processing to a ParseResult in-place.
// The config map controls which features to enable.
//
// Execution order (matches Python _pdf):
//  1. reorderMultiColumn — if page_width > 0
//  2. removeTOCByOutlines — if outlines present
//  3. normalizeLayoutType — always
//  4. filterHeaderFooter — always
//  5. assignDocTypeKwd — always (respects flatten_media_to_text)
//  6. enhanceWithVision — if image_describer present
func PostProcess(ctx context.Context, result *pdftype.ParseResult, config PipelineConfig) error {
	if result == nil {
		return errors.New("PostProcess: nil result")
	}
	if config == nil {
		config = PipelineConfig{}
	}

	// 1. Multi-column reorder
	pw := config.Float64(ConfigKeyPageWidth, 0)
	if pw > 0 {
		zoom := config.Float64(ConfigKeyZoom, 1.0)
		if zoom <= 0 {
			zoom = 1.0
		}
		reorderMultiColumn(result, pw, zoom)
	}

	// 2. Remove TOC pages (only when explicitly enabled).
	// Outlines from config take precedence; otherwise read from ParseResult.
	outlines := config.Outlines()
	if len(outlines) == 0 {
		outlines = result.Outlines
	}
	if config.Bool(ConfigKeyRemoveTOC) && len(outlines) > 0 {
		removeTOCByOutlines(result, outlines)
	}

	// 3-5. Always-on steps
	normalizeLayoutType(result)
	filterHeaderFooter(result)
	assignDocTypeKwd(result, config.Bool(ConfigKeyFlattenMediaToText))

	// 6. VLM enhancement
	tenantID := config.String(ConfigKeyTenantID)
	vlmLLMID := config.String(ConfigKeyVLMLLMID)
	if tenantID != "" && vlmLLMID != "" {
		describer, err := resolveImageDescriber(tenantID, vlmLLMID)
		if err != nil {
			return err
		}
		if err := enhanceWithVision(ctx, result, describer); err != nil {
			return err
		}
	}

	return nil
}

// resolveImageDescriber resolves a VLM model from tenant config and returns
// an ImageDescriber.  Corresponds to Python's
// get_model_config_from_provider_instance + LLMBundle.
// resolveImageDescriber resolves a VLM model from tenant config and returns
// an ImageDescriber.  The implementation is assigned by init() in
// post_steps_cgo.go (production) or post_steps_no_cgo.go (stub).
// Overridable in tests.
var resolveImageDescriber func(tenantID, llmID string) (ImageDescriber, error)

// SetImageDescriberResolver sets the factory that creates an ImageDescriber
// from tenant/LLM configuration. Higher layers (e.g. EE extensions or the
// PDF document pipeline entry point) register the real implementation via
// init(). If never called, PostProcess skips VLM enhancement.
func SetImageDescriberResolver(fn func(tenantID, llmID string) (ImageDescriber, error)) {
	resolveImageDescriber = fn
}

// ── normalizeLayoutType ────────────────────────────────────────────────

// normalizeLayoutType trims whitespace from LayoutType and defaults empty
// values to "text".  Matches Python's layout_type normalization in parser.py.
func normalizeLayoutType(result *pdftype.ParseResult) {
	for i := range result.Sections {
		lt := strings.TrimSpace(result.Sections[i].LayoutType)
		if lt == "" {
			lt = "text"
		}
		result.Sections[i].LayoutType = lt
	}
}

// ── filterHeaderFooter ─────────────────────────────────────────────────

// filterHeaderFooter removes sections whose LayoutType matches
// header/footer/number/reference.  Python: remove_header_footer config.
func filterHeaderFooter(result *pdftype.ParseResult) {
	sections := result.Sections[:0]
	for _, s := range result.Sections {
		if headerFooterPattern.MatchString(strings.TrimSpace(s.LayoutType)) {
			continue
		}
		sections = append(sections, s)
	}
	result.Sections = sections
}

// ── assignDocTypeKwd ───────────────────────────────────────────────────

// assignDocTypeKwd sets DocTypeKwd based on LayoutType and Image presence.
// When flatten is true, all sections become "text" and Image is cleared —
// this matches Python where flatten_media_to_text and VLM are mutually
// exclusive.  Python: parser.py:639-648.
func assignDocTypeKwd(result *pdftype.ParseResult, flatten bool) {
	for i := range result.Sections {
		s := &result.Sections[i]
		if flatten {
			s.DocTypeKwd = "text"
			s.Image = ""
			continue
		}
		lt := strings.TrimSpace(s.LayoutType)
		switch lt {
		case "table":
			s.DocTypeKwd = "table"
		case "figure":
			s.DocTypeKwd = "image"
		default:
			if lt == "" && s.Image != "" {
				s.DocTypeKwd = "image"
			} else {
				s.DocTypeKwd = "text"
			}
		}
	}
}

// ── enhanceWithVision ──────────────────────────────────────────────────

// enhanceWithVision adds VLM-generated descriptions to image/table sections.
func enhanceWithVision(ctx context.Context, result *pdftype.ParseResult, describer ImageDescriber) error {
	if describer == nil {
		return nil
	}
	if len(result.Sections) == 0 {
		return nil
	}

	sem := make(chan struct{}, maxDescribeConcurrency)
	var wg sync.WaitGroup

	for i := range result.Sections {
		s := &result.Sections[i]
		if s.DocTypeKwd != "table" && s.DocTypeKwd != "image" {
			continue
		}
		if s.Image == "" {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, imgB64 string, origText string) {
			defer wg.Done()
			defer func() { <-sem }()

			img, err := util.DecodeBase64PNG(imgB64)
			if err != nil || img == nil {
				return
			}
			desc, err := DescribeImage(ctx, img, describePrompt, describer)
			if err != nil || desc == "" {
				return
			}

			if origText != "" {
				result.Sections[idx].Text = origText + "\n" + desc
			} else {
				result.Sections[idx].Text = desc
			}
		}(i, s.Image, s.Text)
	}
	wg.Wait()

	return nil
}

// ── removeTOCByOutlines ────────────────────────────────────────────────

// removeTOCByOutlines removes sections whose page numbers fall inside
// TOC page ranges identified by PDF outlines.
func removeTOCByOutlines(result *pdftype.ParseResult, outlines []pdftype.Outline) {
	if len(outlines) == 0 {
		return
	}
	tocPage, contentPage := findTOCPageRange(outlines)
	if contentPage <= tocPage {
		return
	}
	sections := result.Sections[:0]
	for _, s := range result.Sections {
		pg := sectionPage(s)
		if pg >= tocPage && pg < contentPage {
			continue
		}
		sections = append(sections, s)
	}
	result.Sections = sections
}

// findTOCPageRange scans outlines for a TOC entry and returns the
// [tocStartPage, contentStartPage) range. Returns (0, 0) when not found.
func findTOCPageRange(outlines []pdftype.Outline) (tocPage, contentPage int) {
trimSplit:
	for i, o := range outlines {
		title := strings.TrimSpace(o.Title)
		if idx := strings.Index(title, "@@"); idx >= 0 {
			title = strings.TrimSpace(title[:idx])
		}
		if !tocTitlePattern.MatchString(strings.ToLower(title)) {
			continue
		}
		tocPage = o.PageNumber
		for _, next := range outlines[i+1:] {
			if next.Level != o.Level {
				continue
			}
			nt := strings.TrimSpace(next.Title)
			if idx := strings.Index(nt, "@@"); idx >= 0 {
				nt = strings.TrimSpace(nt[:idx])
			}
			if tocTitlePattern.MatchString(strings.ToLower(nt)) {
				continue
			}
			contentPage = next.PageNumber
			break trimSplit
		}
		break
	}
	return
}

// sectionPage returns the first page number of a Section, or 0.
func sectionPage(s pdftype.Section) int {
	for _, p := range s.Positions {
		for _, pn := range p.PageNumbers {
			return pn
		}
	}
	return 0
}

// ── reorderMultiColumn ─────────────────────────────────────────────────

// reorderMultiColumn reorders text sections in multi-column layouts.
// If median text column width >= page width / 2 (single-column layout),
// the input order is preserved.
//
// Python: reorder_multi_column_bboxes + sort_X_by_page
func reorderMultiColumn(result *pdftype.ParseResult, pageWidth, zoom float64) {
	if len(result.Sections) < 2 {
		return
	}
	pw := pageWidth / zoom

	// Compute median width from text sections with valid coordinates.
	var widths []float64
	for _, s := range result.Sections {
		if s.LayoutType != "text" {
			continue
		}
		if len(s.Positions) == 0 {
			continue
		}
		w := s.Positions[0].Right - s.Positions[0].Left
		if w > 0 {
			widths = append(widths, w)
		}
	}
	if len(widths) == 0 {
		return
	}
	sort.Float64s(widths)
	medianW := widths[len(widths)/2]

	if medianW >= pw/2 {
		return // single column
	}

	// Sort by (PageNumber, X0, Top).
	sort.Slice(result.Sections, func(i, j int) bool {
		pi := sectionPage(result.Sections[i])
		pj := sectionPage(result.Sections[j])
		if pi != pj {
			return pi < pj
		}
		xi := sectionX0(result.Sections[i])
		xj := sectionX0(result.Sections[j])
		if math.Abs(xi-xj) > 1e-6 {
			return xi < xj
		}
		return sectionTop(result.Sections[i]) < sectionTop(result.Sections[j])
	})

	threshold := medianW / 2
	// Correct same-page sections with nearly-same X0 but inverted Top.
	for i := len(result.Sections) - 1; i >= 1; i-- {
		for j := i - 1; j >= 0; j-- {
			if math.Abs(sectionX0(result.Sections[j+1])-sectionX0(result.Sections[j])) < threshold &&
				sectionTop(result.Sections[j+1]) < sectionTop(result.Sections[j]) &&
				sectionPage(result.Sections[j+1]) == sectionPage(result.Sections[j]) {
				result.Sections[j], result.Sections[j+1] = result.Sections[j+1], result.Sections[j]
			}
		}
	}
}

func sectionX0(s pdftype.Section) float64 {
	for _, p := range s.Positions {
		return p.Left
	}
	return 0
}

func sectionTop(s pdftype.Section) float64 {
	for _, p := range s.Positions {
		return p.Top
	}
	return 0
}
