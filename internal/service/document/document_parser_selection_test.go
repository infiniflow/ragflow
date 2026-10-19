package document

import (
	"mime/multipart"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	syncerconnector "ragflow/internal/syncer/connector"
	"ragflow/internal/utility"
)

func TestDedicatedDocumentParserID(t *testing.T) {
	cases := []struct {
		name     string
		filetype utility.FileType
		filename string
		want     string
	}{
		{"image routes to picture", utility.FileTypeVISUAL, "photo.jpg", "picture"},
		{"video routes to picture", utility.FileTypeVISUAL, "clip.mp4", "picture"},
		{"audio routes to audio", utility.FileTypeAURAL, "talk.mp3", "audio"},
		{"pptx routes to presentation", utility.FileTypeDOC, "deck.pptx", "presentation"},
		{"legacy ppt routes to presentation", utility.FileTypeDOC, "deck.ppt", "presentation"},
		{"pages routes to presentation", utility.FileTypeDOC, "deck.pages", "presentation"},
		{"presentation extension is case-insensitive", utility.FileTypeDOC, "DECK.PPTX", "presentation"},
		{"eml routes to email", utility.FileTypeDOC, "mail.eml", "email"},
		{"msg routes to email", utility.FileTypeDOC, "mail.msg", "email"},
		{"pdf has no dedicated pipeline", utility.FileTypePDF, "report.pdf", ""},
		{"text has no dedicated pipeline", utility.FileTypeDOC, "notes.txt", ""},
		{"no substring false positive", utility.FileTypeDOC, "deck.pptx.md", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dedicatedDocumentParserID(tc.filetype, tc.filename); got != tc.want {
				t.Fatalf("dedicatedDocumentParserID(%q, %q) = %q, want %q",
					tc.filetype, tc.filename, got, tc.want)
			}
		})
	}
}

func TestConfigDeclaredFamily(t *testing.T) {
	flat := entity.JSONMap{
		"Parser:HipSignsRhyme": map[string]any{
			"pdf":                       map[string]any{"suffix": []any{"pdf"}},
			"text&code":                 map[string]any{},
			"vlm":                       map[string]any{},
			"enable_vision_enhancement": false,
		},
	}
	nested := entity.JSONMap{
		"Parser:HipSignsRhyme": map[string]any{
			"setups": map[string]any{
				"pdf":    map[string]any{},
				"slides": map[string]any{},
			},
		},
	}
	// A config never carries two Parser components today; the union keeps the
	// answer independent of map iteration order if one ever does.
	multiple := entity.JSONMap{
		"Parser:One": map[string]any{"pdf": map[string]any{}},
		"Parser:Two": map[string]any{"slides": map[string]any{}},
	}

	cases := []struct {
		name         string
		config       entity.JSONMap
		family       string
		wantDeclared bool
		wantFound    bool
	}{
		{"flat declares the family", flat, "pdf", true, true},
		{"flat misses the family", flat, "slides", false, true},
		{"nested legacy setups are lifted", nested, "slides", true, true},
		{"nested legacy setups miss the family", nested, "text&code", false, true},
		{"any parser entry declaring the family wins", multiple, "slides", true, true},
		{"an entry missing the family still answers", multiple, "audio", false, true},
		{"no parser entry cannot answer", entity.JSONMap{"File": map[string]any{}}, "pdf", false, false},
		{"nil config cannot answer", nil, "pdf", false, false},
		{"non-map parser entry cannot answer", entity.JSONMap{"Parser:X": "oops"}, "pdf", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared, found := configDeclaredFamily(tc.config, tc.family)
			if declared != tc.wantDeclared || found != tc.wantFound {
				t.Fatalf("configDeclaredFamily(%#v, %q) = (%v, %v), want (%v, %v)",
					tc.config, tc.family, declared, found, tc.wantDeclared, tc.wantFound)
			}
		})
	}
}

