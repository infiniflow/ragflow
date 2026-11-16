package esoscommon_test

import (
	"encoding/json"
	"testing"

	"ragflow/internal/engine/internal/esoscommon"
	"ragflow/internal/engine/types"
)

func TestEsIndexNameForTenant(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"t1", "ragflow_t1", false},
		{"  t2  ", "ragflow_t2", false}, // trimmed
		{"", "", true},                  // empty -> error (adversarial)
		{"a,b", "", true},               // comma -> error (adversarial)
	}
	for _, c := range cases {
		got, err := esoscommon.EsIndexNameForTenant(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("EsIndexNameForTenant(%q): expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("EsIndexNameForTenant(%q) = (%q,%v), want (%q,nil)", c.in, got, err, c.want)
		}
	}
}

func TestIsASCIILetter(t *testing.T) {
	if !esoscommon.IsASCIILetter('a') || !esoscommon.IsASCIILetter('Z') {
		t.Error("ASCII letters should be true")
	}
	if esoscommon.IsASCIILetter('1') || esoscommon.IsASCIILetter(' ') {
		t.Error("non-letters should be false")
	}
	// adversarial: a byte outside ASCII range is passed as a rune value 0xff
	if esoscommon.IsASCIILetter(byte(0xff)) {
		t.Error("0xff byte should be false")
	}
}

func TestIsLatinKeyword(t *testing.T) {
	if !esoscommon.IsLatinKeyword("hello") {
		t.Error("latin word should be true")
	}
	if esoscommon.IsLatinKeyword("") {
		t.Error("empty string should be false")
	}
	if esoscommon.IsLatinKeyword("病毒") {
		t.Error("CJK should be false (adversarial)")
	}
	if !esoscommon.IsLatinKeyword("café") {
		t.Error("accented latin should be true")
	}
}

func TestLowerCaseQueryText(t *testing.T) {
	if got := esoscommon.LowerCaseQueryText("Foo AND Bar"); got != "foo AND bar" {
		t.Errorf("got %q want %q", got, "foo AND bar")
	}
	// operators stay upper; non-operator words fold even inside quotes.
	if got := esoscommon.LowerCaseQueryText("\"Hello World\" AND test"); got != "\"hello world\" AND test" {
		t.Errorf("got %q want %q", got, "\"hello world\" AND test")
	}
	// lowercase operator is a regular term, not kept upper
	if got := esoscommon.LowerCaseQueryText("virus or malware"); got != "virus or malware" {
		t.Errorf("got %q want %q", got, "virus or malware")
	}
}

func TestCopyFields(t *testing.T) {
	src := map[string]interface{}{"a": 1, "b": "x"}
	c := esoscommon.CopyFields(src)
	if &c == &src {
		t.Error("CopyFields must return a distinct map")
	}
	if c["a"] != 1 || c["b"] != "x" {
		t.Errorf("CopyFields content mismatch: %v", c)
	}
	c["a"] = 2
	if src["a"] != 1 {
		t.Error("CopyFields must be a shallow copy (top-level writes must not affect src)")
	}
}

func TestSortValuesEqual(t *testing.T) {
	if esoscommon.SortValuesEqual([]interface{}{1, 2}, []interface{}{2, 1}) {
		t.Error("order-dependent: [1,2] vs [2,1] must be false")
	}
	if esoscommon.SortValuesEqual([]interface{}{1, 2}, []interface{}{1, 3}) {
		t.Error("different values should be false")
	}
	// single-element and empty are adversarial edges
	if !esoscommon.SortValuesEqual([]interface{}{1}, []interface{}{1}) {
		t.Error("single element equal should be true")
	}
	if !esoscommon.SortValuesEqual([]interface{}{}, []interface{}{}) {
		t.Error("empty slices should be equal")
	}
	if esoscommon.SortValuesEqual([]interface{}{1}, []interface{}{1, 1}) {
		// differing lengths with same multiset of distinct values
		t.Error("different lengths should be false")
	}
}

func TestToFloat64(t *testing.T) {
	if v, ok := esoscommon.ToFloat64("1.5"); ok {
		t.Errorf("string 1.5 not supported, got (%v,%v)", v, ok)
	}
	if _, ok := esoscommon.ToFloat64("x"); ok {
		t.Error("non-numeric string should fail")
	}
	if _, ok := esoscommon.ToFloat64(nil); ok {
		t.Error("nil should fail")
	}
	if v, ok := esoscommon.ToFloat64(3); !ok || v != 3 {
		t.Errorf("int 3 = (%v,%v)", v, ok)
	}
	if v, ok := esoscommon.ToFloat64(3.5); !ok || v != 3.5 {
		t.Errorf("float 3.5 = (%v,%v)", v, ok)
	}
}

