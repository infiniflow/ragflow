package service

import (
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/tokenizer"
)

// QueryBuilder builds search queries from user questions
// Reference: rag/nlp/query.py
type QueryBuilder struct {
	queryFields []string
}

// NewQueryBuilder creates a new QueryBuilder
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		queryFields: []string{
			"title_tks^10",
			"title_sm_tks^5",
			"important_kwd^30",
			"important_tks^20",
			"question_tks^20",
			"content_ltks^2",
			"content_sm_ltks",
		},
	}
}

// QuestionResult contains the result of processing a question
type QuestionResult struct {
	MatchText string   // Processed match text for ES query_string
	Keywords  []string // Extracted keywords
}

// Question processes the question text and returns match text and keywords
// Reference: rag/nlp/query.py L41-88 (simplified version)
func (qb *QueryBuilder) Question(text string, minMatch float64) (*QuestionResult, error) {
	// Clean the text
	text = cleanText(text)

	// Tokenize using rag_tokenizer
	tokenized, err := tokenizer.Tokenize(text)
	if err != nil {
		// If tokenizer fails, return simple split
		tokenized = strings.ToLower(text)
	}

	// Get keywords from tokenized text
	keywords := extractKeywords(tokenized)

	// Build match text for ES query_string
	matchText := buildMatchText(keywords)

	return &QuestionResult{
		MatchText: matchText,
		Keywords:  keywords,
	}, nil
}

// cleanText cleans and normalizes the input text
func cleanText(text string) string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Replace punctuation and special characters with space
	// Reference: rag/nlp/query.py L44-48
	re := regexp.MustCompile(`[ :|\r\n\t,，.。?？/\` + "`" + `!！&^%()\[\]{}<>]+`)
	text = re.ReplaceAllString(text, " ")

	// Trim spaces
	text = strings.TrimSpace(text)

	return text
}

// extractKeywords extracts keywords from tokenized text
func extractKeywords(tokenized string) []string {
	if tokenized == "" {
		return []string{}
	}

	// Split by space
	parts := strings.Fields(tokenized)

	// Filter out empty and short tokens
	var keywords []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && len(p) > 1 {
			keywords = append(keywords, p)
		}
	}

	// Limit keywords count (similar to Python's [:256])
	if len(keywords) > 256 {
		keywords = keywords[:256]
	}

	return keywords
}

// buildMatchText builds ES query_string format match text
// Reference: rag/nlp/query.py L69-85
func buildMatchText(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}

	// Build boosted query for each keyword
	// Format: (keyword^1.0) with synonym expansion
	var parts []string
	for i, kw := range keywords {
		if kw == "" {
			continue
		}

		// Skip special characters
		if strings.ContainsAny(kw, ".^+()-[]") {
			continue
		}

		// Add keyword with boost (simplified, no synonym for now)
		boost := 1.0
		if i < 5 {
			boost = 2.0 // Boost first few keywords
		}

		// Escape quotes in keyword
		kw = strings.ReplaceAll(kw, `"`, `\"`)
		parts = append(parts, fmt.Sprintf(`(%s^%.1f)`, kw, boost))
	}

	return strings.Join(parts, " OR ")
}

// GetQueryFields returns the default query fields
func (qb *QueryBuilder) GetQueryFields() []string {
	return qb.queryFields
}