func TestSelectionGovernsFamilyMissesAnUndeclaredFamily(t *testing.T) {
	sel := service.CurrentSelection("presentation", nil)
	if selectionGovernsFamily(t.Context(), sel, presentationParserConfig(), "text&code") {
		t.Fatal("selectionGovernsFamily reported an undeclared family as declared")
	}
}

func presentationParserConfig() entity.JSONMap {
	return entity.JSONMap{
		"Parser:HipSignsRhyme": map[string]any{
			"pdf":    map[string]any{"suffix": []any{"pdf"}},
			"slides": map[string]any{"suffix": []any{"pptx", "ppt"}},
		},
	}
}

func generalParserConfig() entity.JSONMap {
	return entity.JSONMap{
		"Parser:HipSignsRhyme": map[string]any{
			"pdf":       map[string]any{"suffix": []any{"pdf"}},
			"text&code": map[string]any{},
			"docx":      map[string]any{},
			"image":     map[string]any{"suffix": []any{"jpg"}},
			"email":     map[string]any{"suffix": []any{"eml", "msg"}},
		},
	}
}

func pictureParserConfig() entity.JSONMap {
	return entity.JSONMap{
		"Parser:ViewsCaptureLight": map[string]any{
			"image": map[string]any{"suffix": []any{"jpg"}},
			"video": map[string]any{"suffix": []any{"mp4"}},
		},
	}
}

func tableParserConfig() entity.JSONMap {
	return entity.JSONMap{
		"Parser:TableRoundStone": map[string]any{
			"spreadsheet": map[string]any{"suffix": []any{"xlsx"}},
			"text&code":   map[string]any{},
		},
	}
}

