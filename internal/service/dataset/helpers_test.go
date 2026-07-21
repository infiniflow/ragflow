package dataset

import (
	"strings"
	"testing"

	"ragflow/internal/service"
)

// TestValidateParserID_AcceptsRegistryRefs verifies that every
// canonical builtin pipeline id passes validation.
func TestValidateParserID_AcceptsRegistryRefs(t *testing.T) {
	for _, id := range []string{"general", "book", "audio", "qa", "table", "tag"} {
		if err := validateParserID(id); err != nil {
			t.Errorf("validateParserID(%q) = %v, want nil", id, err)
		}
	}
}

// TestValidateParserID_AcceptsNaiveAlias verifies the legacy
// parser_id "naive" still validates (alias for general).
func TestValidateParserID_AcceptsNaiveAlias(t *testing.T) {
	if err := validateParserID("naive"); err != nil {
		t.Errorf("validateParserID(naive) = %v, want nil (alias for general)", err)
	}
}

// TestValidateParserID_RejectsUnknown verifies unknown/empty
// values are rejected with an error that lists the valid options.
func TestValidateParserID_RejectsUnknown(t *testing.T) {
	for _, id := range []string{"", "unknown", "NAIVE"} {
		err := validateParserID(id)
		if err == nil {
			t.Errorf("validateParserID(%q) = nil, want error", id)
		}
	}

	err := validateParserID("unknown")
	if err == nil {
		t.Fatal("expected error for unknown parser id")
	}
	msg := err.Error()
	if !strings.Contains(msg, "general") {
		t.Errorf("error message %q should mention general", msg)
	}
}

// --- validateDatasetAvatar ---

func TestValidateDatasetAvatar_MissingPrefix(t *testing.T) {
	err := validateDatasetAvatar("iVBORw0KGgo=")
	if err == nil {
		t.Fatal("expected error for missing MIME prefix")
	}
}

func TestValidateDatasetAvatar_InvalidPrefix(t *testing.T) {
	err := validateDatasetAvatar("wrong:image/png;base64,iVBORw0KGgo=")
	if err == nil {
		t.Fatal("expected error for invalid prefix")
	}
}

func TestValidateDatasetAvatar_UnsupportedMIME(t *testing.T) {
	err := validateDatasetAvatar("data:image/gif;base64,iVBORw0KGgo=")
	if err == nil {
		t.Fatal("expected error for unsupported MIME")
	}
}