func TestConditionTermsList(t *testing.T) {
	if lst, ok := esoscommon.ConditionTermsList([]string{"a", "b"}); !ok || len(lst) != 2 {
		t.Errorf("string slice -> (%v,%v)", lst, ok)
	}
	if lst, ok := esoscommon.ConditionTermsList("a"); ok || lst != nil {
		t.Errorf("scalar string should fail, got (%v,%v)", lst, ok)
	}
	if _, ok := esoscommon.ConditionTermsList(5); ok {
		t.Error("unsupported type should fail (adversarial)")
	}
	if _, ok := esoscommon.ConditionTermsList(nil); ok {
		t.Error("nil should fail")
	}
}

func TestMapMemoryMessageESField(t *testing.T) {
	if got := esoscommon.MapMemoryMessageESField("message_type", false); got != "message_type_kwd" {
		t.Errorf("message_type -> %q", got)
	}
	if got := esoscommon.MapMemoryMessageESField("status", false); got != "status_int" {
		t.Errorf("status -> %q", got)
	}
	if got := esoscommon.MapMemoryMessageESField("content", true); got != "tokenized_content_ltks" {
		t.Errorf("content tokenized -> %q", got)
	}
	if got := esoscommon.MapMemoryMessageESField("content", false); got != "content_ltks" {
		t.Errorf("content plain -> %q", got)
	}
	// boost suffix preserved (coupled field pair)
	if got := esoscommon.MapMemoryMessageESField("content^2", false); got != "content_ltks^2" {
		t.Errorf("content boost -> %q", got)
	}
}

func TestMapMemoryMessageESFields(t *testing.T) {
	// deduplicates mapped fields
	got := esoscommon.MapMemoryMessageESFields([]string{"content", "content"}, false)
	if len(got) != 1 || got[0] != "content_ltks" {
		t.Errorf("dedup -> %v", got)
	}
	// empty input returned unchanged (adversarial)
	empty := esoscommon.MapMemoryMessageESFields([]string{}, false)
	if len(empty) != 0 {
		t.Errorf("empty -> %v", empty)
	}
}

func TestMapSkillSearchFields(t *testing.T) {
	got := esoscommon.MapSkillSearchFields([]string{"name", "description^3"})
	if len(got) != 2 || got[0] != "name_tks" || got[1] != "description_tks^3" {
		t.Errorf("skill fields -> %v", got)
	}
}

func TestFormatOrderedTagFeas(t *testing.T) {
	// nil -> (nil, false)
	if _, ok := esoscommon.FormatOrderedTagFeas(nil); ok {
		t.Error("nil should be false")
	}
	// map[string]int sorted by score desc, key asc tiebreak
	raw, ok := esoscommon.FormatOrderedTagFeas(map[string]int{"b": 1, "a": 2})
	if !ok {
		t.Fatal("map should encode")
	}
	var dec map[string]int
	if err := json.Unmarshal(raw, &dec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dec["a"] != 2 || dec["b"] != 1 {
		t.Errorf("ordering/values wrong: %v", dec)
	}
	// empty map -> "{}" with ok true
	if raw, ok := esoscommon.FormatOrderedTagFeas(map[string]int{}); !ok || string(raw) != "{}" {
		t.Errorf("empty map -> (%s,%v)", raw, ok)
	}
}

func TestParseMemoryMessageVectorSize(t *testing.T) {
	if dim, ok := esoscommon.ParseMemoryMessageVectorSize("q_128_vec"); !ok || dim != 128 {
		t.Errorf("q_128_vec -> (%v,%v)", dim, ok)
	}
	if _, ok := esoscommon.ParseMemoryMessageVectorSize("foo"); ok {
		t.Error("non-vector field should fail (adversarial)")
	}
	if _, ok := esoscommon.ParseMemoryMessageVectorSize("q_0_vec"); ok {
		t.Error("zero dim should fail (adversarial)")
	}
}

func TestMemoryMessageVectorField(t *testing.T) {
	if got := esoscommon.MemoryMessageVectorField(128); got == "" {
		t.Error("expected non-empty vector column name")
	}
}

func TestGetDefaultSkillMapping(t *testing.T) {
	m := esoscommon.GetDefaultSkillMapping(esoscommon.ESVectorField)
	if m == nil {
		t.Error("mapping must not be nil")
	}
}

func TestLoadSkillMapping(t *testing.T) {
	m, err := esoscommon.LoadSkillMapping("conf/skill_es_mapping.json", esoscommon.ESVectorField)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Error("mapping must not be nil")
	}
}

func TestBuildRankFeatureQuery(t *testing.T) {
	q := esoscommon.BuildRankFeatureQuery(map[string]float64{"pagerank": 0.5}, true)
	if len(q) == 0 {
		t.Error("expected at least one rank feature clause")
	}
	// empty input -> empty slice (adversarial)
	if q := esoscommon.BuildRankFeatureQuery(map[string]float64{}, true); len(q) != 0 {
		t.Errorf("empty input -> %v", q)
	}
}