// TestResolveDocumentParser_Priority pins the resolution order: the dataset's
// selection governs the families its config declares, an undeclared family goes
// to the builtin pipeline that owns it, and failing that to general. Undetermined
// support (a canvas without a Parser node) keeps the selection, and so does an
// extension the pipeline vocabulary cannot name unless an owning pipeline takes
// it.
func TestResolveDocumentParser_Priority(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := dao.DB.Create(&entity.Tenant{ID: "tenant-1", LLMID: "llm-default"}).Error; err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	canvasID := "canvas-1"
	legacyConfig := entity.JSONMap{"llm_id": "ministral", "chunk_token_num": 512}
	canvasWithoutParser := entity.JSONMap{"File": map[string]any{}}

	cases := []struct {
		name               string
		kbParser           string
		kbPipelineID       *string
		kbConfig           entity.JSONMap
		filename           string
		filetype           utility.FileType
		wantParserID       string
		wantPipeline       string
		wantFamilyIn       string
		wantFamilyOut      string
		wantBaseConfigKept bool
	}{
		{
			name:          "general keeps a text family it declares",
			kbParser:      "general",
			kbConfig:      generalParserConfig(),
			filename:      "laws.txt",
			filetype:      utility.FileTypeDOC,
			wantParserID:  "general",
			wantFamilyIn:  "text&code",
			wantFamilyOut: "slides",
		},
		{
			name:          "presentation keeps a pdf it declares",
			kbParser:      "presentation",
			kbConfig:      presentationParserConfig(),
			filename:      "report.pdf",
			filetype:      utility.FileTypePDF,
			wantParserID:  "presentation",
			wantFamilyIn:  "slides",
			wantFamilyOut: "text&code",
		},
		{
			name:          "presentation falls back to general for text",
			kbParser:      "presentation",
			kbConfig:      presentationParserConfig(),
			filename:      "laws.txt",
			filetype:      utility.FileTypeDOC,
			wantParserID:  "general",
			wantFamilyIn:  "text&code",
			wantFamilyOut: "slides",
		},
		{
			name:          "table falls back to general for pdf",
			kbParser:      "table",
			kbConfig:      tableParserConfig(),
			filename:      "report.pdf",
			filetype:      utility.FileTypePDF,
			wantParserID:  "general",
			wantFamilyIn:  "docx",
			wantFamilyOut: "",
		},
		{
			name:          "picture owns an image the selection does not declare",
			kbParser:      "presentation",
			kbConfig:      presentationParserConfig(),
			filename:      "photo.jpg",
			filetype:      utility.FileTypeVISUAL,
			wantParserID:  "picture",
			wantFamilyIn:  "image",
			wantFamilyOut: "slides",
		},
		{
			name:          "audio owns audio the selection does not declare",
			kbParser:      "picture",
			kbConfig:      pictureParserConfig(),
			filename:      "talk.mp3",
			filetype:      utility.FileTypeAURAL,
			wantParserID:  "audio",
			wantFamilyIn:  "audio",
			wantFamilyOut: "image",
		},
		{
			name:          "legacy flat config is judged by its template",
			kbParser:      "naive",
			kbConfig:      legacyConfig,
			filename:      "laws.txt",
			filetype:      utility.FileTypeDOC,
			wantParserID:  "naive",
			wantFamilyIn:  "",
			wantFamilyOut: "",
		},
		{
			name:          "legacy flat config still routes audio",
			kbParser:      "naive",
			kbConfig:      legacyConfig,
			filename:      "talk.mp3",
			filetype:      utility.FileTypeAURAL,
			wantParserID:  "audio",
			wantFamilyIn:  "audio",
			wantFamilyOut: "",
		},
		{
			name:          "legacy flat config still routes an owned family",
			kbParser:      "naive",
			kbConfig:      legacyConfig,
			filename:      "deck.pptx",
			filetype:      utility.FileTypeDOC,
			wantParserID:  "presentation",
			wantFamilyIn:  "slides",
			wantFamilyOut: "",
		},
		{
			name:          "legacy flat config keeps an image in general",
			kbParser:      "naive",
			kbConfig:      legacyConfig,
			filename:      "photo.jpg",
			filetype:      utility.FileTypeVISUAL,
			wantParserID:  "naive",
			wantFamilyIn:  "",
			wantFamilyOut: "",
		},
		{
			name:          "canvas keeps a family it declares",
			kbParser:      "presentation",
			kbPipelineID:  &canvasID,
			kbConfig:      presentationParserConfig(),
			filename:      "report.pdf",
			filetype:      utility.FileTypePDF,
			wantParserID:  "",
			wantPipeline:  canvasID,
			wantFamilyIn:  "slides",
			wantFamilyOut: "",
		},
		{
			name:          "canvas falls back to general for an undeclared family",
			kbParser:      "presentation",
			kbPipelineID:  &canvasID,
			kbConfig:      presentationParserConfig(),
			filename:      "laws.txt",
			filetype:      utility.FileTypeDOC,
			wantParserID:  "general",
			wantPipeline:  "",
			wantFamilyIn:  "text&code",
			wantFamilyOut: "slides",
		},
		{
			name:         "canvas without a parser node keeps the canvas",
			kbParser:     "presentation",
			kbPipelineID: &canvasID,
			kbConfig:     canvasWithoutParser,
			filename:     "laws.txt",
			filetype:     utility.FileTypeDOC,
			wantParserID: "",
			wantPipeline: canvasID,
		},
		{
			name:         "canvas without a parser node owns nothing either",
			kbParser:     "presentation",
			kbPipelineID: &canvasID,
			kbConfig:     canvasWithoutParser,
			filename:     "photo.jpg",
			filetype:     utility.FileTypeVISUAL,
			wantParserID: "",
			wantPipeline: canvasID,
		},
		{
			name:         "canvas selects into general for an unowned family",
			kbParser:     "presentation",
			kbPipelineID: &canvasID,
			kbConfig:     pictureParserConfig(),
			filename:     "report.pdf",
			filetype:     utility.FileTypePDF,
			wantParserID: "general",
			wantFamilyIn: "text&code",
		},
		{
			name:         "an unmappable extension still reaches its owner pipeline",
			kbParser:     "general",
			kbConfig:     generalParserConfig(),
			filename:     "deck.pages",
			filetype:     utility.FileTypeDOC,
			wantParserID: "presentation",
			wantFamilyIn: "slides",
		},
		{
			name:         "an unmappable visual extension still reaches picture",
			kbParser:     "general",
			kbConfig:     generalParserConfig(),
			filename:     "clip.rm",
			filetype:     utility.FileTypeVISUAL,
			wantParserID: "picture",
			wantFamilyIn: "video",
		},
		{
			name:         "an unmappable audio extension still reaches audio",
			kbParser:     "general",
			kbConfig:     generalParserConfig(),
			filename:     "song.vorbis",
			filetype:     utility.FileTypeAURAL,
			wantParserID: "audio",
			wantFamilyIn: "audio",
		},
		{
			name:               "an unmappable extension with no owner keeps the selection",
			kbParser:           "general",
			kbConfig:           generalParserConfig(),
			filename:           "notes.rtf",
			filetype:           utility.FileTypeDOC,
			wantParserID:       "general",
			wantBaseConfigKept: true,
		},
		{
			name:               "an owner already selected keeps its edited config",
			kbParser:           "picture",
			kbConfig:           pictureParserConfig(),
			filename:           "clip.psd",
			filetype:           utility.FileTypeVISUAL,
			wantParserID:       "picture",
			wantBaseConfigKept: true,
		},
		{
			name:         "a canvas loses an unmappable extension to its owner",
			kbParser:     "presentation",
			kbPipelineID: &canvasID,
			kbConfig:     presentationParserConfig(),
			filename:     "clip.rm",
			filetype:     utility.FileTypeVISUAL,
			wantParserID: "picture",
			wantFamilyIn: "video",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kb := &entity.Knowledgebase{
				ID:           "kb-" + tc.name,
				TenantID:     "tenant-1",
				Name:         "kb",
				CreatedBy:    "user-1",
				ParserID:     tc.kbParser,
				PipelineID:   tc.kbPipelineID,
				ParserConfig: tc.kbConfig,
			}
			base := cloneParserConfigForDocument(tc.kbConfig)

			selection, gotConfig := resolveDocumentParser(t.Context(), kb, tc.filename, tc.filetype, base)

			gotParser, gotPipelinePtr := parserSelectionColumns(selection)
			gotPipeline := ""
			if gotPipelinePtr != nil {
				gotPipeline = *gotPipelinePtr
			}
			if gotParser != tc.wantParserID || gotPipeline != tc.wantPipeline {
				t.Fatalf("selection = (%q, %q), want (%q, %q)",
					gotParser, gotPipeline, tc.wantParserID, tc.wantPipeline)
			}
			if tc.wantBaseConfigKept && !reflect.DeepEqual(gotConfig, base) {
				t.Fatalf("parser_config = %#v, want the base config kept", gotConfig)
			}
			if tc.wantFamilyIn != "" && !configHasParserFamily(gotConfig, tc.wantFamilyIn) {
				t.Fatalf("parser_config misses family %q: %#v", tc.wantFamilyIn, gotConfig)
			}
			if tc.wantFamilyOut != "" && configHasParserFamily(gotConfig, tc.wantFamilyOut) {
				t.Fatalf("parser_config unexpectedly keeps family %q: %#v", tc.wantFamilyOut, gotConfig)
			}
		})
	}
}