func TestValidateDatasetAvatar_Valid(t *testing.T) {
	err := validateDatasetAvatar("data:image/png;base64,iVBORw0KGgo=")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	err = validateDatasetAvatar("data:image/jpeg;base64,/9j/4AAQ==")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// --- validateDatasetEmbeddingModel ---

func TestValidateDatasetEmbeddingModel_Empty(t *testing.T) {
	err := validateDatasetEmbeddingModel("")
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestValidateDatasetEmbeddingModel_NameOnlyNoProvider(t *testing.T) {
	// A bare model name without @provider (and not a 32-char hex model ID) is
	// rejected, mirroring the Python contract.
	if err := validateDatasetEmbeddingModel("BAAI/bge-large-zh-v1.5"); err == nil {
		t.Fatal("expected error for name without @provider")
	}
}

func TestValidateDatasetEmbeddingModel_HexModelID(t *testing.T) {
	if err := validateDatasetEmbeddingModel("aabbccdd11223344aabbccdd11223344"); err != nil {
		t.Fatalf("expected nil for 32-char hex model ID, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModel_NameWithProvider(t *testing.T) {
	if err := validateDatasetEmbeddingModel("BAAI/bge-large-zh-v1.5@Builtin"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModel_EmptyPart(t *testing.T) {
	err := validateDatasetEmbeddingModel("model@")
	if err == nil {
		t.Fatal("expected error for empty provider")
	}
	err = validateDatasetEmbeddingModel("@provider")
	if err == nil {
		t.Fatal("expected error for empty model name")
	}
}

// --- normalizeDatasetPipelineID ---

func TestNormalizeDatasetPipelineID_Empty(t *testing.T) {
	result, err := normalizeDatasetPipelineID("")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for empty input")
	}
}

func TestNormalizeDatasetPipelineID_Spaces(t *testing.T) {
	result, err := normalizeDatasetPipelineID("  ")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for whitespace-only input")
	}
}

func TestNormalizeDatasetPipelineID_WrongLength(t *testing.T) {
	_, err := normalizeDatasetPipelineID("abc123")
	if err == nil {
		t.Fatal("expected error for wrong length")
	}
}

func TestNormalizeDatasetPipelineID_InvalidChars(t *testing.T) {
	_, err := normalizeDatasetPipelineID("abcdef01-23456789abcdef0123456789")
	if err == nil {
		t.Fatal("expected error for non-hex chars")
	}
}

func TestNormalizeDatasetPipelineID_Valid(t *testing.T) {
	result, err := normalizeDatasetPipelineID("ABCDEF0123456789ABCDEF0123456789")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != "abcdef0123456789abcdef0123456789" {
		t.Errorf("expected lowercased, got %q", *result)
	}
}

// --- validateDatasetParserConfigSize ---

func TestValidateDatasetParserConfigSize_Empty(t *testing.T) {
	if err := validateDatasetParserConfigSize(map[string]interface{}{}); err != nil {
		t.Fatalf("expected nil for empty, got %v", err)
	}
}

func TestValidateDatasetParserConfigSize_UnderLimit(t *testing.T) {
	cfg := map[string]interface{}{
		"Parser:abc": map[string]interface{}{
			"chunk_size": float64(512),
		},
	}
	if err := validateDatasetParserConfigSize(cfg); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetParserConfigSize_OverLimit(t *testing.T) {
	// Build a config that exceeds 65535 bytes.
	bigVal := strings.Repeat("x", 66000)
	cfg := map[string]interface{}{
		"Parser:abc": map[string]interface{}{
			"big_field": bigVal,
		},
	}
	err := validateDatasetParserConfigSize(cfg)
	if err == nil {
		t.Fatal("expected error for oversized parser_config")
	}
	if !strings.Contains(err.Error(), "exceeds size limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- normalizeDatasetID ---

func TestNormalizeDatasetID_Invalid(t *testing.T) {
	_, err := normalizeDatasetID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
	_, err = normalizeDatasetID("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestNormalizeDatasetID_Valid(t *testing.T) {
	raw := "550e8400-e29b-41d4-a716-446655440000"
	result, err := normalizeDatasetID(raw)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	expected := "550e8400e29b41d4a716446655440000"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestNormalizeDatasetID_StripsHyphens(t *testing.T) {
	// UUID with hyphens already removed.
	raw := "550e8400e29b41d4a716446655440000"
	result, err := normalizeDatasetID(raw)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if result != raw {
		t.Errorf("expected %q, got %q", raw, result)
	}
}

// --- normalizeDatasetUpdateExt ---

func TestNormalizeDatasetUpdateExt_Nil(t *testing.T) {
	if result := normalizeDatasetUpdateExt(nil); result != nil {
		t.Fatalf("expected nil for nil input")
	}
}

func TestNormalizeDatasetUpdateExt_PassesThrough(t *testing.T) {
	ext := map[string]interface{}{
		"description": "test",
		"language":    "English",
	}
	result := normalizeDatasetUpdateExt(ext)
	if result["description"] != "test" {
		t.Errorf("expected description preserved, got %v", result["description"])
	}
	if result["language"] != "English" {
		t.Errorf("expected language preserved, got %v", result["language"])
	}
}

func TestNormalizeDatasetUpdateExt_RenamesChunkMethod(t *testing.T) {
	ext := map[string]interface{}{
		"chunk_method": "book",
	}
	result := normalizeDatasetUpdateExt(ext)
	if result["parser_id"] != "book" {
		t.Errorf("expected parser_id=book, got %v", result["parser_id"])
	}
	if _, ok := result["chunk_method"]; ok {
		t.Error("expected chunk_method to be renamed to parser_id")
	}
}

func TestNormalizeDatasetUpdateExt_SkipsTokenAndChunkNum(t *testing.T) {
	ext := map[string]interface{}{
		"token_num":     float64(1000),
		"chunk_num":     float64(50),
		"parser_config": map[string]interface{}{"key": "val"},
	}
	result := normalizeDatasetUpdateExt(ext)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestNormalizeDatasetUpdateExt_ConvertsPagerank(t *testing.T) {
	ext := map[string]interface{}{
		"pagerank": float64(3),
	}
	result := normalizeDatasetUpdateExt(ext)
	if result["pagerank"] != int64(3) {
		t.Errorf("expected pagerank=int64(3), got %T(%v)", result["pagerank"], result["pagerank"])
	}
}

func TestNormalizeDatasetUpdateExt_NonFloatPagerankSkipped(t *testing.T) {
	// Non-float64 pagerank values are not convertible and are dropped.
	ext := map[string]interface{}{
		"pagerank": "auto",
	}
	result := normalizeDatasetUpdateExt(ext)
	if _, ok := result["pagerank"]; ok {
		t.Error("expected non-float pagerank to be skipped")
	}
}

// --- normalizeMetadataConfigFields ---

func TestNormalizeMetadataConfigFields_EmptyKey(t *testing.T) {
	fields := []service.MetadataConfigField{
		{Key: "", Type: "string"},
	}
	_, err := normalizeMetadataConfigFields(fields, "metadata")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestNormalizeMetadataConfigFields_KeyTooLong(t *testing.T) {
	longKey := strings.Repeat("k", 256)
	fields := []service.MetadataConfigField{
		{Key: longKey, Type: "string"},
	}
	_, err := normalizeMetadataConfigFields(fields, "metadata")
	if err == nil {
		t.Fatal("expected error for too-long key")
	}
}

func TestNormalizeMetadataConfigFields_InvalidType(t *testing.T) {
	fields := []service.MetadataConfigField{
		{Key: "my_field", Type: "boolean"},
	}
	_, err := normalizeMetadataConfigFields(fields, "metadata")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestNormalizeMetadataConfigFields_DescriptionTooLong(t *testing.T) {
	longDesc := strings.Repeat("d", 65536)
	fields := []service.MetadataConfigField{
		{Key: "my_field", Type: "string", Description: &longDesc},
	}
	_, err := normalizeMetadataConfigFields(fields, "metadata")
	if err == nil {
		t.Fatal("expected error for too-long description")
	}
}

func TestNormalizeMetadataConfigFields_Valid(t *testing.T) {
	desc := "A description"
	fields := []service.MetadataConfigField{
		{Key: "field1", Type: "string", Description: &desc},
		{Key: "field2", Type: "list"},
	}
	result, err := normalizeMetadataConfigFields(fields, "metadata")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(result))
	}
	if result[0]["key"] != "field1" {
		t.Errorf("expected key=field1, got %v", result[0]["key"])
	}
	if result[0]["type"] != "string" {
		t.Errorf("expected type=string, got %v", result[0]["type"])
	}
	if result[0]["description"] != &desc {
		t.Errorf("expected description preserved")
	}
	if result[1]["key"] != "field2" {
		t.Errorf("expected key=field2, got %v", result[1]["key"])
	}
	if result[1]["type"] != "list" {
		t.Errorf("expected type=list, got %v", result[1]["type"])
	}
}

func TestNormalizeMetadataConfigFields_TrimsKey(t *testing.T) {
	fields := []service.MetadataConfigField{
		{Key: "  my_field  ", Type: "number"},
	}
	result, err := normalizeMetadataConfigFields(fields, "metadata")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if result[0]["key"] != "my_field" {
		t.Errorf("expected trimmed key 'my_field', got %v", result[0]["key"])
	}
}
