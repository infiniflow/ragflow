package dao

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// Test bridges keep migration internals out of the production API.
var MigrateIngestionParserConfigsForTest = migrateIngestionParserConfigs
var MigrateParserChunkerTokenizerConfigForTest = migrateParserChunkerTokenizerConfig
var MigrateExtractorConfigForTest = migrateExtractorConfig

func TestMigrateParserChunkerTokenizerConfig(t *testing.T) {
	cases := []struct {
		name, parser, input, want string
		pipeline                  *string
	}{
		{
			name:  "engine and chunk settings",
			input: `{"layout_recognize":true,"chunk_token_num":128,"delimiter":"\n","overlapped_percent":0.29,"table_context_size":100,"image_context_size":50,"filename_embd_weight":0.3}`,
			want:  `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"deepdoc"}},"GeneralChunker:SixApplesFall":{"chunk_token_size":128,"delimiters":["\n"],"overlapped_percent":0.29,"table_context_size":100,"image_context_size":50},"Tokenizer:LegalReadersDecide":{"filename_embd_weight":0.3}}`,
		},
		{name: "engine off", input: `{"layout_recognize":false}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"}}}`},
		{name: "engine spellings", input: `{"layout_recognize":"plain text"}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"}}}`},
		{name: "tcadp reaches both families", input: `{"layout_recognize":"TCADP Parser"}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"tcadp parser"},"spreadsheet":{"parse_method":"tcadp"}}}`},
		{name: "tcadp on a spreadsheet-only pipeline", parser: "table", input: `{"layout_recognize":"TCADP Parser"}`, want: `{"Parser:HipSignsRhyme":{"spreadsheet":{"parse_method":"tcadp"}}}`},
		{name: "tcadp result options follow the engine", parser: "table", input: `{"layout_recognize":"TCADP Parser","table_result_type":"2","markdown_image_response_type":"2"}`, want: `{"Parser:HipSignsRhyme":{"spreadsheet":{"markdown_image_response_type":"2","parse_method":"tcadp","table_result_type":"2"}}}`},
		{name: "tcadp result options without the engine", parser: "table", input: `{"layout_recognize":"DeepDOC","table_result_type":"2"}`, want: `{}`},
		{name: "model reference stays a layout recognizer", input: `{"layout_recognize":"bge-m3@mineru"}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"layout_recognizer":"bge-m3@mineru"}}}`},
		{name: "existing engine wins over old model reference", input: `{"layout_recognize":"foo@MinerU","Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"}}}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"}}}`},
		{name: "existing model reference wins over old engine", input: `{"layout_recognize":"DeepDOC","Parser:HipSignsRhyme":{"pdf":{"layout_recognizer":"foo@MinerU"}}}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"layout_recognizer":"foo@MinerU"}}}`},
		{name: "existing engine supersedes unsupported old engine", input: `{"layout_recognize":"MagicOCR","Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"}}}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"}}}`},
		{name: "PDF selector does not mask spreadsheet engine", input: `{"layout_recognize":"TCADP Parser","Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"}}}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text"},"spreadsheet":{"parse_method":"tcadp"}}}`},
		{name: "MinerU options reach the PDF setup", input: `{"layout_recognize":"MinerU","mineru_parse_method":"ocr","mineru_lang":"English"}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"mineru","mineru_parse_method":"ocr","mineru_lang":"English"}}}`},
		{name: "existing MinerU options win", input: `{"mineru_lang":"English","Parser:HipSignsRhyme":{"pdf":{"parse_method":"mineru","mineru_lang":"Chinese"}}}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"mineru","mineru_lang":"Chinese"}}}`},
		{name: "picture has no engine path", parser: "picture", input: `{"layout_recognize":"PaddleOCR"}`, want: `{}`},
		{name: "audio has no engine path", parser: "audio", input: `{"layout_recognize":true}`, want: `{}`},
		{name: "pages are normalized", input: `{"pages":[[3,5],[1,2],[6,7]]}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"pages":[[1,7]]}}}`},
		{name: "pages on a pdf-less pipeline", parser: "table", input: `{"pages":[[1,5]]}`, want: `{}`},
		{
			name:   "table columns move to the spreadsheet family",
			parser: "table",
			input:  `{"table_column_mode":"manual","table_column_roles":{"a":"vectorize","b":"metadata","c":"nonsense"},"table_column_names":["a","b",7]}`,
			want:   `{"Parser:HipSignsRhyme":{"spreadsheet":{"column_mode":"manual","column_roles":{"a":"indexing","b":"metadata"},"column_names":["a","b"]}}}`,
		},
		{name: "table columns need a spreadsheet family", parser: "paper", input: `{"table_column_mode":"manual"}`, want: `{}`},
		{name: "zero chunk size on the general chunker", input: `{"chunk_token_num":0}`, want: `{"GeneralChunker:SixApplesFall":{"chunk_token_size":0}}`},
		{name: "zero chunk size on the token chunker", parser: "email", input: `{"chunk_token_num":0}`, want: `{}`},
		{name: "chunk size reached the email chunker", parser: "email", input: `{"chunk_token_num":256}`, want: `{"TokenChunker:WarmBreadSmells":{"chunk_token_size":256}}`},
		{name: "numeric string chunk size", input: `{"chunk_token_num":"256"}`, want: `{"GeneralChunker:SixApplesFall":{"chunk_token_size":256}}`},
		{name: "unusable chunk size is dropped", input: `{"chunk_token_num":"many"}`, want: `{}`},
		{name: "title chunker cap", parser: "book", input: `{"chunk_token_num":256}`, want: `{"TitleChunker:GrumpyGarlicsBake":{"chunk_token_cap":256}}`},
		{name: "title chunker cap out of range", parser: "book", input: `{"chunk_token_num":64}`, want: `{}`},
		{name: "manual chunker has no cap", parser: "manual", input: `{"chunk_token_num":512}`, want: `{}`},
		{name: "delimiter with backticks and unicode", input: "{\"delimiter\":\"`--`。？\"}", want: `{"GeneralChunker:SixApplesFall":{"delimiters":["` + "`--`" + `","。","？"]}}`},
		{name: "delimiter on a title chunker", parser: "book", input: `{"delimiter":"\n"}`, want: `{}`},
		{name: "context sizes need the general or token chunker", parser: "book", input: `{"table_context_size":10,"image_context_size":20}`, want: `{}`},
		{name: "overlap on a title chunker", parser: "book", input: `{"overlapped_percent":0.29}`, want: `{}`},
		{name: "parent child group", input: `{"parent_child":{"use_parent_child":true,"children_delimiter":"。"}}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"。"},"children_delimiters":["。"]}}`},
		{name: "legacy parent child flags", input: `{"enable_children":true,"children_delimiter":"。；"}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"。；"},"children_delimiters":["。","；"]}}`},
		{name: "parent child switched off", input: `{"enable_children":false}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":false,"children_delimiter":"\n"},"children_delimiters":[]}}`},
		{name: "group shape wins over the legacy flag", input: `{"parent_child":{"use_parent_child":false,"children_delimiter":"|"},"enable_children":true,"children_delimiter":"。"}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":false,"children_delimiter":"|"},"children_delimiters":[]}}`},
		{name: "parent child delimiters default to a newline", input: `{"parent_child":{"use_parent_child":true}}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"\n"},"children_delimiters":["\n"]}}`},
		{name: "parent child on a QA chunker", parser: "qa", input: `{"enable_children":true}`, want: `{}`},
		{name: "effective Python child delimiter", input: `{"children_delimiter":"。"}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"。"},"children_delimiters":["。"]}}`},
		{name: "escaped Python child delimiter", input: `{"enable_children":true,"children_delimiter":"\\n"}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"\n"},"children_delimiters":["\n"]}}`},
		{name: "explicit empty child delimiter stays disabled", input: `{"parent_child":{"use_parent_child":true,"children_delimiter":""}}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":""},"children_delimiters":[]}}`},
		{name: "existing disabled child group wins", input: `{"enable_children":true,"children_delimiter":"。","GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":false,"children_delimiter":"|"}}}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":false,"children_delimiter":"|"},"children_delimiters":[]}}`},
		{name: "existing enabled child group wins", input: `{"enable_children":false,"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"|"}}}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"|"},"children_delimiters":["|"]}}`},
		{name: "explicit runtime child delimiters win", input: `{"enable_children":true,"children_delimiter":"。","GeneralChunker:SixApplesFall":{"children_delimiters":["|"]}}`, want: `{"GeneralChunker:SixApplesFall":{"parent_child":{"use_parent_child":true,"children_delimiter":"。"},"children_delimiters":["|"]}}`},
		{name: "filename weight", parser: "picture", input: `{"filename_embd_weight":0}`, want: `{"Tokenizer:SharpLensFocus":{"filename_embd_weight":0}}`},
		{name: "null filename weight", parser: "picture", input: `{"filename_embd_weight":null}`, want: `{}`},
		{name: "null engine", input: `{"layout_recognize":null}`, want: `{}`},
		{name: "naive alias", parser: "naive", input: `{"layout_recognize":true}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"deepdoc"}}}`},
		{name: "presentation pdf family", parser: "presentation", input: `{"layout_recognize":true}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"deepdoc"}}}`},
		{name: "book carries its remove_toc", parser: "book", input: `{"layout_recognize":true}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"deepdoc","remove_toc":true}}}`},
		{name: "book carries remove_toc alongside pages", parser: "book", input: `{"pages":[[2,4]]}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"pages":[[2,4]],"remove_toc":true}}}`},
		{name: "laws carries its remove_toc", parser: "laws", input: `{"layout_recognize":"plaintext"}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text","remove_toc":true}}}`},
		{name: "paper carries multi column", parser: "paper", input: `{"layout_recognize":"plaintext"}`, want: `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"plain_text","enable_multi_column":true}}}`},
		{
			name:  "existing node params win",
			input: `{"layout_recognize":true,"chunk_token_num":4096,"filename_embd_weight":0.9,"Parser:HipSignsRhyme":{"pdf":{"parse_method":"somark"}},"GeneralChunker:SixApplesFall":{"chunk_token_size":512,"delimiters":["\n"]},"Tokenizer:LegalReadersDecide":{"filename_embd_weight":0.1}}`,
			want:  `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"somark"}},"GeneralChunker:SixApplesFall":{"chunk_token_size":512,"delimiters":["\n"]},"Tokenizer:LegalReadersDecide":{"filename_embd_weight":0.1}}`,
		},
		{
			name:   "an existing family map is merged, not replaced",
			parser: "book",
			input:  `{"layout_recognize":true,"Parser:HipSignsRhyme":{"pdf":{"preprocess":["main_content"],"remove_toc":false}}}`,
			want:   `{"Parser:HipSignsRhyme":{"pdf":{"preprocess":["main_content"],"remove_toc":false,"parse_method":"deepdoc"}}}`,
		},
		{name: "retired settings are deleted", input: `{"html4excel":true,"task_page_size":256,"analyze_hyperlink":true,"hyperlink_urls":true,"video_prompt":"x","mineru_formula_enable":true}`, want: `{}`},
		{name: "no owned keys", input: `{"chunk_method":"naive","topn_tags":5}`, want: `{"chunk_method":"naive","topn_tags":5}`},
		{name: "empty config", input: `{}`, want: `{}`},
		{name: "canvas rows are skipped", pipeline: ptrString("canvas-1"), input: `{"layout_recognize":true,"chunk_token_num":128}`, want: `{"layout_recognize":true,"chunk_token_num":128}`},
		{name: "unknown parser ids are preserved", parser: "knowledge_graph", input: `{"layout_recognize":true,"chunk_token_num":128}`, want: `{"layout_recognize":true,"chunk_token_num":128}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config, want := decodeIngestionConfig(t, tc.input), decodeIngestionConfig(t, tc.want)
			parser := tc.parser
			if parser == "" {
				parser = "general"
			}
			if _, err := migrateParserChunkerTokenizerConfig(config, parser, tc.pipeline, "knowledgebase", "row-1"); err != nil {
				t.Fatal(err)
			}
			if got := decodeIngestionConfig(t, encodeIngestionConfig(t, config)); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s; want %s", encodeIngestionConfig(t, config), tc.want)
			}
			snapshot := encodeIngestionConfig(t, config)
			if changed, err := migrateParserChunkerTokenizerConfig(config, parser, tc.pipeline, "knowledgebase", "row-1"); err != nil || changed {
				t.Fatalf("second run changed=%v, err=%v", changed, err)
			}
			if encodeIngestionConfig(t, config) != snapshot {
				t.Fatal("second run changed config")
			}
		})
	}
}

func TestMigrateParserChunkerTokenizerConfigRejectsMalformedValues(t *testing.T) {
	for _, raw := range []string{
		`{"layout_recognize":"MagicOCR"}`,
		`{"layout_recognize":"foo@mistral ocr"}`,
		`{"layout_recognize":"model@instance@somark"}`,
		`{"layout_recognize":"model@instance@opendataloader"}`,
		`{"layout_recognize":"model@instance@paddleocr"}`,
		`{"pages":"1-5"}`,
		`{"pages":[[0,5]]}`,
		`{"pages":[[5,1]]}`,
		`{"layout_recognize":5}`,
		`{"delimiter":5}`,
		`{"parent_child":"yes"}`,
		`{"parent_child":{"use_parent_child":"true"}}`,
		`{"enable_children":"true"}`,
		`{"children_delimiter":5}`,
		`{"table_column_mode":5}`,
		`{"table_column_roles":"a"}`,
		`{"table_column_names":"a"}`,
		`{"filename_embd_weight":"heavy"}`,
		`{"table_result_type":2}`,
		`{"layout_recognize":""}`,
	} {
		t.Run(raw, func(t *testing.T) {
			config := decodeIngestionConfig(t, raw)
			before := encodeIngestionConfig(t, config)
			if _, err := migrateParserChunkerTokenizerConfig(config, "general", nil, "knowledgebase", "row-1"); err == nil {
				t.Fatalf("malformed value was accepted: %s", before)
			}
			if encodeIngestionConfig(t, config) != before {
				t.Fatal("a rejected row was modified")
			}
		})
	}
}

func TestMigrateParserChunkerTokenizerConfigLogsRetiredFields(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	previous := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = previous })
	config := decodeIngestionConfig(t, `{"html4excel":true,"task_page_size":12,"mineru_formula_enable":false}`)
	if _, err := migrateParserChunkerTokenizerConfig(config, "general", nil, "document", "doc-1"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"html4excel", "task_page_size", "mineru_formula_enable"} {
		entries := logs.FilterField(zap.String("field", field)).All()
		if len(entries) != 1 || entries[0].ContextMap()["table"] != "document" || entries[0].ContextMap()["row_id"] != "doc-1" {
			t.Errorf("missing row-specific warning for %s: %v", field, entries)
		}
	}
}

func TestMigrateIngestionParserConfigsBatches(t *testing.T) {
	db := openIngestionMigrationDB(t)
	for i := 0; i < 257; i++ {
		for _, table := range []string{"knowledgebase", "document"} {
			insertIngestionConfig(t, db, table, fmt.Sprintf("%032d", i), "general", python027IngestionConfig)
		}
	}
	if err := migrateIngestionParserConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"knowledgebase", "document"} {
		var rows []ingestionConfigMigrationRow
		if err := db.Table(table).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) != 257 {
			t.Fatalf("%s rows=%d", table, len(rows))
		}
		for _, row := range rows {
			config := decodeIngestionConfig(t, *row.ParserConfig)
			extractor, _ := config[ingestionTargets["general"].extractorNode].(map[string]any)
			keywords, _ := extractor["keywords"].(map[string]any)
			if keywords["top_n"] != float64(8) {
				t.Fatalf("%s.%s: extractor settings not migrated: %v", table, row.ID, config)
			}
			parserNode, ok := config["Parser:HipSignsRhyme"].(map[string]any)
			if !ok {
				t.Fatalf("%s.%s: parser node missing: %v", table, row.ID, config)
			}
			pdf, _ := parserNode["pdf"].(map[string]any)
			if pdf["parse_method"] != "plain_text" {
				t.Fatalf("%s.%s: pdf=%v", table, row.ID, pdf)
			}
			chunker, _ := config["GeneralChunker:SixApplesFall"].(map[string]any)
			if chunker["chunk_token_size"] != float64(256) {
				t.Fatalf("%s.%s: chunker=%v", table, row.ID, chunker)
			}
			if _, leftovers := config["chunk_token_num"]; leftovers {
				t.Fatalf("%s.%s: flat key survived: %v", table, row.ID, config)
			}
		}
	}
	if got := databaseVersion(t, db); got != ingestionParserConfigTargetVersion {
		t.Fatalf("version=%s", got)
	}
}

func TestMigrateIngestionParserConfigsRejectsInvalidRows(t *testing.T) {
	for _, raw := range []string{
		`{"pages":[[9,2]]}`,
		`not-json`,
		`{"auto_keywords":"8","auto_questions":2}`,
		`{"enable_metadata":true,"metadata":[{"name":"author"}]}`,
		`{"enable_metadata":true,"metadata":{"type":"object","required":["author"],"properties":{"author":{"type":"object","properties":{"name":{"type":"string"}}}}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			db := openIngestionMigrationDB(t)
			insertIngestionConfig(t, db, "knowledgebase", "a", "general", python027IngestionConfig)
			insertIngestionConfig(t, db, "knowledgebase", "b", "general", raw)
			if err := migrateIngestionParserConfigs(t.Context(), db); err == nil {
				t.Fatal("invalid input was marked as migrated")
			}
			var rows []ingestionConfigMigrationRow
			if err := db.Table("knowledgebase").Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if *rows[0].ParserConfig != python027IngestionConfig || *rows[1].ParserConfig != raw {
				t.Fatal("failed batch modified source configs")
			}
			if databaseVersion(t, db) != "" {
				t.Fatal("failed migration advanced version")
			}
		})
	}
}

func TestMigrateIngestionParserConfigsVersionAndEmptyRows(t *testing.T) {
	for _, tc := range []struct {
		version string
		skip    bool
	}{
		{"", false}, {"v0.27.2", false}, {"v1.0.0-rc1", false},
		{"v1.0.0-rc2.dev2", true},
		{ingestionParserConfigTargetVersion, true}, {"v1.0.0", true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			db := openIngestionMigrationDB(t)
			if tc.version != "" {
				writeDatabaseVersion(t, db, tc.version)
			}
			untouched := []string{``, `null`, `{}`, `{"Parser:HipSignsRhyme":{"pdf":{"parse_method":"somark"}}}`, `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false}}}`}
			for i, raw := range untouched {
				insertIngestionConfig(t, db, "knowledgebase", fmt.Sprint(i), "general", raw)
			}
			insertIngestionConfig(t, db, "knowledgebase", "old", "general", python027IngestionConfig)
			if err := migrateIngestionParserConfigs(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			var rows []ingestionConfigMigrationRow
			if err := db.Table("knowledgebase").Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			for i, raw := range untouched {
				if *rows[i].ParserConfig != raw {
					t.Fatalf("untouched row %d rewritten: %s", i, *rows[i].ParserConfig)
				}
			}
			unchanged := *rows[len(untouched)].ParserConfig == python027IngestionConfig
			if unchanged != tc.skip {
				t.Fatalf("version gate: skipped=%v; want %v", unchanged, tc.skip)
			}
			wantVersion := ingestionParserConfigTargetVersion
			if tc.skip {
				wantVersion = tc.version
			}
			if databaseVersion(t, db) != wantVersion {
				t.Fatal("unexpected version")
			}
		})
	}
}

// python027IngestionConfig is a dataset parser_config as Python v0.27.2 stored
// it: every value differs from the Go default so a lost field is visible.
const python027IngestionConfig = `{"auto_keywords":8,"layout_recognize":"plain text","chunk_token_num":256,"delimiter":"\n","overlapped_percent":0.29,"filename_embd_weight":0.2,"pages":[[1,3]],"table_context_size":10,"image_context_size":20,"html4excel":false}`

func openIngestionMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertIngestionConfig(t *testing.T, db *gorm.DB, table, id, parserID, raw string) {
	t.Helper()
	values := map[string]any{"id": id, "parser_id": parserID, "parser_config": raw, "created_by": "user"}
	if table == "knowledgebase" {
		values["tenant_id"] = "tenant"
		values["name"] = "dataset"
		values["embd_id"] = "embedding"
	} else {
		values["kb_id"] = "dataset"
		values["source_type"] = "local"
		values["type"] = "doc"
		values["suffix"] = "pdf"
	}
	if err := db.Table(table).Create(values).Error; err != nil {
		t.Fatal(err)
	}
}

func decodeIngestionConfig(t *testing.T, raw string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func encodeIngestionConfig(t *testing.T, config map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const python027ParserConfig = `{"auto_keywords":8,"auto_questions":3,"enable_metadata":true,"metadata":[{"key":"author","description":"who wrote it"}],"built_in_metadata":[{"key":"update_time"}],"topn_tags":5,"tag_kb_ids":["tag-kb"]}`

func TestMigrateExtractorConfig(t *testing.T) {
	cases := []struct {
		name, parser, input, want string
		pipeline                  *string
	}{
		{name: "Python fields", parser: "naive", input: python027ParserConfig, want: `{"Extractor:AutoExtractDefault":{"keywords":{"top_n":8},"questions":{"top_n":3},"metadata":{"enabled":true,"metadata":[{"key":"author","description":"who wrote it"}],"built_in_metadata":[{"key":"update_time"}]}},"topn_tags":5,"tag_kb_ids":["tag-kb"]}`},
		{name: "missing metadata toggle stays disabled", input: `{"metadata":[{"key":"author"}]}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false,"metadata":[{"key":"author"}],"built_in_metadata":[]}}}`},
		{name: "explicit zero and false", input: `{"auto_keywords":0,"enable_metadata":false,"metadata":[]}`, want: `{"Extractor:AutoExtractDefault":{"keywords":{"top_n":0},"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`},
		{name: "existing node values win over Python fields", input: `{"auto_keywords":8,"enable_metadata":true,"metadata":[{"key":"author"}],"Extractor:AutoExtractDefault":{"keywords":{"top_n":2,"enabled":false},"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`, want: `{"Extractor:AutoExtractDefault":{"keywords":{"top_n":2,"enabled":false},"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`},
		{name: "modular transport is authoritative", input: `{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[]},"Extractor:AutoExtractDefault":{"metadata":{"enabled":false}}}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[]}}}`},
		{name: "empty modular transport disables metadata", input: `{"metadata":{},"Extractor:AutoExtractDefault":{"metadata":{"enabled":true}}}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`},
		{name: "flat schema", input: `{"enable_metadata":true,"metadata":{"type":"object","properties":{"author":{"type":"string","description":"who","enum":["A"]}},"additionalProperties":false}}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author","type":"string","description":"who","enum":["A"]}],"built_in_metadata":[]}}}`},
		{name: "description alias", input: `{"enable_metadata":true,"metadata":[{"key":"author","descriptions":"who"}]}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author","description":"who"}],"built_in_metadata":[]}}}`},
		{name: "tag settings untouched", input: `{"topn_tags":5,"tag_kb_ids":["tag-kb"],"tags":{"tag_file_id":"file"}}`, want: `{"topn_tags":5,"tag_kb_ids":["tag-kb"],"tags":{"tag_file_id":"file"}}`},
		{name: "early Go nodes untouched", input: `{"Extractor:AutoExtractDefault":{"auto_keywords":8,"keywords_sys_prompt":"saved","enable_summary":true,"sys_prompt":"saved summary","auto_tags":5,"tag_file_id":"file","metadata_config":{"enabled":true}}}`, want: `{"Extractor:AutoExtractDefault":{"auto_keywords":8,"keywords_sys_prompt":"saved","enable_summary":true,"sys_prompt":"saved summary","auto_tags":5,"tag_file_id":"file","metadata_config":{"enabled":true}}}`},
		{name: "custom node", pipeline: ptrString("canvas-1"), input: `{"auto_questions":2,"Extractor:Custom":{"summary":{"enabled":true}}}`, want: `{"Extractor:Custom":{"questions":{"top_n":2},"summary":{"enabled":true}}}`},
		{name: "QA has no target", parser: "qa", input: python027ParserConfig, want: python027ParserConfig},
		{name: "table has no target", parser: "table", input: python027ParserConfig, want: python027ParserConfig},
		{name: "QA ignores stale extractor", parser: "qa", input: `{"auto_keywords":8,"Extractor:Old":{}}`, want: `{"auto_keywords":8,"Extractor:Old":{}}`},
		{name: "table ignores stale extractor", parser: "table", input: `{"auto_keywords":8,"Extractor:Old":{}}`, want: `{"auto_keywords":8,"Extractor:Old":{}}`},
		{name: "canvas has no target", pipeline: ptrString("canvas-1"), input: python027ParserConfig, want: python027ParserConfig},
		{name: "empty row", input: `{}`, want: `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config, want := decodeIngestionConfig(t, tc.input), decodeIngestionConfig(t, tc.want)
			parser := tc.parser
			if parser == "" {
				parser = "general"
			}
			if _, err := migrateExtractorConfig(config, parser, tc.pipeline); err != nil {
				t.Fatal(err)
			}
			if got := decodeIngestionConfig(t, encodeIngestionConfig(t, config)); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s; want %s", encodeIngestionConfig(t, config), tc.want)
			}
			snapshot := encodeIngestionConfig(t, config)
			if changed, err := migrateExtractorConfig(config, parser, tc.pipeline); err != nil || changed {
				t.Fatalf("second run changed=%v, err=%v", changed, err)
			}
			if encodeIngestionConfig(t, config) != snapshot {
				t.Fatal("second run changed config")
			}
		})
	}
}

func ptrString(value string) *string { return &value }

func TestMigrateIngestionParserConfigsSharesDev1Cleanup(t *testing.T) {
	if ingestionParserConfigTargetVersion != "v1.0.0-rc2.dev1" {
		t.Fatalf("migration version = %s", ingestionParserConfigTargetVersion)
	}
	db := openIngestionMigrationDB(t)
	insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{"raptor":{},"auto_keywords":8}`)
	if err := migrateIngestionParserConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	var row ingestionConfigMigrationRow
	if err := db.Table("knowledgebase").Where("id = ?", "kb").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	config := decodeIngestionConfig(t, *row.ParserConfig)
	if _, remains := config["raptor"]; remains {
		t.Fatal("dataset cleanup was skipped")
	}
	if _, migrated := config[ingestionTargets["general"].extractorNode]; !migrated {
		t.Fatal("component conversion was skipped")
	}
}

func TestMigrateIngestionParserConfigsUpdateError(t *testing.T) {
	for _, table := range []string{"knowledgebase", "document"} {
		t.Run(table, func(t *testing.T) {
			db := openIngestionMigrationDB(t)
			for _, id := range []string{"row-a", "row-b"} {
				insertIngestionConfig(t, db, table, id, "general", python027IngestionConfig)
			}
			trigger := fmt.Sprintf(`CREATE TRIGGER reject_parser_config_update
				BEFORE UPDATE OF parser_config ON %s WHEN OLD.id = 'row-b'
				BEGIN SELECT RAISE(FAIL, 'forced parser config update failure'); END`, table)
			if err := db.Exec(trigger).Error; err != nil {
				t.Fatal(err)
			}
			err := migrateIngestionParserConfigs(t.Context(), db)
			if err == nil {
				t.Fatal("expected update failure")
			}
			for _, detail := range []string{table, "row-b", "forced parser config update failure"} {
				if !strings.Contains(err.Error(), detail) {
					t.Errorf("error %q does not identify %q", err, detail)
				}
			}
			var rows []ingestionConfigMigrationRow
			if err := db.Table(table).Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if *row.ParserConfig != python027IngestionConfig {
					t.Fatalf("failed batch changed row %s", row.ID)
				}
			}
			if got := databaseVersion(t, db); got != "" {
				t.Fatalf("failed migration advanced version to %q", got)
			}
		})
	}
}