// TestUploadLocalDocuments_RoutesFileTypesToDedicatedParsers pins the Python
// FileService.get_parser parity: an uploaded presentation or audio file is
// re-pointed at the pipeline that owns its file type and carries that
// pipeline's document config, while image/email/text uploads stay on the
// dataset parser now that the general template declares those families.
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
		"Parser:HipSignsRhyme": map[string]any{
			"pdf":       map[string]any{"suffix": []any{"pdf"}},
			"text&code": map[string]any{},
			"image":     map[string]any{"ocr_enabled": true, "suffix": []any{"jpg"}},
			"email":     map[string]any{"suffix": []any{"eml", "msg"}},
		},
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
		makeTestFileHeader(t, "file", "note.eml", []byte("eml-bytes")),
		makeTestFileHeader(t, "file", "talk.mp3", []byte("mp3-bytes")),
		makeTestFileHeader(t, "file", "notes.txt", []byte("plain text")),
	}
	got, errs := svc.UploadLocalDocuments(ctx, kb, "user-1", files, "", nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(got) != len(files) {
		t.Fatalf("uploaded docs = %d, want %d", len(got), len(files))
	}

	if parserID, _ := got[0]["parser_id"].(string); parserID != "naive" {
		t.Fatalf("jpg parser_id = %q, want the dataset parser naive", parserID)
	}
	imageSetup := parserFamilySetup(t, got[0], "image")
	if imageSetup["ocr_enabled"] != true {
		t.Fatalf("jpg image setup = %#v, want the general pipeline defaults", imageSetup)
	}

	if parserID, _ := got[1]["parser_id"].(string); parserID != "presentation" {
		t.Fatalf("pptx parser_id = %q, want presentation", parserID)
	}
	slidesSetup := parserFamilySetup(t, got[1], "slides")
	if _, ok := slidesSetup["suffix"]; !ok {
		t.Fatalf("pptx slides setup = %#v, want the presentation pipeline defaults", slidesSetup)
	}

	if parserID, _ := got[2]["parser_id"].(string); parserID != "naive" {
		t.Fatalf("eml parser_id = %q, want the dataset parser naive", parserID)
	}
	parserFamilySetup(t, got[2], "email")

	if parserID, _ := got[3]["parser_id"].(string); parserID != "audio" {
		t.Fatalf("mp3 parser_id = %q, want audio", parserID)
	}

	// The rebuilt configs go through the same assembly as a parser switch, so
	// the tenant LLM id and the dataset metadata group reach each target
	// pipeline's Extractor node.
	for _, doc := range []map[string]interface{}{got[1], got[3]} {
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

	if parserID, _ := got[4]["parser_id"].(string); parserID != "naive" {
		t.Fatalf("txt parser_id = %q, want the dataset parser naive", parserID)
	}
	txtCfg, _ := got[4]["parser_config"].(map[string]interface{})
	if _, ok := txtCfg["GeneralChunker:SixApplesFall"]; !ok {
		t.Fatalf("txt parser_config = %#v, want the dataset config untouched", txtCfg)
	}
}

// TestUploadLocalDocuments_FallsBackWhenSelectionLacksFamily covers the upload
// path end to end: a text file uploaded to a dataset whose selection declares
// only pdf and slides is reseeded with the catch-all general pipeline, and a
// canvas dataset keeps a family its canvas declares while falling back for the
// rest.
func TestUploadLocalDocuments_FallsBackWhenSelectionLacksFamily(t *testing.T) {
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

	ctx := t.Context()
	svc := testDocumentService(t)

	presentationKB := &entity.Knowledgebase{
		ID:           "kb-presentation",
		TenantID:     "tenant-1",
		Name:         "kb-presentation",
		CreatedBy:    "user-1",
		ParserID:     "presentation",
		ParserConfig: presentationParserConfig(),
	}
	if err := dao.DB.Create(presentationKB).Error; err != nil {
		t.Fatalf("insert presentation kb: %v", err)
	}
	got, errs := svc.UploadLocalDocuments(ctx, presentationKB, "user-1", []*multipart.FileHeader{
		makeTestFileHeader(t, "file", "laws.txt", []byte("plain text")),
		makeTestFileHeader(t, "file", "deck.pptx", []byte("pptx-bytes")),
	}, "", nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if parserID, _ := got[0]["parser_id"].(string); parserID != "general" {
		t.Fatalf("txt parser_id = %q, want general", parserID)
	}
	parserFamilySetup(t, got[0], "text&code")
	if parserID, _ := got[1]["parser_id"].(string); parserID != "presentation" {
		t.Fatalf("pptx parser_id = %q, want presentation", parserID)
	}
	parserFamilySetup(t, got[1], "slides")

	canvasID := "canvas-1"
	canvasKB := &entity.Knowledgebase{
		ID:           "kb-canvas",
		TenantID:     "tenant-1",
		Name:         "kb-canvas",
		CreatedBy:    "user-1",
		ParserID:     "presentation",
		PipelineID:   &canvasID,
		ParserConfig: presentationParserConfig(),
	}
	if err := dao.DB.Create(canvasKB).Error; err != nil {
		t.Fatalf("insert canvas kb: %v", err)
	}
	got, errs = svc.UploadLocalDocuments(ctx, canvasKB, "user-1", []*multipart.FileHeader{
		makeTestFileHeader(t, "file", "report.pdf", []byte("%PDF-1.4")),
		makeTestFileHeader(t, "file", "laws.txt", []byte("plain text")),
	}, "", nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	// The canvas declares pdf: the document stays on the canvas with parser_id
	// blanked and the canvas carried on pipeline_id.
	if parserID, _ := got[0]["parser_id"].(string); parserID != "" {
		t.Fatalf("canvas pdf parser_id = %q, want blank", parserID)
	}
	if pipelineID, _ := got[0]["pipeline_id"].(string); pipelineID != canvasID {
		t.Fatalf("canvas pdf pipeline_id = %q, want %q", pipelineID, canvasID)
	}
	parserFamilySetup(t, got[0], "pdf")
	// The canvas does not declare text&code: the document leaves the canvas for
	// general.
	if parserID, _ := got[1]["parser_id"].(string); parserID != "general" {
		t.Fatalf("canvas txt parser_id = %q, want general", parserID)
	}
	if pipelineID, ok := got[1]["pipeline_id"].(string); ok && pipelineID != "" {
		t.Fatalf("canvas txt pipeline_id = %q, want none", pipelineID)
	}
	parserFamilySetup(t, got[1], "text&code")
}

// TestInsertSyncDocument_FallsBackWhenSelectionLacksFamily covers the connector
// path: it shares resolveDocumentParser with the upload path, so a synced text
// file lands on general too.
func TestInsertSyncDocument_FallsBackWhenSelectionLacksFamily(t *testing.T) {
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

	kb := &entity.Knowledgebase{
		ID:           "kb-sync",
		TenantID:     "tenant-1",
		Name:         "kb-sync",
		CreatedBy:    "user-1",
		ParserID:     "presentation",
		ParserConfig: presentationParserConfig(),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}

	input := service.DocumentUpsertInput{
		TaskContext:    dao.SyncTaskContext{Knowledgebase: *kb},
		SourceType:     "connector",
		DocumentID:     "doc-sync-1",
		SourceDocument: syncerconnector.SourceDocument{Blob: []byte("plain text")},
	}
	svc := testDocumentService(t)
	res, err := svc.insertSyncDocument(t.Context(), input, "laws.txt", "", "txt", "", "tenant-1")
	if err != nil {
		t.Fatalf("insertSyncDocument: %v", err)
	}

	doc, err := svc.documentDAO.GetByID(t.Context(), dao.DB, res.DocID)
	if err != nil {
		t.Fatalf("load synced document: %v", err)
	}
	if doc.ParserID != "general" {
		t.Fatalf("synced txt parser_id = %q, want general", doc.ParserID)
	}
	if doc.PipelineID != nil {
		t.Fatalf("synced txt pipeline_id = %v, want nil", *doc.PipelineID)
	}
	if !configHasParserFamily(doc.ParserConfig, "text&code") {
		t.Fatalf("synced txt parser_config misses text&code: %#v", doc.ParserConfig)
	}
}

// configHasParserFamily reports whether the config's Parser entry declares the
// given family, read through the production declaration reader.
func configHasParserFamily(config entity.JSONMap, family string) bool {
	declared, _ := configDeclaredFamily(config, family)
	return declared
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