func TestBuildQueryStringQuery(t *testing.T) {
	q := esoscommon.BuildQueryStringQuery(&types.MatchTextExpr{MatchingText: "foo"}, false, false)
	if q == nil {
		t.Error("query must not be nil")
	}
	if _, ok := q["query_string"]; !ok {
		t.Errorf("expected query_string key, got %v", q)
	}
}

func TestParseOrderByExpr(t *testing.T) {
	q := esoscommon.ParseOrderByExpr(&types.OrderByExpr{Fields: []types.OrderByField{{Field: "a", Type: types.SortAsc}}})
	if q == nil {
		t.Error("order by must not be nil")
	}
	if q := esoscommon.ParseOrderByExpr(nil); q != nil {
		t.Errorf("nil should yield nil, got %v", q)
	}
}

func TestGetChunkScore(t *testing.T) {
	if v := esoscommon.GetChunkScore(map[string]interface{}{"_score": 1.5}); v != 1.5 {
		t.Errorf("_score 1.5 -> %v", v)
	}
	if v := esoscommon.GetChunkScore(map[string]interface{}{}); v != 0 {
		t.Errorf("missing score -> %v", v)
	}
}

func TestSortByScore(t *testing.T) {
	in := []map[string]interface{}{
		{"_score": 1.0},
		{"_score": 3.0},
		{"_score": 2.0},
	}
	out := esoscommon.SortByScore(in, 2)
	if len(out) != 2 {
		t.Fatalf("limit 2 -> %d", len(out))
	}
	if out[0]["_score"].(float64) < out[1]["_score"].(float64) {
		t.Error("should be sorted by score desc")
	}
}

func TestSortByFields(t *testing.T) {
	in := []map[string]interface{}{
		{"a": 1.0},
		{"a": 3.0},
	}
	out := esoscommon.SortByFields(in, &types.OrderByExpr{Fields: []types.OrderByField{{Field: "a", Type: types.SortDesc}}})
	if len(out) != 2 || out[0]["a"].(float64) < out[1]["a"].(float64) {
		t.Errorf("sort by fields desc wrong: %v", out)
	}
}

func TestCalculateScores(t *testing.T) {
	in := []map[string]interface{}{
		{"score": 1.0, "pagerank": 0.5},
	}
	out := esoscommon.CalculateScores(in, "score", "pagerank")
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}
}

func TestNormalizeMemoryMessageChunks(t *testing.T) {
	in := []map[string]interface{}{
		{"content_ltks": "hi", "message_type_kwd": "chat", "status_int": 1, "q_1_vec": []float32{0.1}},
	}
	esoscommon.NormalizeMemoryMessageChunks(in)
	c := in[0]
	if c["content"] != "hi" {
		t.Errorf("content_ltks should map to content, got %v", c["content"])
	}
	if c["message_type"] != "chat" {
		t.Errorf("message_type_kwd should map to message_type, got %v", c["message_type"])
	}
	if c["status"] != true {
		t.Errorf("status_int 1 should map to status true, got %v", c["status"])
	}
	if c["content_embed"] == nil {
		t.Error("vector field q_1_vec should map to content_embed")
	}
	if _, ok := c["content_ltks"]; ok {
		t.Error("content_ltks should be deleted after remap")
	}
}

func TestMapMemoryMessageToESDoc(t *testing.T) {
	doc := esoscommon.MapMemoryMessageToESDoc(map[string]interface{}{"id": "1", "content": "x"})
	if doc == nil {
		t.Error("doc must not be nil")
	}
}

func TestMapMemoryMessageESConditionFields(t *testing.T) {
	out := esoscommon.MapMemoryMessageESConditionFields(map[string]interface{}{"message_type": "x"})
	if out == nil {
		t.Error("must not be nil")
	}
}

func TestMapMemoryMessageESUpdateFields(t *testing.T) {
	out := esoscommon.MapMemoryMessageESUpdateFields(map[string]interface{}{"content": "x"})
	if out == nil {
		t.Error("must not be nil")
	}
}

func TestMemoryMessageStatusBool(t *testing.T) {
	if !esoscommon.MemoryMessageStatusBool(true) {
		t.Error("true -> true")
	}
	if !esoscommon.MemoryMessageStatusBool("weird") {
		t.Error("non-empty string should be true")
	}
}

func TestMemoryMessageStatusInt(t *testing.T) {
	if v := esoscommon.MemoryMessageStatusInt(1); v != 1 {
		t.Errorf("int 1 -> %v", v)
	}
	if v := esoscommon.MemoryMessageStatusInt("active"); v != 1 {
		t.Errorf("string active -> %v", v)
	}
}

func TestMemoryMessageVectorProperty(t *testing.T) {
	p := esoscommon.MemoryMessageVectorProperty(128, esoscommon.ESVectorField)
	if p == nil {
		t.Error("property must not be nil")
	}
}

func TestGetMemoryMessageMapping(t *testing.T) {
	m := esoscommon.GetMemoryMessageMapping(128, esoscommon.ESVectorField)
	if m == nil {
		t.Error("mapping must not be nil")
	}
}
