package esoscommon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

var queryStringOperators = map[string]bool{"AND": true, "OR": true, "NOT": true}

var memoryMessageVectorFieldRE = regexp.MustCompile(`^q_\d+_vec$`)

// VectorFieldSpec describes how a particular engine renders a dense/knn vector
// mapping property. Elasticsearch and OpenSearch use different mapping syntax,
// so the shared builders must be told which flavour to emit instead of
// hardcoding one engine's syntax (which silently breaks the other engine).
type VectorFieldSpec struct {
	Type         string // "dense_vector" (Elasticsearch) | "knn_vector" (OpenSearch)
	DimsKey      string // "dims" (Elasticsearch) | "dimension" (OpenSearch)
	SimKey       string // "similarity" (Elasticsearch) | "space_type" (OpenSearch)
	SimValue     string // "cosine" | "cosinesimil"
	IndexKnnbool bool   // OpenSearch sets settings.index.knn=true
}

var ESVectorField = VectorFieldSpec{
	Type:     "dense_vector",
	DimsKey:  "dims",
	SimKey:   "similarity",
	SimValue: "cosine",
}

var OSVectorField = VectorFieldSpec{
	Type:         "knn_vector",
	DimsKey:      "dimension",
	SimKey:       "space_type",
	SimValue:     "cosinesimil",
	IndexKnnbool: true,
}

