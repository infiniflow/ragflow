//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package tablequery defines a closed, structured query IR for single-table
// analytical retrieval across document engines.
//
// Untrusted LLM outputs never supply SQL text, table names, or dataset scope.
// Instead, the LLM emits a JSON Query validated against an authorization-owned
// Scope, and each engine compiles the resulting ValidatedQuery into its native
// read-only SQL (parameterized where supported, strictly escaped otherwise).
package tablequery

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kaptinlin/jsonrepair"
)

const (
	// DefaultMaxLimit is applied whenever Scope.MaxLimit or Query.Limit is unset.
	DefaultMaxLimit = 1024
	// DefaultCitationLimit bounds secondary citation queries derived from aggregates.
	DefaultCitationLimit = 20

	maxSelectItems    = 64
	maxAggregateItems = 32
	maxGroupByItems   = 16
	maxOrderByItems   = 16
	maxFilterDepth    = 8
	maxFilterNodes    = 64
	maxInListLen      = 128
	maxOffset         = 100000
)

// ErrUnsupported indicates the LLM explicitly signaled that the user question
// cannot be represented as a single-table structured query and should fall back
// to vector retrieval.
var ErrUnsupported = errors.New("tablequery: unsupported question")

// FieldType classifies a logical column in Scope.Fields so engine compilers can
// choose the appropriate comparison, tokenization, or numeric cast semantics.
type FieldType string

const (
	FieldString FieldType = "string"
	FieldNumber FieldType = "number"
	FieldDate   FieldType = "date"
	FieldBool   FieldType = "bool"
	FieldText   FieldType = "text"
)

// Field describes one whitelisted column from field_map.
type Field struct {
	Name        string
	DisplayName string
	Type        FieldType
}

// Scope is built exclusively by the authorization layer and is never supplied
// or modified by the LLM.
type Scope struct {
	TenantID  string
	TableName string
	KBIDs     []string
	Fields    map[string]Field
	MaxLimit  int
}

// AggFn enumerates supported aggregate functions.
type AggFn string

const (
	AggCount         AggFn = "count"
	AggCountDistinct AggFn = "count_distinct"
	AggSum           AggFn = "sum"
	AggAvg           AggFn = "avg"
	AggMin           AggFn = "min"
	AggMax           AggFn = "max"
)

// Aggregate describes one aggregate projection.
type Aggregate struct {
	Fn    AggFn  `json:"fn"`
	Field string `json:"field,omitempty"`
	As    string `json:"as"`
}

// FilterOp enumerates supported filter predicates.
type FilterOp string

const (
	OpEq         FilterOp = "eq"
	OpNeq        FilterOp = "neq"
	OpGt         FilterOp = "gt"
	OpGte        FilterOp = "gte"
	OpLt         FilterOp = "lt"
	OpLte        FilterOp = "lte"
	OpIn         FilterOp = "in"
	OpNotIn      FilterOp = "not_in"
	OpContains   FilterOp = "contains"
	OpStartsWith FilterOp = "starts_with"
	OpEndsWith   FilterOp = "ends_with"
	OpIsNull     FilterOp = "is_null"
	OpIsNotNull  FilterOp = "is_not_null"
)

// FilterNode represents a recursive boolean filter tree. Exactly one of And,
// Or, Not, or a leaf (Field + Op) may be populated.
type FilterNode struct {
	And   []FilterNode `json:"and,omitempty"`
	Or    []FilterNode `json:"or,omitempty"`
	Not   *FilterNode  `json:"not,omitempty"`
	Field string       `json:"field,omitempty"`
	Op    FilterOp     `json:"op,omitempty"`
	Value any          `json:"value,omitempty"`
}

// OrderSpec specifies a single ORDER BY item.
type OrderSpec struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc,omitempty"`
}

// Query is the closed JSON IR emitted by the LLM.
type Query struct {
	Unsupported bool        `json:"unsupported,omitempty"`
	Select      []string    `json:"select,omitempty"`
	Distinct    bool        `json:"distinct,omitempty"`
	Aggregates  []Aggregate `json:"aggregates,omitempty"`
	Where       *FilterNode `json:"where,omitempty"`
	GroupBy     []string    `json:"group_by,omitempty"`
	Having      *FilterNode `json:"having,omitempty"`
	OrderBy     []OrderSpec `json:"order_by,omitempty"`
	Limit       int         `json:"limit,omitempty"`
	Offset      int         `json:"offset,omitempty"`
}

