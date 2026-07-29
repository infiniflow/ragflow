package wiki

import (
	"fmt"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

const wikiMapSystem = `You are a knowledge extraction engine. Extract structured knowledge from the provided document sections. Return ONLY valid JSON matching the schema exactly. Never include text outside the JSON object. Keep the source language of the document.`

const wikiReduceSystem = `You are a knowledge synthesis engine. Normalize and deduplicate the structured extracts while preserving source_chunk_ids provenance. Claims, relations, and topics are pass-through facts; do not invent new ones. Return ONLY valid JSON matching the same schema.`

const wikiPlanSystem = `You are a knowledge compilation planner. Given structured knowledge, produce a wiki page plan. Return ONLY valid JSON.`

const wikiRefineSystem = `You are a technical writer. Write a complete wiki page from the plan, evidence checklist, and source text. Preserve factual density, keep the source language, and return only markdown.`

const wikiMapUserTemplate = `## Document context
Document id: {doc_id}
Batch contains {chunk_count} packed chunk(s). Each chunk is introduced by a
"[CHUNK_ID <id>]" line. The chunk_id values to choose from are:
{chunk_id_list}

## Packed chunks
{packed_chunks}

---

Extract all knowledge from every chunk and return a single JSON object with this
exact schema:

{
  "entities": [
    {
      "name": "string",
      "type": "string",
      "aliases": ["string"],
      "source_chunk_id": "string"
    }
  ],
  "concepts": [
    {
      "term": "string",
      "definition_excerpt": "string",
      "source_chunk_id": "string"
    }
  ],
  "claims": [
    {
      "statement": "string",
      "subject": "string",
      "confidence": "explicit",
      "source_chunk_id": "string"
    }
  ],
  "relations": [
    {
      "from": "string",
      "to": "string",
      "type": "string",
      "source_chunk_id": "string"
    }
  ],
  "topics": ["string"]
}

## Customization
The following rules and schema customizations override the defaults above and
must be honoured when extracting from this knowledge base:

- Entity types (override default): {entity_type_rules}
- Relation types (override default): {relation_type_rules}
- Concept term guidance: {concept_term}
- Concept definition guidance: {concept_definition_excerpt}
- Claim statement guidance: {claim_statement}
- Claim subject guidance: {claim_subject}
{custom_rules}

Rules:
- source_chunk_id MUST be one of the chunk_id values listed above.
- Do not invent opaque identifiers, hashes, or prompt scaffolding as names.
- Extract entities and concepts from human-readable text only.
- Claims should be liberal: every factual sentence about an entity or concept is a claim.
- Relations should be explicit links only.
- Topics should capture coherent subtopics that could become wiki sections.
- Return empty arrays [] for categories with no findings.
- Return ONLY the JSON object, no markdown fences, no commentary.`

const wikiReduceUserTemplate = `## Reduced input
Document id: {doc_id}
{
  "entities": {entities},
  "concepts": {concepts},
  "claims": {claims},
  "relations": {relations},
  "topics": {topics}
}

Normalize duplicates, merge aliases and provenance, and return the same schema.`

const wikiPlanBatchUserTemplate = `## Knowledge base context
Document id: {doc_id}
Batch: {batch_index}/{batch_total}

## Reduced input
{
  "entities": {entities},
  "concepts": {concepts},
  "claims": {claims},
  "relations": {relations},
  "topics": {topics}
}

Return a JSON compilation plan with one or more page entries:
{
  "pages": [
    {
      "action": "CREATE",
      "slug": "string",
      "title": "string",
      "page_type": "entity | concept | topic",
      "topic": "string",
      "entity_names": ["string"],
      "related_kb_pages": ["string"],
      "priority": 1,
      "lead": "string",
      "sections": [
        { "heading": "string", "points": ["string"] }
      ]
    }
  ],
  "estimated_page_count": 1,
  "compilation_notes": "string"
}

Rules:
- Prefer one page per high-signal entity or concept when the batch supports it.
- Use page_type=entity for entity pages, page_type=concept for concept pages, and page_type=topic for cross-cutting themes.
- entity_names must name the entities and concepts that justify the page.
- related_kb_pages should list other slugs from the same plan that the page should cross-link to.
- Keep the page in the source language.
- Return ONLY the JSON object.`

const wikiPlanMergeUserTemplate = `## Knowledge base context
Document id: {doc_id}

## Partial plans
{candidates}

Merge the partial plans into one final compilation plan with the same JSON shape as above.
Preserve distinct pages when they cover different entities or concepts; drop only near-duplicate slugs.
Return ONLY the JSON object.`

const wikiPlanReconcileSystem = `You are a wiki page reconciliation engine. Compare a planned wiki page with existing wiki pages and decide whether the planned page should UPDATE one of them or CREATE a new page. Return only valid JSON.`

const wikiPlanReconcileUserTemplate = `## Planned page
{planned_page}

## Candidate existing pages
{candidates}

Return JSON:
{
  "action": "UPDATE | CREATE",
  "slug": "string",
  "reason": "string"
}

Rules:
- Choose UPDATE only when the planned page and candidate refer to the same underlying entity, concept, or topic.
- Prefer CREATE when the overlap is weak, ambiguous, or just generally related.
- If action is UPDATE, slug must be exactly one candidate slug.
- Return only JSON.`

const wikiRefineWriterExample = `Each page must be a proper encyclopedic article, not a flat bullet list:
1. Opening paragraph that defines the page subject.
2. H2 sections with prose before any bullets.
3. Bold key terms on first use.
4. Link related pages with [[slug]] or [[slug|display text]].
5. End with a short "See also" section when relevant.`

const wikiRefineWriterSystemTemplate = `You are an enterprise knowledge compilation writer. Your job is to write a single, high-quality wiki page by reading the SOURCE TEXT and using the evidence checklist as guidance for what to cover.

Write in the same language as the source text. Do not translate content.

# Page structure
{template_example}

# Wikilinks
- Use [[slug]] or [[slug|display text]] to cross-link.
- Only link to slugs from the available-pages list.

Return only markdown.`

const wikiRefineWriterUserTemplate = `## Task
{action} the following wiki page.

## Page specification
- Slug: {slug}
- Title: {title}
- Type: {page_type}

## Available pages (ONLY use these slugs for [[wikilinks]])
{all_plan_slugs}

{existing_section}

## Source document text
{source_context}

## Evidence checklist ({evidence_count} items)
{evidence_blocks}

## Instructions
Write the complete wiki page in markdown based on the source text above.
Cross-link to other pages using [[slug]] or [[slug|display text]] and only use slugs from the available pages list.
Return only the markdown content.`

const wikiRefineMergeSystem = `You are a wiki page merger. You receive two versions of the same wiki page:
- EXISTING: the current version in the knowledge base.
- INCOMING: a new version generated from a different source document.

Produce one unified page that preserves all factual content from both versions.
- Keep all facts, numbers, procedures, and named entities from both versions.
- Remove exact duplicates.
- Preserve and normalize [[wikilinks]].
- Keep the same language as the existing page.
- Do not summarize or condense away factual detail.
- Return only markdown.`

func formatWikiChunkBatch(docID string, batch []common.Chunk) (string, []string) {
	var b strings.Builder
	ids := make([]string, 0, len(batch))
	for i, c := range batch {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			id = fmt.Sprintf("chunk-%d", i+1)
		}
		ids = append(ids, id)
		text := firstNonEmpty(c.Text, c.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		b.WriteString("[CHUNK_ID ")
		b.WriteString(id)
		b.WriteString("]\n")
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	_ = docID
	return b.String(), ids
}

func buildWikiMapPrompt(docID string, batch []common.Chunk, parserConfig any, language string) (string, []string) {
	body, ids := formatWikiChunkBatch(docID, batch)
	entityTypeRules := "person|org|product|regulation|location|system|equipment|other"
	relationTypeRules := "include|ordered|owns|part_of|caused_by|regulates|uses|located_in|other"
	conceptTerm := "named term or topic"
	conceptDef := "short definition excerpt from the source text"
	claimStatement := "factual statement"
	claimSubject := "entity or concept that the claim is about"
	customRules := ""

	if cfg, ok := parserConfig.(map[string]any); ok {
		if rules := wikiTemplateCustomRules(cfg, language); rules != "" {
			customRules = rules
		}
		if ent := wikiTemplateFields(cfg, "entity"); len(ent) > 0 {
			if s := wikiRenderSchemaBody(ent, language, defaultEntitySchemaBody); s != "" {
				entityTypeRules = s
			}
		}
		if rel := wikiTemplateFields(cfg, "relation"); len(rel) > 0 {
			if s := wikiRenderSchemaBody(rel, language, defaultRelationSchemaBody); s != "" {
				relationTypeRules = s
			}
		}
		if conceptFields := wikiTemplateFields(cfg, "concept"); len(conceptFields) > 0 {
			if s := wikiPipeJoin(conceptFields, "term"); s != "" {
				conceptTerm = s
			}
			if s := wikiColonJoin(conceptFields, "term", "definition_excerpt"); s != "" {
				conceptDef = s
			}
		}
		if claimFields := wikiTemplateFields(cfg, "claim"); len(claimFields) > 0 {
			if s := wikiNamedFieldDescription(claimFields, "statement"); s != "" {
				claimStatement = s
			}
			if s := wikiNamedFieldDescription(claimFields, "subject"); s != "" {
				claimSubject = s
			}
		}
	}

	user := renderWikiTemplate(wikiMapUserTemplate, map[string]string{
		"doc_id":                     docID,
		"chunk_count":                fmt.Sprintf("%d", len(batch)),
		"chunk_id_list":              strings.Join(prefixWithDash(ids), "\n"),
		"packed_chunks":              body,
		"entity_type_rules":          entityTypeRules,
		"relation_type_rules":        relationTypeRules,
		"concept_term":               conceptTerm,
		"concept_definition_excerpt": conceptDef,
		"claim_statement":            claimStatement,
		"claim_subject":              claimSubject,
		"custom_rules":               customRules,
	})
	return user, ids
}

func buildWikiRefineWriterSystem(example string) string {
	body := strings.TrimSpace(example)
	if body == "" {
		body = wikiRefineWriterExample
	}
	return renderWikiTemplate(wikiRefineWriterSystemTemplate, map[string]string{
		"template_example": body,
	})
}

const defaultEntitySchemaBody = `"name": "string — entity canonical name as it appears in text",
      "type": "string — one of: person|org|product|regulation|location|system|equipment|other",
      "aliases": ["string"],
      "source_chunk_id": "string — exact value from the chunk_id list above"`

const defaultRelationSchemaBody = `"from": "string — source entity/concept name",
      "to": "string — target entity/concept name",
      "type": "string — e.g. owns|part_of|caused_by|regulates|uses|located_in|other",
      "source_chunk_id": "string — exact value from the chunk_id list above"`

func wikiTemplateCustomRules(parserConfig map[string]any, language string) string {
	guideline := common.GetMap(parserConfig, "guideline")
	if len(guideline) == 0 {
		return ""
	}
	var sections []string
	if rules := common.Localize(common.Get(guideline, "rules_for_entities"), language); strings.TrimSpace(rules) != "" {
		sections = append(sections, "## Entity extraction rules (from knowledge base config):\n"+rules)
	}
	if rules := common.Localize(common.Get(guideline, "rules_for_relations"), language); strings.TrimSpace(rules) != "" {
		sections = append(sections, "## Relation extraction rules (from knowledge base config):\n"+rules)
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n" + strings.Join(sections, "\n\n") + "\n"
}

func wikiTemplateFields(parserConfig map[string]any, section string) []any {
	cfg := common.GetMap(parserConfig, section)
	if len(cfg) == 0 {
		return nil
	}
	if fields, ok := cfg["fields"].([]any); ok {
		return fields
	}
	if fields, ok := cfg["fields"].([]map[string]any); ok {
		out := make([]any, 0, len(fields))
		for _, f := range fields {
			out = append(out, f)
		}
		return out
	}
	return nil
}

func wikiRenderSchemaBody(fields []any, language, defaultBody string) string {
	if len(fields) == 0 {
		return defaultBody
	}
	lines := make([]string, 0, len(fields)+1)
	seen := map[string]bool{}
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(firstString(field["name"]))
		if name == "" || name == "source_chunk_id" || seen[name] {
			continue
		}
		seen[name] = true
		ftype := strings.TrimSpace(firstString(field["type"]))
		if ftype == "" {
			ftype = "str"
		}
		desc := common.Localize(field["description"], language)
		var placeholder string
		switch ftype {
		case "list":
			placeholder = `["string"]`
		case "int":
			placeholder = "0"
		case "float":
			placeholder = "0.0"
		case "bool":
			placeholder = "false"
		default:
			if strings.TrimSpace(desc) != "" {
				desc = strings.NewReplacer("\n", " ", "{", "(", "}", ")").Replace(desc)
				placeholder = fmt.Sprintf(`"string — %s"`, strings.TrimSpace(desc))
			} else {
				placeholder = `"string"`
			}
		}
		lines = append(lines, fmt.Sprintf(`      "%s": %s`, name, placeholder))
	}
	if len(lines) == 0 {
		return defaultBody
	}
	lines = append(lines, `      "source_chunk_id": "string — exact value from the chunk_id list above"`)
	return strings.Join(lines, ",\n")
}

func wikiPipeJoin(fields []any, key string) string {
	var vals []string
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if s := strings.TrimSpace(firstString(field[key])); s != "" {
			vals = append(vals, s)
		}
	}
	return strings.Join(vals, "|")
}

func wikiColonJoin(fields []any, leftKey, rightKey string) string {
	var vals []string
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		left := strings.TrimSpace(firstString(field[leftKey]))
		right := strings.TrimSpace(firstString(field[rightKey]))
		if left != "" || right != "" {
			vals = append(vals, left+":"+right)
		}
	}
	return strings.Join(vals, "\n")
}

func wikiNamedFieldDescription(fields []any, name string) string {
	for _, raw := range fields {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(firstString(field["name"])), name) {
			if s := strings.TrimSpace(common.Localize(field["description"], "en")); s != "" {
				return s
			}
		}
	}
	return ""
}