func BuildQueryStringQuery(matchText *types.MatchTextExpr, isSkillIndex, isMemoryIndex bool) map[string]interface{} {
	if matchText == nil {
		return nil
	}

	minimumShouldMatch := "0%"
	if matchText.ExtraOptions != nil {
		if msm, ok := matchText.ExtraOptions["minimum_should_match"].(float64); ok {
			minimumShouldMatch = common.FormatMinimumShouldMatchPercent(msm)
		}
	}

	fields := matchText.Fields
	if fields == nil || len(fields) == 0 {
		if isSkillIndex {
			fields = []string{"name_tks^10", "tags_tks^5", "description_tks^3", "content_tks^1"}
		} else if isMemoryIndex {
			fields = []string{"tokenized_content_ltks"}
		} else {
			fields = []string{"title_tks^10", "title_sm_tks^5", "important_kwd^30", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"}
		}
	}
	if isSkillIndex {
		fields = MapSkillSearchFields(fields)
	}
	if isMemoryIndex {
		fields = MapMemoryMessageESFields(fields, true)
	}

	boost := 1.0
	if matchText.ExtraOptions != nil {
		if b, ok := matchText.ExtraOptions["boost"].(float64); ok {
			boost = b
		}
	}

	return map[string]interface{}{
		"query_string": map[string]interface{}{
			"fields":               fields,
			"type":                 "best_fields",
			"query":                LowerCaseQueryText(matchText.MatchingText),
			"minimum_should_match": minimumShouldMatch,
			"boost":                boost,
		},
	}
}

// BuildRankFeatureQuery builds rank_feature clauses for learning-to-rank.
//
// emitPagerankRankFeature selects the engine flavour:
//   - Elasticsearch emits a top-level rank_feature clause for pagerank_fea and
//     then passes "" to CalculateScores, so pagerank is counted exactly once.
//   - OpenSearch keeps pagerank scoring in the secondary CalculateScores path
//     (it passes common.PAGERANK_FLD there) and must NOT also emit a
//     rank_feature boost here, otherwise pagerank is counted twice.
func BuildRankFeatureQuery(rankFeature map[string]float64, emitPagerankRankFeature bool) []map[string]interface{} {
	if rankFeature == nil || len(rankFeature) == 0 {
		return nil
	}

	// Sort keys for deterministic query order (Go map iteration is randomized)
	keys := make([]string, 0, len(rankFeature))
	for k := range rankFeature {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queries []map[string]interface{}
	for _, fld := range keys {
		if fld == common.PAGERANK_FLD && !emitPagerankRankFeature {
			// OpenSearch scores pagerank via CalculateScores only; skip the
			// rank_feature boost to avoid double counting.
			continue
		}
		sc := rankFeature[fld]
		field := fmt.Sprintf("%s.%s", common.TAG_FLD, fld)
		if fld == common.PAGERANK_FLD {
			// pagerank_fea is a top-level rank_feature field (the "*_fea" dynamic
			// template), not one key inside the tag_feas map.
			field = common.PAGERANK_FLD
		}
		queries = append(queries, map[string]interface{}{
			"rank_feature": map[string]interface{}{
				"field":  field,
				"linear": map[string]interface{}{},
				"boost":  sc,
			},
		})
	}
	return queries
}

func CalculateScores(chunks []map[string]interface{}, scoreColumn, pagerankField string) []map[string]interface{} {
	for i := range chunks {
		score := 0.0
		if scoreVal, ok := chunks[i][scoreColumn]; ok {
			if f, ok := ToFloat64(scoreVal); ok {
				score += f
			}
		}
		if pagerankField != "" {
			if prVal, ok := chunks[i][pagerankField]; ok {
				if f, ok := ToFloat64(prVal); ok {
					score += f
				}
			}
		}
		chunks[i]["_score"] = score
	}
	return chunks
}

func ConditionTermsList(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if ok {
		return items, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		// []byte is a scalar blob, not a list of ids/keywords.
		return nil, false
	}
	out := make([]interface{}, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out = append(out, rv.Index(i).Interface())
	}
	return out, true
}

func CopyFields(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}

func EsIndexNameForTenant(tenantID string) (string, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "", fmt.Errorf("tenant id cannot be empty")
	}
	if strings.Contains(tenantID, ",") {
		return "", fmt.Errorf("regexp search supports exactly one tenant")
	}
	return "ragflow_" + tenantID, nil
}

func FormatOrderedTagFeas(v any) (json.RawMessage, bool) {
	if v == nil {
		return nil, false
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw, true
	}
	if marshaler, ok := v.(json.Marshaler); ok {
		b, err := marshaler.MarshalJSON()
		if err == nil {
			return json.RawMessage(b), true
		}
	}
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	roundScore := func(f float64) int {
		if f < 0 {
			return int(f - 0.5)
		}
		return int(f + 0.5)
	}

	switch m := v.(type) {
	case map[string]int:
		kvs = make([]kv, 0, len(m))
		for k, val := range m {
			kvs = append(kvs, kv{k, val})
		}
	case map[string]float64:
		kvs = make([]kv, 0, len(m))
		for k, val := range m {
			kvs = append(kvs, kv{k, roundScore(val)})
		}
	case map[string]any:
		kvs = make([]kv, 0, len(m))
		for k, val := range m {
			switch score := val.(type) {
			case int:
				kvs = append(kvs, kv{k, score})
			case float64:
				kvs = append(kvs, kv{k, roundScore(score)})
			case int64:
				kvs = append(kvs, kv{k, int(score)})
			case json.Number:
				if f, err := score.Float64(); err == nil {
					kvs = append(kvs, kv{k, roundScore(f)})
				}
			}
		}
	default:
		return nil, false
	}

	if len(kvs) == 0 {
		return json.RawMessage("{}"), true
	}

	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].v != kvs[j].v {
			return kvs[i].v > kvs[j].v
		}
		return kvs[i].k < kvs[j].k
	})

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, item := range kvs {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(item.k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(item.v))
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.Bytes()), true
}

func GetChunkScore(chunk map[string]interface{}) float64 {
	if v, ok := chunk["_score"].(float64); ok {
		return v
	}
	if v, ok := chunk["SCORE"].(float64); ok {
		return v
	}
	if v, ok := chunk["SIMILARITY"].(float64); ok {
		return v
	}
	return 0.0
}

