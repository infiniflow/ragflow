package dao

import "strings"

// An ORDER BY body is built by string concatenation, so a column name that
// reached it from a query parameter would be an injection point. Every entity
// therefore names the columns its list rows expose, and only those names can be
// rendered. The allowlists and the rendering live here so the rule has one
// owner rather than a copy in each data access file.

// OrderTerm is one column and direction in an ORDER BY list.
type OrderTerm struct {
	Column string
	Desc   bool
}

func renderTerm(table string, term OrderTerm) string {
	column := term.Column
	if table != "" {
		column = table + "." + column
	}
	if term.Desc {
		return column + " DESC"
	}
	return column + " ASC"
}

// orderClause renders terms as an ORDER BY body, keeping only the terms whose
// column the entity allows. When no term survives, the fallback column is
// rendered in the direction the first requested term asked for, which is what
// the single-value orderby parameter has always done with a name it does not
// recognise.
func orderClause(allowed map[string]struct{}, terms []OrderTerm, fallback string) string {
	return qualifiedOrderClause("", allowed, terms, fallback)
}

// qualifiedOrderClause is orderClause with a table qualifier on every column,
// for queries whose joins would otherwise leave a bare column name ambiguous.
func qualifiedOrderClause(table string, allowed map[string]struct{}, terms []OrderTerm, fallback string) string {
	kept := make([]string, 0, len(terms))
	for _, term := range terms {
		if _, ok := allowed[term.Column]; ok {
			kept = append(kept, renderTerm(table, term))
		}
	}
	if len(kept) == 0 {
		desc := len(terms) > 0 && terms[0].Desc
		return renderTerm(table, OrderTerm{Column: fallback, Desc: desc})
	}
	return strings.Join(kept, ", ")
}

// defaultOrderColumn is the column every entity falls back to.
const defaultOrderColumn = "create_time"

// chatOrderableColumns allows the scalar dialog columns the list rows expose plus the base timestamp columns.
var chatOrderableColumns = map[string]struct{}{
	"id":                       {},
	"tenant_id":                {},
	"name":                     {},
	"language":                 {},
	"llm_id":                   {},
	"tenant_llm_id":            {},
	"prompt_type":              {},
	"similarity_threshold":     {},
	"vector_similarity_weight": {},
	"top_n":                    {},
	"rerank_candidates_count":  {},
	"top_k":                    {},
	"do_refer":                 {},
	"rerank_id":                {},
	"tenant_rerank_id":         {},
	"status":                   {},
	"create_time":              {},
	"create_date":              {},
	"update_time":              {},
	"update_date":              {},
}

// chatSessionOrderableColumns allows the scalar conversation columns the list rows expose plus the base timestamp columns.
var chatSessionOrderableColumns = map[string]struct{}{
	"id":          {},
	"dialog_id":   {},
	"name":        {},
	"user_id":     {},
	"create_time": {},
	"create_date": {},
	"update_time": {},
	"update_date": {},
}

// compilationTemplateGroupOrderableColumns keeps the four columns this list has
// always accepted rather than every scalar column of the row, so moving the rule
// here does not widen what callers can order by.
var compilationTemplateGroupOrderableColumns = map[string]struct{}{
	"name":        {},
	"scope":       {},
	"create_time": {},
	"update_time": {},
}

// fileOrderableColumns allows the scalar file columns the list rows expose plus the base timestamp columns.
var fileOrderableColumns = map[string]struct{}{
	"id":          {},
	"parent_id":   {},
	"tenant_id":   {},
	"created_by":  {},
	"name":        {},
	"location":    {},
	"size":        {},
	"type":        {},
	"source_type": {},
	"create_time": {},
	"create_date": {},
	"update_time": {},
	"update_date": {},
}

// knowledgebaseOrderableColumns allows the scalar knowledge base columns the list rows expose plus the base timestamp columns.
var knowledgebaseOrderableColumns = map[string]struct{}{
	"id":             {},
	"tenant_id":      {},
	"name":           {},
	"language":       {},
	"permission":     {},
	"doc_num":        {},
	"token_num":      {},
	"chunk_num":      {},
	"parser_id":      {},
	"pagerank":       {},
	"embd_id":        {},
	"tenant_embd_id": {},
	"create_time":    {},
	"create_date":    {},
	"update_time":    {},
	"update_date":    {},
}

// pipelineLogOrderableColumns allows the scalar pipeline operation log columns the list rows expose plus the base timestamp columns.
var pipelineLogOrderableColumns = map[string]struct{}{
	"id":               {},
	"document_id":      {},
	"tenant_id":        {},
	"kb_id":            {},
	"pipeline_id":      {},
	"pipeline_title":   {},
	"parser_id":        {},
	"document_name":    {},
	"document_suffix":  {},
	"document_type":    {},
	"source_from":      {},
	"progress":         {},
	"process_begin_at": {},
	"process_duration": {},
	"task_type":        {},
	"operation_status": {},
	"status":           {},
	"create_time":      {},
	"create_date":      {},
	"update_time":      {},
	"update_date":      {},
}

// searchOrderableColumns allows the scalar search columns the list rows expose plus the base timestamp columns.
var searchOrderableColumns = map[string]struct{}{
	"id":          {},
	"tenant_id":   {},
	"name":        {},
	"created_by":  {},
	"status":      {},
	"create_time": {},
	"create_date": {},
	"update_time": {},
	"update_date": {},
}

// userCanvasOrderableColumns allows the scalar user canvas columns the list rows expose plus the base timestamp columns.
var userCanvasOrderableColumns = map[string]struct{}{
	"id":              {},
	"user_id":         {},
	"title":           {},
	"permission":      {},
	"canvas_type":     {},
	"canvas_category": {},
	"tags":            {},
	"create_time":     {},
	"create_date":     {},
	"update_time":     {},
	"update_date":     {},
}

func chatOrderClause(orderby string, desc bool) string {
	return orderClause(chatOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func chatSessionOrderClause(orderby string, desc bool) string {
	return orderClause(chatSessionOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func compilationTemplateGroupOrderClause(orderby string, desc bool) string {
	return orderClause(compilationTemplateGroupOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func fileOrderClause(orderBy string, desc bool) string {
	return orderClause(fileOrderableColumns, []OrderTerm{{Column: orderBy, Desc: desc}}, defaultOrderColumn)
}

func knowledgebaseOrderClause(orderby string, desc bool) string {
	return orderClause(knowledgebaseOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func knowledgebaseQualifiedOrderClause(orderby string, desc bool) string {
	return qualifiedOrderClause("knowledgebase", knowledgebaseOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func pipelineLogOrderClause(orderby string, desc bool) string {
	return orderClause(pipelineLogOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func searchOrderClause(orderby string, desc bool) string {
	return orderClause(searchOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func userCanvasOrderClause(orderby string, desc bool) string {
	return orderClause(userCanvasOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}

func userCanvasQualifiedOrderClause(orderby string, desc bool) string {
	return qualifiedOrderClause("user_canvas", userCanvasOrderableColumns, []OrderTerm{{Column: orderby, Desc: desc}}, defaultOrderColumn)
}