// ValidatedQuery is an opaque, immutable query that has passed full Scope and
// schema validation. Unexported fields ensure no caller can bypass validation.
type ValidatedQuery struct {
	scope            Scope
	query            Query
	includeCitations bool
	citationOnly     bool
	aggAliases       map[string]Aggregate
}

var (
	uuidRe        = regexp.MustCompile(`^[0-9a-fA-F]{8}(-?[0-9a-fA-F]{4}){3}-?[0-9a-fA-F]{12}$`)
	tableIdentRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,127}$`)
	aliasIdentRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	thinkBlockRe  = regexp.MustCompile(`(?s)<think>.*?</think>`)
	cnThinkLineRe = regexp.MustCompile(`(?s)思考\n.*?\n`)
	jsonFenceRe   = regexp.MustCompile("(?i)```(?:json)?\\s*")
	fenceEndRe    = regexp.MustCompile("```\\s*$")
)

var reservedAliases = map[string]bool{
	"doc_id":    true,
	"kb_id":     true,
	"kb_id_kwd": true,
	"docnm":     true,
	"docnm_kwd": true,
}

// IsValidUUID reports whether s is a canonical 32-hex UUID (with or without dashes).
func IsValidUUID(s string) bool {
	return s != "" && uuidRe.MatchString(s)
}

// IsValidTableIdentifier reports whether name is safe to emit as a physical
// table or index identifier.
func IsValidTableIdentifier(name string) bool {
	return name != "" && tableIdentRe.MatchString(name)
}

// FieldsFromFieldMap builds a Scope.Fields map from a knowledge base field_map.
// Display labels are preserved verbatim while logical types are inferred from
// RAGFlow's standard field suffixes and metadata labels.
func FieldsFromFieldMap(fieldMap map[string]interface{}) map[string]Field {
	if len(fieldMap) == 0 {
		return nil
	}
	fields := make(map[string]Field, len(fieldMap))
	for name, rawVal := range fieldMap {
		if strings.TrimSpace(name) == "" {
			continue
		}
		display := ""
		if rawVal != nil {
			display = fmt.Sprintf("%v", rawVal)
		}
		fields[name] = Field{
			Name:        name,
			DisplayName: display,
			Type:        inferFieldType(name, display),
		}
	}
	return fields
}

func inferFieldType(name, label string) FieldType {
	lowerName := strings.ToLower(name)
	lowerLabel := strings.ToLower(label)
	switch {
	case strings.HasSuffix(lowerName, "_tks"), strings.HasSuffix(lowerName, "_ltks"):
		return FieldText
	case strings.HasSuffix(lowerName, "_int"),
		strings.HasSuffix(lowerName, "_flt"),
		strings.HasSuffix(lowerName, "_long"),
		strings.HasSuffix(lowerName, "_float"),
		strings.HasSuffix(lowerName, "_double"),
		strings.HasSuffix(lowerName, "_num"),
		strings.Contains(lowerLabel, "(int"),
		strings.Contains(lowerLabel, "(float"),
		strings.Contains(lowerLabel, "(double"),
		strings.Contains(lowerLabel, "(number"):
		return FieldNumber
	case strings.HasSuffix(lowerName, "_dt"),
		strings.HasSuffix(lowerName, "_date"),
		strings.HasSuffix(lowerName, "_time"),
		strings.Contains(lowerLabel, "(date"):
		return FieldDate
	case strings.HasSuffix(lowerName, "_bool"),
		strings.Contains(lowerLabel, "(bool"):
		return FieldBool
	default:
		return FieldString
	}
}

// ValidateScope validates the authorization scope before invoking the LLM.
func ValidateScope(scope Scope) error {
	if !IsValidTableIdentifier(scope.TableName) {
		return fmt.Errorf("tablequery: invalid table name %q", scope.TableName)
	}
	if len(scope.KBIDs) == 0 {
		return fmt.Errorf("tablequery: at least one authorized kb_id is required")
	}
	for _, id := range scope.KBIDs {
		if !IsValidUUID(id) {
			return fmt.Errorf("tablequery: invalid kb_id UUID %q", id)
		}
	}
	if len(scope.Fields) == 0 {
		return fmt.Errorf("tablequery: empty field whitelist")
	}
	for k, f := range scope.Fields {
		if k == "" || (f.Name != "" && f.Name != k) {
			return fmt.Errorf("tablequery: inconsistent field key %q", k)
		}
	}
	return nil
}