// GetDefaultSkillMapping returns the fallback skill-index mapping. vf selects
// the vector flavour: Elasticsearch uses dense_vector/dims/similarity with no
// index.knn setting, OpenSearch uses knn_vector/dimension/space_type and sets
// index.knn=true (OpenSearch rejects dense_vector mappings).
func GetDefaultSkillMapping(vf VectorFieldSpec) map[string]interface{} {
	settings := map[string]interface{}{
		"number_of_shards":   1,
		"number_of_replicas": 0,
		"refresh_interval":   "1000ms",
	}
	if vf.IndexKnnbool {
		// OpenSearch requires index.knn=true for knn_vector; the original
		// per-engine mapping nested every index setting under settings.index.
		settings = map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   1,
				"number_of_replicas": 0,
				"refresh_interval":   "1000ms",
				"knn":                true,
			},
		}
	}

	// Vector columns follow the shared "q_<dim>_vec" convention. The only
	// engine-specific part is the mapping syntax, injected through vf.
	vectorDims := []int{3072, 2560, 1536, 1024, 768, 512, 256}
	properties := map[string]interface{}{
		"skill_id": map[string]interface{}{
			"type":  "keyword",
			"store": true,
		},
		"name": map[string]interface{}{
			"type":  "text",
			"index": false,
			"store": true,
		},
		"name_tks": map[string]interface{}{
			"type":     "text",
			"analyzer": "whitespace",
			"store":    true,
		},
		"tags": map[string]interface{}{
			"type":  "text",
			"index": false,
			"store": true,
		},
		"tags_tks": map[string]interface{}{
			"type":     "text",
			"analyzer": "whitespace",
			"store":    true,
		},
		"description": map[string]interface{}{
			"type":  "text",
			"index": false,
			"store": true,
		},
		"description_tks": map[string]interface{}{
			"type":     "text",
			"analyzer": "whitespace",
			"store":    true,
		},
		"content": map[string]interface{}{
			"type":  "text",
			"index": false,
			"store": true,
		},
		"content_tks": map[string]interface{}{
			"type":     "text",
			"analyzer": "whitespace",
			"store":    true,
		},
		"version": map[string]interface{}{
			"type":  "keyword",
			"store": true,
		},
		"status": map[string]interface{}{
			"type":  "keyword",
			"store": true,
		},
		"create_time": map[string]interface{}{
			"type":  "long",
			"store": true,
		},
		"update_time": map[string]interface{}{
			"type":  "long",
			"store": true,
		},
	}
	for _, dim := range vectorDims {
		properties[common.VectorColumnName(dim)] = map[string]interface{}{
			"type":     vf.Type,
			vf.DimsKey: dim,
			"index":    true,
			vf.SimKey:  vf.SimValue,
		}
	}

	return map[string]interface{}{
		"settings": settings,
		"mappings": map[string]interface{}{
			"dynamic":    false,
			"properties": properties,
		},
	}
}

// GetMemoryMessageMapping returns the memory-message index mapping. vf selects
// the vector flavour: Elasticsearch uses dense_vector with no index.knn setting,
// OpenSearch uses knn_vector and must set settings.index.knn=true (otherwise
// the knn_vector field cannot be created).
func GetMemoryMessageMapping(vectorSize int, vf VectorFieldSpec) map[string]interface{} {
	vectorField := MemoryMessageVectorField(vectorSize)
	settings := map[string]interface{}{
		"number_of_shards":   1,
		"number_of_replicas": 0,
	}
	if vf.IndexKnnbool {
		settings["index"] = map[string]interface{}{
			"knn": true,
		}
	}
	return map[string]interface{}{
		"settings": settings,
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type": "keyword",
				},
				"doc_id": map[string]interface{}{
					"type": "keyword",
				},
				"kb_id": map[string]interface{}{
					"type": "keyword",
				},
				"memory_id": map[string]interface{}{
					"type": "keyword",
				},
				"user_id": map[string]interface{}{
					"type": "keyword",
				},
				"agent_id": map[string]interface{}{
					"type": "keyword",
				},
				"session_id": map[string]interface{}{
					"type": "keyword",
				},
				"message_id": map[string]interface{}{
					"type": "long",
				},
				"source_id": map[string]interface{}{
					"type": "long",
				},
				"message_type_kwd": map[string]interface{}{
					"type": "keyword",
				},
				"status_int": map[string]interface{}{
					"type": "integer",
				},
				"content_ltks": map[string]interface{}{
					"type":     "text",
					"analyzer": "whitespace",
				},
				"tokenized_content_ltks": map[string]interface{}{
					"type":     "text",
					"analyzer": "whitespace",
				},
				"valid_at": map[string]interface{}{
					"type":   "date",
					"format": "yyyy-MM-dd HH:mm:ss||strict_date_optional_time||epoch_millis",
				},
				"invalid_at": map[string]interface{}{
					"type":   "date",
					"format": "yyyy-MM-dd HH:mm:ss||strict_date_optional_time||epoch_millis",
				},
				"forget_at": map[string]interface{}{
					"type":   "date",
					"format": "yyyy-MM-dd HH:mm:ss||strict_date_optional_time||epoch_millis",
				},
				vectorField: MemoryMessageVectorProperty(vectorSize, vf),
			},
		},
	}
}

func IsASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func IsLatinKeyword(keyword string) bool {
	for _, r := range keyword {
		if !unicode.In(r, unicode.Latin) {
			return false
		}
	}
	return keyword != ""
}

// LoadSkillMapping reads the engine-specific skill-index mapping file. The
// caller must pass the correct file for its engine (conf/skill_es_mapping.json
// for Elasticsearch, conf/skill_os_mapping.json for OpenSearch) together with
// the matching vf: OpenSearch rejects the dense_vector syntax in the ES file,
// so the loader must never silently substitute one engine's file or default
// for the other. When the file is missing the engine-correct default mapping
// (built from vf) is returned as a graceful fallback, preserving the original
// per-engine behaviour; a parse error is still surfaced as an error.
func LoadSkillMapping(filename string, vf VectorFieldSpec) (map[string]interface{}, error) {
	base := filepath.Base(filename)
	possiblePaths := []string{
		filename,
		"../conf/" + base,
		"/app/conf/" + base,
	}

	var data []byte
	var err error
	for _, path := range possiblePaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		// File missing: fall back to the engine-correct default rather than
		// erroring, matching the original per-engine behaviour. The default is
		// built from vf so it carries the correct vector flavour.
		return GetDefaultSkillMapping(vf), nil
	}

	var mapping map[string]interface{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse skill mapping %q: %w", filename, err)
	}

	return mapping, nil
}

