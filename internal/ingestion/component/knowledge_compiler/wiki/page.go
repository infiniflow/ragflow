package wiki

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

type wikiPageResult struct {
	Slug           string
	Title          string
	PageType       string
	Topic          string
	Action         string
	EntityNames    []string
	RelatedKBPages []string
	ContentRaw     string
	Content        string
	Summary        string
	Outlinks       []string
	SourceChunkIDs []string
	SourceDocIDs   []string
}

// mapFromChunks runs the MAP stage: extract raw facts/notes from the source.
func mapFromChunks(ctx context.Context, deps common.Deps, llmID, sections string) (string, error) {
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: wikiMapSystem,
		UserPrompt:   sections,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// reduceFromExtracts consolidates the MAP output into topic clusters.
func reduceFromExtracts(ctx context.Context, deps common.Deps, llmID, raw string) (string, error) {
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: wikiReduceSystem,
		UserPrompt:   raw,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// planFromReduction arranges topics into a page outline.
func planFromReduction(ctx context.Context, deps common.Deps, llmID, reduced string) (string, error) {
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: wikiPlanSystem,
		UserPrompt:   reduced,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// refineFromPlan writes the final wiki page from the outline.
func refineFromPlan(ctx context.Context, deps common.Deps, llmID, plan string) (string, error) {
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: wikiRefineSystem,
		UserPrompt:   plan,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Section is one heading block parsed out of a wiki page's markdown body. The
// page itself is a separate product; sections live as children linked via
// parent_id so the retrieval-side can scope queries to a section (e.g. user
// asks "what does the page say about X?" and the SIDE picks the page + the X
// section).
type Section struct {
	Title   string
	Level   int    // 1..6, ATX heading depth
	Content string // section body until the next heading of equal-or-lower depth
}

// ParseOutline splits a wiki page's markdown body into a flat section list.
// Headings are ATX style ("#", "##", ...). The page is treated as a single
// implicit "preamble" section when it starts with non-heading text. The page
// itself is *not* in the result; buildPageProducts emits it separately so
// callers can attach the section list under it.
func ParseOutline(md string) []Section {
	var sections []Section
	var current *Section
	flush := func() {
		if current != nil {
			current.Content = strings.TrimSpace(current.Content)
			if current.Title != "" || current.Content != "" {
				sections = append(sections, *current)
			}
			current = nil
		}
	}
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if lvl := headingLevel(trimmed); lvl > 0 {
			flush()
			title := strings.TrimSpace(trimmed[lvl:])
			current = &Section{Title: strings.TrimSpace(stripMarkdownEmphasis(title)), Level: lvl}
			continue
		}
		if current == nil {
			// Preamble: treat leading prose as a synthetic level-0 section so
			// the page body is not silently discarded.
			current = &Section{Title: "", Level: 0}
		}
		current.Content += trimmed + "\n"
	}
	flush()
	return sections
}

// headingLevel returns the ATX heading level of s (1..6) or 0 if not a heading.
func headingLevel(s string) int {
	if !strings.HasPrefix(s, "#") {
		return 0
	}
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n > 6 || n >= len(s) {
		return 0
	}
	if s[n] != ' ' {
		return 0
	}
	return n
}

func stripMarkdownEmphasis(s string) string {
	return strings.Trim(s, "`*_")
}

// buildPageProducts turns the final wiki page into one "page" product plus one
// "section" product per heading in the page body. Sections carry their title,
// level, and content; vector embedding is applied by the caller in a single
// batched Encode. Sections are linked to the page via parent_id.
func buildPageProducts(tenantID, docID, page string, sourceChunkIDs []string) []common.Product {
	return buildWikiPageProducts(tenantID, docID, []wikiPageResult{{
		Slug:           slugify(firstHeading(page)),
		Title:          firstHeading(page),
		PageType:       "concept",
		Topic:          firstParagraph(page),
		Action:         "CREATE",
		ContentRaw:     page,
		Content:        page,
		Summary:        firstParagraph(page),
		SourceChunkIDs: sourceChunkIDs,
		SourceDocIDs:   []string{docID},
	}})
}

// buildWikiPageProducts turns multiple wiki page drafts into page + section
// products. Each page result becomes one page product and one section product
// per heading in its markdown body.
func buildWikiPageProducts(tenantID, docID string, pages []wikiPageResult) []common.Product {
	if len(pages) == 0 {
		return nil
	}
	out := make([]common.Product, 0, len(pages)*2)
	for _, page := range pages {
		slug := strings.TrimSpace(page.Slug)
		if slug == "" {
			slug = slugify(page.Title)
		}
		if slug == "" {
			continue
		}
		title := strings.TrimSpace(page.Title)
		if title == "" {
			title = slug
		}
		pageType := strings.TrimSpace(page.PageType)
		if pageType == "" {
			pageType = "concept"
		}
		content := page.Content
		if strings.TrimSpace(content) == "" {
			content = page.ContentRaw
		}
		summary := strings.TrimSpace(page.Summary)
		if summary == "" {
			summary = firstParagraph(content)
		}
		sourceChunkIDs := uniqueStrings(page.SourceChunkIDs)
		sourceDocIDs := uniqueStrings(page.SourceDocIDs)
		if len(sourceDocIDs) == 0 && docID != "" {
			sourceDocIDs = []string{docID}
		}
		pageID := common.StableRowID(tenantID, docID, string(common.VariantWiki), "page", slug)
		out = append(out, common.Product{
			ID:       pageID,
			DocID:    docID,
			TenantID: tenantID,
			Variant:  common.VariantWiki,
			Content:  content,
			ParentID: "",
			Meta: map[string]any{
				"kind":             "page",
				"slug":             slug,
				"title":            title,
				"page_type":        pageType,
				"topic":            strings.TrimSpace(page.Topic),
				"summary":          summary,
				"entity_names":     uniqueStrings(page.EntityNames),
				"related_kb_pages": uniqueStrings(page.RelatedKBPages),
				"outlinks":         uniqueStrings(page.Outlinks),
				"source_chunk_ids": sourceChunkIDs,
				"source_doc_ids":   sourceDocIDs,
				"content_md_raw":   page.ContentRaw,
			},
		})
		for i, sec := range ParseOutline(content) {
			if strings.TrimSpace(sec.Title) == "" {
				continue
			}
			sectionContent := strings.TrimSpace(sec.Content)
			if sectionContent == "" {
				continue
			}
			out = append(out, common.Product{
				ID:       common.StableRowID(tenantID, docID, string(common.VariantWiki), "section", slug, sec.Title, pageID),
				DocID:    docID,
				TenantID: tenantID,
				Variant:  common.VariantWiki,
				Content:  sectionContent,
				ParentID: pageID,
				Meta: map[string]any{
					"kind":             "section",
					"slug":             slugify(sec.Title),
					"title":            sec.Title,
					"page_slug":        slug,
					"page_title":       title,
					"page_type":        pageType,
					"section_level":    sec.Level,
					"section_index":    i,
					"source_chunk_ids": sourceChunkIDs,
					"source_doc_ids":   sourceDocIDs,
				},
			})
		}
	}
	return out
}

// firstHeading returns the first markdown "# "-prefixed line, trimmed.
func firstHeading(md string) string {
	for _, line := range strings.Split(md, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(s, "# "))
		}
	}
	return ""
}

// firstParagraph returns the first non-empty, non-heading line, capped.
func firstParagraph(md string) string {
	for _, line := range strings.Split(md, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if len(s) > 300 {
			return s[:300]
		}
		return s
	}
	return ""
}

// slugify produces a filesystem-safe slug from a heading. ASCII letters/digits
// and hyphens are kept verbatim, and unicode letters/digits (e.g. CJK titles)
// are preserved too so distinct non-Latin titles don't all collapse to "page"
// (M17). When the result is empty (punctuation-only or whitespace-only title)
// a stable hash of the input is used so different such titles stay distinct.
func slugify(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "page"
	}
	s = strings.ToLower(trimmed)
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		h := fnv.New32a()
		_, _ = h.Write([]byte(trimmed))
		return "page-" + fmt.Sprintf("%08x", h.Sum32())
	}
	return b.String()
}
