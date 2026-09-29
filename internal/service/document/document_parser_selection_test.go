package document

import (
	"mime/multipart"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

func TestDefaultDocumentParserID(t *testing.T) {
	cases := []struct {
		name     string
		filetype utility.FileType
		filename string
		fallback string
		want     string
	}{
		{"image routes to picture", utility.FileTypeVISUAL, "photo.jpg", "naive", "picture"},
		{"video routes to picture", utility.FileTypeVISUAL, "clip.mp4", "naive", "picture"},
		{"audio routes to audio", utility.FileTypeAURAL, "talk.mp3", "naive", "audio"},
		{"pptx routes to presentation", utility.FileTypeDOC, "deck.pptx", "naive", "presentation"},
		{"legacy ppt routes to presentation", utility.FileTypeDOC, "deck.ppt", "naive", "presentation"},
		{"pages routes to presentation", utility.FileTypeDOC, "deck.pages", "naive", "presentation"},
		{"presentation extension is case-insensitive", utility.FileTypeDOC, "DECK.PPTX", "naive", "presentation"},
		{"eml routes to email", utility.FileTypeDOC, "mail.eml", "naive", "email"},
		{"msg routes to email", utility.FileTypeDOC, "mail.msg", "naive", "email"},
		{"pdf keeps the dataset parser", utility.FileTypePDF, "report.pdf", "naive", "naive"},
		{"text keeps the dataset parser", utility.FileTypeDOC, "notes.txt", "manual", "manual"},
		{"no substring false positive", utility.FileTypeDOC, "deck.pptx.md", "naive", "naive"},
		{"dedicated dataset parser is kept", utility.FileTypeVISUAL, "photo.jpg", "picture", "picture"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultDocumentParserID(tc.filetype, tc.filename, tc.fallback)
			if got != tc.want {
				t.Fatalf("defaultDocumentParserID(%q, %q, %q) = %q, want %q",
					tc.filetype, tc.filename, tc.fallback, got, tc.want)
			}
		})
	}
}

// TestUploadLocalDocuments_RoutesFileTypesToDedicatedParsers pins the Python
// FileService.get_parser parity: an uploaded image or presentation is re-pointed
// at the pipeline that owns its file type and carries that pipeline's document
// config (the config the parser-gap validation and the runtime read), while a
// plain text upload keeps the dataset parser and config untouched.
func TestUploadLocalDocuments_RoutesFileTypesToDedicatedParsers(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	mockStorage := newFakeUploadStorage()
	factory := storage.GetStorageFactory()
	origStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() { factory.SetStorage(origStorage) })

	if err := dao.DB.Create(&entity.Tenant{
		ID: "tenant-1", LLMID: "llm-default", EmbdID: "embd-default", ASRID: "asr-default",
	}).Error; err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	kbConfig := entity.JSONMap{
		"GeneralChunker:SixApplesFall": map[string]any{"chunk_token_size": 512},
		"metadata": map[string]any{
			"enabled":           true,
			"metadata":          []any{},
			"built_in_metadata": []any{},
		},
	}
	kb := &entity.Knowledgebase{
		ID:           "kb-route",
		TenantID:     "tenant-1",
		Name:         "kb-route",
		CreatedBy:    "user-1",
		ParserID:     "naive",
		ParserConfig: kbConfig,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}

	ctx := t.Context()
	svc := testDocumentService(t)
	files := []*multipart.FileHeader{
		makeTestFileHeader(t, "file", "photo.jpg", []byte("jpeg-bytes")),
		makeTestFileHeader(t, "file", "deck.pptx", []byte("pptx-bytes")),
		makeTestFileHeader(t, "file", "notes.txt", []byte("plain text")),
	}
	got, errs := svc.UploadLocalDocuments(ctx, kb, "user-1", files, "", nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(got) != len(files) {
		t.Fatalf("uploaded docs = %d, want %d", len(got), len(files))
	}

	if parserID, _ := got[0]["parser_id"].(string); parserID != "picture" {
		t.Fatalf("jpg parser_id = %q, want picture", parserID)
	}
	imageSetup := parserFamilySetup(t, got[0], "image")
	if imageSetup["parse_method"] != "ocr" {
		t.Fatalf("jpg image setup = %#v, want the picture pipeline defaults", imageSetup)
	}

	if parserID, _ := got[1]["parser_id"].(string); parserID != "presentation" {
		t.Fatalf("pptx parser_id = %q, want presentation", parserID)
	}
	slidesSetup := parserFamilySetup(t, got[1], "slides")
	if _, ok := slidesSetup["suffix"]; !ok {
		t.Fatalf("pptx slides setup = %#v, want the presentation pipeline defaults", slidesSetup)
	}

	// The rebuilt config goes through the same assembly as a parser switch, so
	// the tenant LLM id and the dataset metadata group reach the target
	// pipeline's Extractor node.
	for _, doc := range got[:2] {
		cfg := doc["parser_config"].(map[string]interface{})
		extractor, _ := cfg["Extractor:AutoExtractDefault"].(map[string]interface{})
		if extractor["llm_id"] != "llm-default" {
			t.Fatalf("%v parser_config extractor = %#v, want tenant llm id", doc["name"], extractor)
		}
		nodeMeta, _ := extractor["metadata"].(map[string]interface{})
		if nodeMeta["enabled"] != true {
			t.Fatalf("%v extractor metadata = %#v, want the dataset metadata group", doc["name"], extractor["metadata"])
		}
	}

	if parserID, _ := got[2]["parser_id"].(string); parserID != "naive" {
		t.Fatalf("txt parser_id = %q, want the dataset parser naive", parserID)
	}
	txtCfg, _ := got[2]["parser_config"].(map[string]interface{})
	if _, ok := txtCfg["GeneralChunker:SixApplesFall"]; !ok {
		t.Fatalf("txt parser_config = %#v, want the dataset config untouched", txtCfg)
	}
}

// parserFamilySetup returns the family setup of the document's Parser node, the
// shape the web parser-gap validation keys on.
func parserFamilySetup(t *testing.T, doc map[string]interface{}, family string) map[string]interface{} {
	t.Helper()
	cfg, ok := doc["parser_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("%v parser_config = %T, want map", doc["name"], doc["parser_config"])
	}
	for cpnID, raw := range cfg {
		if !strings.HasPrefix(cpnID, "Parser:") {
			continue
		}
		params, _ := raw.(map[string]interface{})
		if setup, ok := params[family].(map[string]interface{}); ok {
			return setup
		}
	}
	t.Fatalf("%v parser_config has no %q setup on a Parser node: %#v", doc["name"], family, cfg)
	return nil
}