func LowerCaseQueryText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	inQuote := false
	escaped := false
	for i := 0; i < len(text); {
		c := text[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			i++
			continue
		}
		switch {
		case c == '\\':
			b.WriteByte(c)
			escaped = true
			i++
		case c == '"':
			inQuote = !inQuote
			b.WriteByte(c)
			i++
		case IsASCIILetter(c):
			j := i
			for j < len(text) && IsASCIILetter(text[j]) {
				j++
			}
			word := text[i:j]
			if !inQuote && queryStringOperators[word] {
				b.WriteString(word)
			} else {
				b.WriteString(strings.ToLower(word))
			}
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func MapMemoryMessageESConditionFields(condition map[string]interface{}) map[string]interface{} {
	mapped := make(map[string]interface{}, len(condition))
	for k, v := range condition {
		mapped[MapMemoryMessageESField(k, false)] = v
	}
	return mapped
}

func MapMemoryMessageESField(field string, useTokenizedContent bool) string {
	name := field
	boost := ""
	if base, suffix, ok := strings.Cut(field, "^"); ok {
		name = base
		boost = "^" + suffix
	}

	switch name {
	case "message_type":
		name = "message_type_kwd"
	case "status":
		name = "status_int"
	case "content":
		if useTokenizedContent {
			name = "tokenized_content_ltks"
		} else {
			name = "content_ltks"
		}
	}
	return name + boost
}

func MapMemoryMessageESFields(fields []string, useTokenizedContent bool) []string {
	if len(fields) == 0 {
		return fields
	}
	mapped := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		mappedField := MapMemoryMessageESField(field, useTokenizedContent)
		if _, ok := seen[mappedField]; ok {
			continue
		}
		seen[mappedField] = struct{}{}
		mapped = append(mapped, mappedField)
	}
	return mapped
}

func MapMemoryMessageESUpdateFields(newValue map[string]interface{}) map[string]interface{} {
	doc := make(map[string]interface{}, len(newValue))
	for k, v := range newValue {
		switch k {
		case "remove", "add":
			doc[k] = v
		default:
			doc[MapMemoryMessageESField(k, false)] = v
		}
	}
	// Mirror Python memory/utils/es_conn.py update: writing content
	// refreshes tokenized_content_ltks before write.
	if content, ok := newValue["content"].(string); ok {
		doc["tokenized_content_ltks"] = TokenizeMemoryMessageContent(content)
	}
	return doc
}

func MapMemoryMessageToESDoc(message map[string]interface{}) map[string]interface{} {
	doc := make(map[string]interface{}, len(message)+2)
	for k, v := range message {
		switch k {
		case "message_type":
			doc["message_type_kwd"] = v
		case "status":
			doc["status_int"] = MemoryMessageStatusInt(v)
		case "content":
			content, _ := v.(string)
			doc["content_ltks"] = content
			doc["tokenized_content_ltks"] = TokenizeMemoryMessageContent(content)
		default:
			doc[k] = v
		}
	}
	return doc
}

func MapSkillSearchFields(fields []string) []string {
	mapped := make([]string, 0, len(fields))
	for _, field := range fields {
		name, boost, hasBoost := strings.Cut(field, "^")
		switch name {
		case "name":
			name = "name_tks"
		case "tags":
			name = "tags_tks"
		case "description":
			name = "description_tks"
		case "content":
			name = "content_tks"
		}
		if hasBoost {
			mapped = append(mapped, name+"^"+boost)
		} else {
			mapped = append(mapped, name)
		}
	}
	return mapped
}

func MemoryMessageStatusBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case json.Number:
		n, err := v.Int64()
		return err == nil && n != 0
	case string:
		return v != "" && v != "0" && !strings.EqualFold(v, "false")
	default:
		return false
	}
}

func MemoryMessageStatusInt(value interface{}) int {
	if MemoryMessageStatusBool(value) {
		return 1
	}
	return 0
}

func MemoryMessageVectorField(vectorSize int) string {
	return common.VectorColumnName(vectorSize)
}

// MemoryMessageVectorProperty returns the vector mapping property for the
// memory-message index. vf selects the engine flavour (dense_vector for
// Elasticsearch, knn_vector for OpenSearch).
func MemoryMessageVectorProperty(vectorSize int, vf VectorFieldSpec) map[string]interface{} {
	return map[string]interface{}{
		"type":     vf.Type,
		vf.DimsKey: vectorSize,
		"index":    true,
		vf.SimKey:  vf.SimValue,
	}
}

func NormalizeMemoryMessageChunks(chunks []map[string]interface{}) {
	for _, chunk := range chunks {
		for key, val := range chunk {
			if memoryMessageVectorFieldRE.MatchString(key) {
				chunk["content_embed"] = val
				delete(chunk, key)
			}
		}
		if val, ok := chunk["message_type_kwd"]; ok {
			chunk["message_type"] = val
			delete(chunk, "message_type_kwd")
		}
		if val, ok := chunk["status_int"]; ok {
			chunk["status"] = MemoryMessageStatusBool(val)
			delete(chunk, "status_int")
		}
		if val, ok := chunk["content_ltks"]; ok {
			chunk["content"] = val
			delete(chunk, "content_ltks")
		}
	}
}

func ParseMemoryMessageVectorSize(field string) (int, bool) {
	if !memoryMessageVectorFieldRE.MatchString(field) {
		return 0, false
	}
	dim, ok := common.VectorColumnDim(field)
	if !ok || dim <= 0 {
		return 0, false
	}
	return dim, true
}