// ParseAndValidate parses raw LLM output into a Query (cleaning markdown fences
// and repairing minor JSON issues) and validates it strictly against scope.
func ParseAndValidate(raw string, scope Scope, includeCitations bool) (*ValidatedQuery, error) {
	cleaned := stripLLMJSONArtifacts(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("tablequery: empty JSON response")
	}
	repaired, err := jsonrepair.Repair(cleaned)
	if err == nil && strings.TrimSpace(repaired) != "" {
		cleaned = repaired
	}

	var q Query
	dec := json.NewDecoder(strings.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil {
		return nil, fmt.Errorf("tablequery: decode JSON: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("tablequery: trailing content after JSON object")
	}
	if q.Unsupported {
		return nil, ErrUnsupported
	}
	return Validate(scope, q, includeCitations)
}

func stripLLMJSONArtifacts(s string) string {
	s = thinkBlockRe.ReplaceAllString(s, "")
	s = cnThinkLineRe.ReplaceAllString(s, "")
	s = jsonFenceRe.ReplaceAllString(s, "")
	s = fenceEndRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	// Extract the outermost JSON object if the model wrapped it in brief prose.
	if start := strings.IndexByte(s, '{'); start >= 0 {
		if end := strings.LastIndexByte(s, '}'); end > start {
			s = s[start : end+1]
		}
	}
	return strings.TrimSpace(s)
}

// Validate checks a structured Query against an authorization Scope and returns
// an immutable ValidatedQuery ready for engine compilation.
func Validate(scope Scope, q Query, includeCitations bool) (*ValidatedQuery, error) {
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}
	if q.Unsupported {
		return nil, ErrUnsupported
	}

	normScope := normalizeScope(scope)

	if len(q.Select) == 0 && len(q.Aggregates) == 0 {
		return nil, fmt.Errorf("tablequery: query must specify at least one select field or aggregate")
	}
	if len(q.Select) > maxSelectItems {
		return nil, fmt.Errorf("tablequery: too many select fields (%d > %d)", len(q.Select), maxSelectItems)
	}
	if len(q.Aggregates) > maxAggregateItems {
		return nil, fmt.Errorf("tablequery: too many aggregates (%d > %d)", len(q.Aggregates), maxAggregateItems)
	}
	if len(q.GroupBy) > maxGroupByItems {
		return nil, fmt.Errorf("tablequery: too many group_by fields (%d > %d)", len(q.GroupBy), maxGroupByItems)
	}
	if len(q.OrderBy) > maxOrderByItems {
		return nil, fmt.Errorf("tablequery: too many order_by items (%d > %d)", len(q.OrderBy), maxOrderByItems)
	}

	// Validate select fields against the whitelist.
	selectFields := make([]string, 0, len(q.Select))
	seenSelect := make(map[string]bool, len(q.Select))
	for _, f := range q.Select {
		resolved, ok := resolveFieldName(normScope.Fields, f)
		if !ok {
			return nil, fmt.Errorf("tablequery: unknown select field %q (allowed: %s)", f, allowedFieldsHint(normScope.Fields))
		}
		if !seenSelect[resolved] {
			seenSelect[resolved] = true
			selectFields = append(selectFields, resolved)
		}
	}

	// Validate aggregates.
	aggAliases := make(map[string]Aggregate, len(q.Aggregates))
	aggList := make([]Aggregate, 0, len(q.Aggregates))
	for i, agg := range q.Aggregates {
		fn := AggFn(strings.ToLower(strings.TrimSpace(string(agg.Fn))))
		switch fn {
		case AggCount, AggCountDistinct, AggSum, AggAvg, AggMin, AggMax:
		default:
			return nil, fmt.Errorf("tablequery: unsupported aggregate function %q", agg.Fn)
		}

		field := strings.TrimSpace(agg.Field)
		if field == "*" {
			field = ""
		}
		if fn == AggCount {
			if field != "" {
				resolved, ok := resolveFieldName(normScope.Fields, field)
				if !ok {
					return nil, fmt.Errorf("tablequery: unknown aggregate field %q (allowed: %s)", agg.Field, allowedFieldsHint(normScope.Fields))
				}
				field = resolved
			}
		} else {
			if field == "" {
				return nil, fmt.Errorf("tablequery: aggregate %q requires a field", fn)
			}
			resolved, ok := resolveFieldName(normScope.Fields, field)
			if !ok {
				return nil, fmt.Errorf("tablequery: unknown aggregate field %q (allowed: %s)", agg.Field, allowedFieldsHint(normScope.Fields))
			}
			field = resolved
		}

		alias := strings.TrimSpace(agg.As)
		if alias == "" {
			alias = defaultAggregateAlias(fn, i)
		}
		if !aliasIdentRe.MatchString(alias) {
			return nil, fmt.Errorf("tablequery: invalid aggregate alias %q", alias)
		}
		if reservedAliases[strings.ToLower(alias)] {
			return nil, fmt.Errorf("tablequery: aggregate alias %q collides with reserved citation column", alias)
		}
		if _, dup := aggAliases[alias]; dup {
			return nil, fmt.Errorf("tablequery: duplicate aggregate alias %q", alias)
		}

		normAgg := Aggregate{Fn: fn, Field: field, As: alias}
		aggAliases[alias] = normAgg
		aggList = append(aggList, normAgg)
	}

	// Validate group_by fields.
	groupBy := make([]string, 0, len(q.GroupBy))
	groupSet := make(map[string]bool, len(q.GroupBy))
	for _, f := range q.GroupBy {
		resolved, ok := resolveFieldName(normScope.Fields, f)
		if !ok {
			return nil, fmt.Errorf("tablequery: unknown group_by field %q (allowed: %s)", f, allowedFieldsHint(normScope.Fields))
		}
		if !groupSet[resolved] {
			groupSet[resolved] = true
			groupBy = append(groupBy, resolved)
		}
	}

	// If aggregates are present, every non-aggregated select column must be in group_by.
	if len(aggList) > 0 && len(selectFields) > 0 {
		for _, sf := range selectFields {
			if !groupSet[sf] {
				groupSet[sf] = true
				groupBy = append(groupBy, sf)
			}
		}
	}

	// Validate WHERE filter tree.
	var normWhere *FilterNode
	if q.Where != nil {
		nodes := 0
		w, err := validateFilterNode(q.Where, normScope.Fields, nil, 0, &nodes)
		if err != nil {
			return nil, fmt.Errorf("tablequery: invalid where: %w", err)
		}
		normWhere = w
	}

	// Validate HAVING filter tree (requires aggregates or group_by).
	var normHaving *FilterNode
	if q.Having != nil {
		if len(aggList) == 0 && len(groupBy) == 0 {
			return nil, fmt.Errorf("tablequery: having requires aggregates or group_by")
		}
		nodes := 0
		h, err := validateFilterNode(q.Having, normScope.Fields, aggAliases, 0, &nodes)
		if err != nil {
			return nil, fmt.Errorf("tablequery: invalid having: %w", err)
		}
		normHaving = h
	}

	// Validate ORDER BY.
	orderBy := make([]OrderSpec, 0, len(q.OrderBy))
	for _, o := range q.OrderBy {
		if _, isAgg := aggAliases[o.Field]; isAgg {
			orderBy = append(orderBy, OrderSpec{Field: o.Field, Desc: o.Desc})
			continue
		}
		if resolved, ok := resolveFieldName(normScope.Fields, o.Field); ok {
			orderBy = append(orderBy, OrderSpec{Field: resolved, Desc: o.Desc})
			continue
		}
		return nil, fmt.Errorf("tablequery: unknown order_by field %q", o.Field)
	}

	// Clamp LIMIT and validate OFFSET.
	limit := q.Limit
	if limit <= 0 || limit > normScope.MaxLimit {
		limit = normScope.MaxLimit
	}
	if q.Offset < 0 || q.Offset > maxOffset {
		return nil, fmt.Errorf("tablequery: offset %d out of range [0, %d]", q.Offset, maxOffset)
	}

	isAgg := len(aggList) > 0 || len(groupBy) > 0
	normQuery := Query{
		Select:     selectFields,
		Distinct:   q.Distinct && !isAgg,
		Aggregates: aggList,
		Where:      normWhere,
		GroupBy:    groupBy,
		Having:     normHaving,
		OrderBy:    orderBy,
		Limit:      limit,
		Offset:     q.Offset,
	}

	return &ValidatedQuery{
		scope:            normScope,
		query:            normQuery,
		includeCitations: includeCitations && !isAgg && !normQuery.Distinct,
		aggAliases:       aggAliases,
	}, nil
}

// BuildRowCountQuery constructs a deterministic COUNT(*) query without calling
// an LLM.
func BuildRowCountQuery(scope Scope) (*ValidatedQuery, error) {
	return Validate(scope, Query{
		Aggregates: []Aggregate{{Fn: AggCount, As: "rows"}},
		Limit:      1,
	}, false)
}

// DeriveCitationQuery derives a bounded citation-column query reusing the exact
// validated Scope and Where predicate from an aggregate query. Returns false
// when the query has no WHERE filter (matching existing secondary-fetch policy).
func (vq *ValidatedQuery) DeriveCitationQuery(limit int) (*ValidatedQuery, bool) {
	if vq == nil || vq.query.Where == nil {
		return nil, false
	}
	if limit <= 0 || limit > vq.scope.MaxLimit {
		limit = DefaultCitationLimit
	}
	if limit > vq.scope.MaxLimit {
		limit = vq.scope.MaxLimit
	}
	return &ValidatedQuery{
		scope:            vq.scope,
		query:            Query{Where: vq.query.Where, Limit: limit},
		includeCitations: true,
		citationOnly:     true,
	}, true
}

// Scope returns a defensive copy of the validated authorization scope.
func (vq *ValidatedQuery) Scope() Scope {
	if vq == nil {
		return Scope{}
	}
	return normalizeScope(vq.scope)
}

// Query returns a copy of the normalized IR query.
func (vq *ValidatedQuery) Query() Query {
	if vq == nil {
		return Query{}
	}
	return vq.query
}

// IsAggregate reports whether the query computes aggregates or groups rows.
func (vq *ValidatedQuery) IsAggregate() bool {
	if vq == nil {
		return false
	}
	return len(vq.query.Aggregates) > 0 || len(vq.query.GroupBy) > 0
}

// HasWhere reports whether the query has a user filter predicate.
func (vq *ValidatedQuery) HasWhere() bool {
	return vq != nil && vq.query.Where != nil
}

// IncludesCitations reports whether compiled SQL projects citation columns.
func (vq *ValidatedQuery) IncludesCitations() bool {
	return vq != nil && vq.includeCitations
}

func normalizeScope(s Scope) Scope {
	maxLim := s.MaxLimit
	if maxLim <= 0 {
		maxLim = DefaultMaxLimit
	}
	kbCopy := make([]string, len(s.KBIDs))
	copy(kbCopy, s.KBIDs)
	fieldsCopy := make(map[string]Field, len(s.Fields))
	for k, f := range s.Fields {
		if f.Name == "" {
			f.Name = k
		}
		if f.Type == "" {
			f.Type = inferFieldType(f.Name, f.DisplayName)
		}
		fieldsCopy[k] = f
	}
	return Scope{
		TenantID:  s.TenantID,
		TableName: s.TableName,
		KBIDs:     kbCopy,
		Fields:    fieldsCopy,
		MaxLimit:  maxLim,
	}
}

func defaultAggregateAlias(fn AggFn, idx int) string {
	if idx == 0 {
		return string(fn) + "_val"
	}
	return fmt.Sprintf("%s_%d", fn, idx+1)
}

func resolveFieldName(fields map[string]Field, candidate string) (string, bool) {
	if _, ok := fields[candidate]; ok {
		return candidate, true
	}
	trimmed := strings.TrimSpace(candidate)
	if _, ok := fields[trimmed]; ok {
		return trimmed, true
	}
	// Case-insensitive fallback only when unique.
	match := ""
	for k := range fields {
		if strings.EqualFold(k, trimmed) {
			if match != "" {
				return "", false
			}
			match = k
		}
	}
	if match != "" {
		return match, true
	}
	return "", false
}

func allowedFieldsHint(fields map[string]Field) string {
	names := make([]string, 0, len(fields))
	for k := range fields {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) > 20 {
		names = append(names[:20], "...")
	}
	return strings.Join(names, ", ")
}

func validateFilterNode(
	node *FilterNode,
	fields map[string]Field,
	aggAliases map[string]Aggregate,
	depth int,
	nodeCount *int,
) (*FilterNode, error) {
	if node == nil {
		return nil, fmt.Errorf("nil filter node")
	}
	if depth > maxFilterDepth {
		return nil, fmt.Errorf("filter nesting exceeds max depth %d", maxFilterDepth)
	}
	*nodeCount++
	if *nodeCount > maxFilterNodes {
		return nil, fmt.Errorf("filter exceeds max node count %d", maxFilterNodes)
	}

	kinds := 0
	if len(node.And) > 0 {
		kinds++
	}
	if len(node.Or) > 0 {
		kinds++
	}
	if node.Not != nil {
		kinds++
	}
	if node.Field != "" || node.Op != "" {
		kinds++
	}
	if kinds != 1 {
		return nil, fmt.Errorf("filter node must set exactly one of and, or, not, or field+op")
	}

	if len(node.And) > 0 {
		children := make([]FilterNode, 0, len(node.And))
		for i := range node.And {
			c, err := validateFilterNode(&node.And[i], fields, aggAliases, depth+1, nodeCount)
			if err != nil {
				return nil, err
			}
			children = append(children, *c)
		}
		return &FilterNode{And: children}, nil
	}
	if len(node.Or) > 0 {
		children := make([]FilterNode, 0, len(node.Or))
		for i := range node.Or {
			c, err := validateFilterNode(&node.Or[i], fields, aggAliases, depth+1, nodeCount)
			if err != nil {
				return nil, err
			}
			children = append(children, *c)
		}
		return &FilterNode{Or: children}, nil
	}
	if node.Not != nil {
		child, err := validateFilterNode(node.Not, fields, aggAliases, depth+1, nodeCount)
		if err != nil {
			return nil, err
		}
		return &FilterNode{Not: child}, nil
	}

	// Leaf predicate.
	fieldName := node.Field
	if aggAliases != nil {
		if _, isAgg := aggAliases[fieldName]; !isAgg {
			resolved, ok := resolveFieldName(fields, fieldName)
			if !ok {
				return nil, fmt.Errorf("unknown field %q (allowed: %s)", fieldName, allowedFieldsHint(fields))
			}
			fieldName = resolved
		}
	} else {
		resolved, ok := resolveFieldName(fields, fieldName)
		if !ok {
			return nil, fmt.Errorf("unknown field %q (allowed: %s)", fieldName, allowedFieldsHint(fields))
		}
		fieldName = resolved
	}

	op := FilterOp(strings.ToLower(strings.TrimSpace(string(node.Op))))
	switch op {
	case OpIsNull, OpIsNotNull:
		if node.Value != nil {
			return nil, fmt.Errorf("operator %q must not carry a value", op)
		}
		return &FilterNode{Field: fieldName, Op: op}, nil

	case OpContains, OpStartsWith, OpEndsWith:
		s, ok := node.Value.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("operator %q requires a non-empty string value", op)
		}
		return &FilterNode{Field: fieldName, Op: op, Value: s}, nil

	case OpIn, OpNotIn:
		vals, err := validateScalarSlice(node.Value)
		if err != nil {
			return nil, fmt.Errorf("operator %q: %w", op, err)
		}
		return &FilterNode{Field: fieldName, Op: op, Value: vals}, nil

	case OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte:
		val, err := validateScalar(node.Value)
		if err != nil {
			return nil, fmt.Errorf("operator %q: %w", op, err)
		}
		return &FilterNode{Field: fieldName, Op: op, Value: val}, nil

	default:
		return nil, fmt.Errorf("unsupported filter operator %q", node.Op)
	}
}