func ParseOrderByExpr(orderBy *types.OrderByExpr) []map[string]interface{} {
	if orderBy == nil || len(orderBy.Fields) == 0 {
		return nil
	}

	var result []map[string]interface{}
	for _, field := range orderBy.Fields {
		direction := "asc"
		if field.Type == types.SortDesc {
			direction = "desc"
		}

		// Skip id field (cannot order by text field). The logical "id" column is
		// shimmed from the ES _id in the response path, but ordering on _id
		// requires indices.id_field_data, which is disabled by default on the
		// bundled Elasticsearch 8.x deployment and would make every id-ordered
		// query fail at runtime. Callers that need deep pagination must supply a
		// real stored, sortable field instead of relying on the logical id.
		if field.Field == "id" {
			continue
		}

		// Special handling for page_num_int and top_int
		if field.Field == "page_num_int" || field.Field == "top_int" {
			result = append(result, map[string]interface{}{
				field.Field: map[string]interface{}{
					"order":         direction,
					"unmapped_type": "float",
					"mode":          "avg",
					"numeric_type":  "double",
				},
			})
		} else if strings.HasSuffix(field.Field, "_int") || strings.HasSuffix(field.Field, "_flt") {
			// Fields ending with _int or _flt
			result = append(result, map[string]interface{}{
				field.Field: map[string]interface{}{
					"order":         direction,
					"unmapped_type": "float",
				},
			})
		} else if field.Field == "_score" || field.Field == "score" {
			result = append(result, map[string]interface{}{
				"_score": direction,
			})
		} else {
			// Default: unmapped_type = keyword
			result = append(result, map[string]interface{}{
				field.Field: map[string]interface{}{
					"order":         direction,
					"unmapped_type": "keyword",
				},
			})
		}
	}

	return result
}

func SortByFields(chunks []map[string]interface{}, expr *types.OrderByExpr) []map[string]interface{} {
	if expr == nil || len(expr.Fields) == 0 {
		return chunks
	}
	sort.SliceStable(chunks, func(i, j int) bool {
		for _, field := range expr.Fields {
			vi, oki := chunks[i][field.Field]
			vj, okj := chunks[j][field.Field]
			if !oki || !okj {
				// Missing field sorts before a present one.
				if !oki && !okj {
					continue
				}
				return !oki
			}
			// Compare the two values and derive a stable equality key. The raw
			// interface{} equality test below would panic on uncomparable types
			// ([]interface{}, map from ES source), so the equality check always
			// runs on string forms instead.
			var less bool
			var keyI, keyJ string
			if fi, ei := ToFloat64(vi); ei {
				if fj, ej := ToFloat64(vj); ej {
					less = fi < fj
				} else {
					less = false // numeric before non-numeric
				}
				keyI, keyJ = fmt.Sprintf("%v", vi), fmt.Sprintf("%v", vj)
			} else {
				si, siOk := vi.(string)
				sj, sjOk := vj.(string)
				if siOk && sjOk {
					less = si < sj
					keyI, keyJ = si, sj
				} else {
					keyI, keyJ = fmt.Sprintf("%v", vi), fmt.Sprintf("%v", vj)
					less = keyI < keyJ
				}
			}
			if keyI != keyJ {
				if field.Type == types.SortDesc {
					return !less
				}
				return less
			}
		}
		return false
	})
	return chunks
}

func SortByScore(chunks []map[string]interface{}, limit int) []map[string]interface{} {
	if len(chunks) == 0 {
		return chunks
	}

	// Stable: the reference does not re-sort at all, so ties keep the order ES
	// returned them in.
	sort.SliceStable(chunks, func(i, j int) bool {
		scoreI := GetChunkScore(chunks[i])
		scoreJ := GetChunkScore(chunks[j])
		return scoreI > scoreJ
	})

	// Limit
	if len(chunks) > limit && limit > 0 {
		chunks = chunks[:limit]
	}

	return chunks
}

func SortValuesEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ToFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

func TokenizeMemoryMessageContent(content string) string {
	tokenized, err := tokenizer.Tokenize(content)
	if err != nil {
		common.Warn("memory message tokenize failed, storing raw content", zap.Error(err))
		return content
	}
	fine, err := tokenizer.FineGrainedTokenize(tokenized)
	if err != nil {
		common.Warn("memory message fine-grained tokenize failed, storing coarse tokens", zap.Error(err))
		return tokenized
	}
	return fine
}