func validateScalar(v any) (any, error) {
	if v == nil {
		return nil, fmt.Errorf("value must not be null (use is_null/is_not_null)")
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case bool:
		return x, nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("non-finite numeric value")
		}
		return x, nil
	case float32:
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("non-finite numeric value")
		}
		return f, nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case json.Number:
		f, err := strconv.ParseFloat(string(x), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("invalid numeric value %q", x)
		}
		return f, nil
	default:
		return nil, fmt.Errorf("expected scalar value, got %T", v)
	}
}

func validateScalarSlice(v any) ([]any, error) {
	var raw []any
	switch x := v.(type) {
	case []any:
		raw = x
	case []string:
		raw = make([]any, len(x))
		for i, s := range x {
			raw[i] = s
		}
	case []float64:
		raw = make([]any, len(x))
		for i, n := range x {
			raw[i] = n
		}
	case []int:
		raw = make([]any, len(x))
		for i, n := range x {
			raw[i] = n
		}
	default:
		return nil, fmt.Errorf("expected non-empty array of scalars, got %T", v)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("array must not be empty")
	}
	if len(raw) > maxInListLen {
		return nil, fmt.Errorf("array length %d exceeds max %d", len(raw), maxInListLen)
	}
	out := make([]any, 0, len(raw))
	for _, elem := range raw {
		val, err := validateScalar(elem)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
	}
	return out, nil
}
