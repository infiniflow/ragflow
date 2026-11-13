package dao

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func readLegacyParserConfig(t *testing.T, db *gorm.DB, table, id string) map[string]any {
	t.Helper()
	var row ingestionConfigMigrationRow
	if err := db.Table(table).Select("id", "parser_config").Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ParserConfig == nil {
		return nil
	}
	return decodeIngestionConfig(t, *row.ParserConfig)
}

func readCanvasDSL(t *testing.T, db *gorm.DB, table, id string) map[string]any {
	t.Helper()
	var row canvasDSLMigrationRow
	if err := db.Table(table).Select("id", "dsl").Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.DSL == nil {
		return nil
	}
	return decodeIngestionConfig(t, *row.DSL)
}

func TestIngestionLegacyConfigTargetVersionOrder(t *testing.T) {
	current := parseMigrationVersion(ingestionLegacyConfigTargetVersion)
	previous := parseMigrationVersion(resourceUserIDMigrationTargetVersion)
	if current == "" || previous == "" {
		t.Fatalf("unparseable version: current=%q previous=%q", current, previous)
	}
	if compareMigrationVersion(current, previous) <= 0 {
		t.Fatalf("legacy ingestion target %s must sort after resource target %s",
			ingestionLegacyConfigTargetVersion, resourceUserIDMigrationTargetVersion)
	}
}

func TestMigrateIngestionLegacyConfigsNodeLevel(t *testing.T) {
	db := openIngestionMigrationDB(t)
	writeDatabaseVersion(t, db, "v1.0.0.1")
	const raw = `{
		"Extractor:AutoExtractDefault": {
			"auto_keywords": 4,
			"keywords_sys_prompt": "kw prompt",
			"auto_questions": 2,
			"questions_sys_prompt": "q prompt",
			"auto_tags": 1,
			"tag_file_id": "tag-file",
			"enable_summary": 1,
			"sys_prompt": "summary prompt",
			"metadata_config": {
				"enabled": true,
				"metadata": [{"key": "author"}],
				"built_in_metadata": [{"key": "file_name"}]
			}
		},
		"Extractor:FieldsOnly": {
			"fields": [{"key": "node_field"}],
			"enable_metadata": true
		},
		"TitleChunker:GrumpyGarlicsBake": {
			"method": "group",
			"rules": [{"levels": [{"expression": "^#"}, {"expression": "^##"}]}]
		},
		"TitleChunker:NullLevels": {
			"method": "group",
			"levels": null,
			"rules": [{"levels": [{"expression": "^###"}]}]
		},
		"TitleChunker:BadLevels": {
			"method": "group",
			"levels": ["not-a-group"],
			"rules": [{"levels": [{"expression": "^####"}]}]
		},
		"TitleChunker:EmptyLevels": {
			"method": "group",
			"levels": [],
			"rules": [{"levels": [{"expression": "^#####"}]}]
		},
		"TokenChunker:WarmBreadSmells": {
			"delimiter_mode": "token_size",
			"chunk_token_size": 512
		},
		"TokenChunker:OneChunk": {
			"delimiter_mode": "one"
		},
		"TokenChunker:NullMode": {
			"delimiter_mode": null,
			"chunk_token_size": 128
		}
	}`
	for _, table := range []string{"knowledgebase", "document"} {
		insertIngestionConfig(t, db, table, "row", "general", raw)
	}

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	want := decodeIngestionConfig(t, `{
		"Extractor:AutoExtractDefault": {
			"keywords": {"top_n": 4, "system_prompt": "kw prompt"},
			"questions": {"top_n": 2, "system_prompt": "q prompt"},
			"tags": {"top_n": 1, "tag_file_id": "tag-file"},
			"summary": {"enabled": true, "system_prompt": "summary prompt"},
			"metadata": {
				"enabled": true,
				"metadata": [{"key": "author"}],
				"built_in_metadata": [{"key": "file_name"}]
			}
		},
		"Extractor:FieldsOnly": {
			"metadata": {
				"enabled": true,
				"metadata": [{"key": "node_field"}],
				"built_in_metadata": []
			}
		},
		"TitleChunker:GrumpyGarlicsBake": {
			"method": "group",
			"levels": [["^#", "^##"]]
		},
		"TitleChunker:NullLevels": {
			"method": "group",
			"levels": [["^###"]]
		},
		"TitleChunker:BadLevels": {
			"method": "group",
			"levels": [["^####"]]
		},
		"TitleChunker:EmptyLevels": {
			"method": "group",
			"levels": [["^#####"]]
		},
		"TokenChunker:WarmBreadSmells": {
			"delimiter_mode": "delimiter",
			"chunk_token_size": 512
		},
		"TokenChunker:OneChunk": {
			"delimiter_mode": "one"
		},
		"TokenChunker:NullMode": {
			"delimiter_mode": null,
			"chunk_token_size": 128
		}
	}`)
	for _, table := range []string{"knowledgebase", "document"} {
		if got := readLegacyParserConfig(t, db, table, "row"); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s config = %#v; want %#v", table, got, want)
		}
		before := encodeIngestionConfig(t, readLegacyParserConfig(t, db, table, "row"))
		if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		after := encodeIngestionConfig(t, readLegacyParserConfig(t, db, table, "row"))
		if after != before {
			t.Fatalf("%s second run changed config:\nbefore %s\nafter  %s", table, before, after)
		}
	}
	if got := databaseVersion(t, db); got != ingestionLegacyConfigTargetVersion {
		t.Fatalf("version = %q; want %q", got, ingestionLegacyConfigTargetVersion)
	}
}

// TestMigrateIngestionLegacyConfigsFieldNameRouting keeps the prompt of a
// pre-modular one-field-per-node Extractor with the group that node extracted.
// The same node shape appears in parser_config snapshots and canvas DSLs.
func TestMigrateIngestionLegacyConfigsFieldNameRouting(t *testing.T) {
	db := openIngestionMigrationDB(t)
	const nodes = `{
		"Extractor:Summary": {"field_name": "summary", "sys_prompt": "summary prompt"},
		"Extractor:Keywords": {"field_name": "keywords", "sys_prompt": "keywords prompt"},
		"Extractor:Questions": {"field_name": "questions", "sys_prompt": "questions prompt"},
		"Extractor:Metadata": {
			"field_name": "metadata",
			"sys_prompt": "metadata prompt",
			"metadata_config": {"enabled": true, "metadata": [{"key": "author"}], "built_in_metadata": []}
		},
		"Extractor:Unnamed": {"sys_prompt": "plain prompt"}
	}`
	insertIngestionConfig(t, db, "document", "row", "naive", nodes)
	insertCanvasDSL(t, db, "user_canvas", "canvas", `{
		"components": {
			"Extractor:Summary": {"obj": {"component_name": "Extractor", "params": {"field_name": "summary", "sys_prompt": "summary prompt"}}},
			"Extractor:Keywords": {"obj": {"component_name": "Extractor", "params": {"field_name": "keywords", "sys_prompt": "keywords prompt"}}},
			"Extractor:Metadata": {"obj": {"component_name": "Extractor", "params": {"field_name": "metadata", "sys_prompt": "metadata prompt", "metadata_config": {"enabled": true, "metadata": [{"key": "author"}], "built_in_metadata": []}}}}
		},
		"graph": {"nodes": [
			{"id": "Extractor:Summary", "data": {"label": "Extractor", "form": {"field_name": "summary", "sys_prompt": "summary prompt"}}},
			{"id": "Extractor:Keywords", "data": {"label": "Extractor", "form": {"field_name": "keywords", "sys_prompt": "keywords prompt"}}},
			{"id": "Extractor:Metadata", "data": {"label": "Extractor", "form": {"field_name": "metadata", "sys_prompt": "metadata prompt", "metadata_config": {"enabled": true, "metadata": [{"key": "author"}], "built_in_metadata": []}}}}
		]}
	}`)

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	want := decodeIngestionConfig(t, `{
		"Extractor:Summary": {"summary": {"enabled": true, "system_prompt": "summary prompt"}},
		"Extractor:Keywords": {"keywords": {"system_prompt": "keywords prompt"}},
		"Extractor:Questions": {"questions": {"system_prompt": "questions prompt"}},
		"Extractor:Metadata": {"metadata": {"enabled": true, "metadata": [{"key": "author"}], "built_in_metadata": []}},
		"Extractor:Unnamed": {"summary": {"system_prompt": "plain prompt"}}
	}`)
	if got := readLegacyParserConfig(t, db, "document", "row"); !reflect.DeepEqual(got, want) {
		t.Fatalf("document config = %#v; want %#v", got, want)
	}

	dsl := readCanvasDSL(t, db, "user_canvas", "canvas")
	wantCanvas := decodeIngestionConfig(t, `{
		"Extractor:Summary": {"summary": {"enabled": true, "system_prompt": "summary prompt"}},
		"Extractor:Keywords": {"keywords": {"system_prompt": "keywords prompt"}},
		"Extractor:Metadata": {"metadata": {"enabled": true, "metadata": [{"key": "author"}], "built_in_metadata": []}}
	}`)
	components := dsl["components"].(map[string]any)
	for id, wantParams := range wantCanvas {
		params := components[id].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
		if !reflect.DeepEqual(params, wantParams) {
			t.Fatalf("%s params = %#v; want %#v", id, params, wantParams)
		}
	}
	for _, raw := range dsl["graph"].(map[string]any)["nodes"].([]any) {
		node := raw.(map[string]any)
		id := node["id"].(string)
		form := node["data"].(map[string]any)["form"].(map[string]any)
		if !reflect.DeepEqual(form, wantCanvas[id]) {
			t.Fatalf("%s form = %#v; want %#v", id, form, wantCanvas[id])
		}
	}
}

func TestMigrateIngestionLegacyConfigsPrecedence(t *testing.T) {
	db := openIngestionMigrationDB(t)
	insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{
		"Extractor:AutoExtractDefault": {
			"auto_keywords": 9,
			"keywords_sys_prompt": "flat kw",
			"keywords": {"top_n": 2, "system_prompt": "node kw"},
			"enable_summary": 0,
			"field_name": "summary",
			"sys_prompt": "flat summary",
			"summary": {"system_prompt": "node summary"},
			"auto_tags": 5,
			"tag_file_id": "flat-tag",
			"tags": {"top_n": 1, "tag_file_id": "node-tag"},
			"metadata": {"enabled": false, "metadata": [], "built_in_metadata": []},
			"metadata_config": {"enabled": true, "metadata": [{"key": "flat"}], "built_in_metadata": []}
		}
	}`)

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	want := decodeIngestionConfig(t, `{
		"Extractor:AutoExtractDefault": {
			"keywords": {"top_n": 2, "system_prompt": "node kw"},
			"summary": {"enabled": false, "system_prompt": "node summary"},
			"tags": {"top_n": 1, "tag_file_id": "node-tag"},
			"metadata": {"enabled": false, "metadata": [], "built_in_metadata": []}
		}
	}`)
	if got := readLegacyParserConfig(t, db, "knowledgebase", "kb"); !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %#v; want %#v", got, want)
	}
}

func TestMigrateIngestionLegacyConfigsMetadataPrecedence(t *testing.T) {
	db := openIngestionMigrationDB(t)
	insertIngestionConfig(t, db, "knowledgebase", "group", "general", `{
		"metadata": {"enabled": true, "metadata": [{"key": "top"}], "built_in_metadata": []},
		"Extractor:AutoExtractDefault": {
			"metadata_config": {"enabled": false, "metadata": [{"key": "node"}], "built_in_metadata": []}
		}
	}`)
	insertIngestionConfig(t, db, "knowledgebase", "flat-list", "general", `{
		"metadata": [{"key": "flat-list"}],
		"Extractor:AutoExtractDefault": {
			"metadata_config": {"enabled": true, "metadata": [{"key": "node"}], "built_in_metadata": []}
		}
	}`)

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		id   string
		want string
	}{
		{"group", `{"Extractor:AutoExtractDefault": {"metadata": {"enabled": true, "metadata": [{"key": "top"}], "built_in_metadata": []}}}`},
		{"flat-list", `{"Extractor:AutoExtractDefault": {"metadata": {"enabled": true, "metadata": [{"key": "node"}], "built_in_metadata": []}}}`},
	} {
		if got := readLegacyParserConfig(t, db, "knowledgebase", tc.id); !reflect.DeepEqual(got, decodeIngestionConfig(t, tc.want)) {
			t.Fatalf("%s config = %#v; want %s", tc.id, got, tc.want)
		}
	}
}

func TestMigrateIngestionLegacyConfigsNodeWinsOverTopLevel(t *testing.T) {
	db := openIngestionMigrationDB(t)
	insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{
		"auto_keywords": 99,
		"auto_tags": 99,
		"tag_file_id": "top-tag",
		"enable_summary": 1,
		"sys_prompt": "top summary",
		"metadata_config": {"enabled": true, "metadata": [{"key": "top"}], "built_in_metadata": []},
		"Extractor:AutoExtractDefault": {
			"auto_keywords": 3,
			"keywords_sys_prompt": "node kw",
			"auto_tags": 2,
			"tag_file_id": "node-tag",
			"enable_summary": 0,
			"sys_prompt": "node summary",
			"metadata_config": {"enabled": false, "metadata": [{"key": "node"}], "built_in_metadata": []}
		}
	}`)

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	want := decodeIngestionConfig(t, `{
		"Extractor:AutoExtractDefault": {
			"keywords": {"top_n": 3, "system_prompt": "node kw"},
			"tags": {"top_n": 2, "tag_file_id": "node-tag"},
			"summary": {"enabled": false, "system_prompt": "node summary"},
			"metadata": {"enabled": false, "metadata": [{"key": "node"}], "built_in_metadata": []}
		}
	}`)
	if got := readLegacyParserConfig(t, db, "knowledgebase", "kb"); !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %#v; want %#v", got, want)
	}
}

func TestMigrateIngestionLegacyConfigsMarkerGap(t *testing.T) {
	db := openIngestionMigrationDB(t)
	writeDatabaseVersion(t, db, "v1.0.0-rc2.dev1")
	insertIngestionConfig(t, db, "knowledgebase", "flat", "general", `{
		"auto_keywords": 7,
		"keywords_sys_prompt": "flat kw",
		"enable_summary": 1,
		"sys_prompt": "flat summary",
		"auto_tags": 2,
		"tag_file_id": "flat-tag",
		"metadata_config": {"enabled": true, "metadata": [{"key": "cfg"}], "built_in_metadata": []}
	}`)
	insertIngestionConfig(t, db, "knowledgebase", "fields", "general", `{
		"enable_metadata": true,
		"fields": [{"key": "legacy"}]
	}`)

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	wantFlat := decodeIngestionConfig(t, `{
		"Extractor:AutoExtractDefault": {
			"keywords": {"top_n": 7, "system_prompt": "flat kw"},
			"summary": {"enabled": true, "system_prompt": "flat summary"},
			"tags": {"top_n": 2, "tag_file_id": "flat-tag"},
			"metadata": {"enabled": true, "metadata": [{"key": "cfg"}], "built_in_metadata": []}
		}
	}`)
	if got := readLegacyParserConfig(t, db, "knowledgebase", "flat"); !reflect.DeepEqual(got, wantFlat) {
		t.Fatalf("flat config = %#v; want %#v", got, wantFlat)
	}
	wantFields := decodeIngestionConfig(t, `{
		"Extractor:AutoExtractDefault": {
			"metadata": {"enabled": true, "metadata": [{"key": "legacy"}], "built_in_metadata": []}
		}
	}`)
	if got := readLegacyParserConfig(t, db, "knowledgebase", "fields"); !reflect.DeepEqual(got, wantFields) {
		t.Fatalf("fields config = %#v; want %#v", got, wantFields)
	}
	if got := databaseVersion(t, db); got != ingestionLegacyConfigTargetVersion {
		t.Fatalf("version = %q; want %q", got, ingestionLegacyConfigTargetVersion)
	}
}

func TestMigrateIngestionLegacyConfigsCanvasDSL(t *testing.T) {
	db := openIngestionMigrationDB(t)
	const raw = `{
		"components": {
			"Extractor:AutoExtractDefault": {
				"obj": {
					"component_name": "Extractor",
					"params": {
						"auto_keywords": 3,
						"keywords_sys_prompt": "kw",
						"metadata_config": {"enabled": true, "metadata": [{"key": "author"}], "built_in_metadata": []}
					}
				}
			},
			"TitleChunker:GrumpyGarlicsBake": {
				"obj": {
					"component_name": "TitleChunker",
					"params": {"rules": [{"levels": [{"expression": "^#"}]}]}
				}
			},
			"TokenChunker:WarmBreadSmells": {
				"obj": {
					"component_name": "TokenChunker",
					"params": {"delimiter_mode": "token_size"}
				}
			},
			"Parser:HipSignsRhyme": {
				"obj": {
					"component_name": "Parser",
					"params": {"pdf": {"parse_method": "DeepDOC"}}
				}
			}
		},
		"graph": {
			"nodes": [
				{
					"id": "Extractor:AutoExtractDefault",
					"data": {
						"label": "My Extractor",
						"form": {
							"auto_keywords": 3,
							"keywords_sys_prompt": "kw",
							"metadata_config": {"enabled": true, "metadata": [{"key": "author"}], "built_in_metadata": []}
						}
					}
				},
				{
					"id": "TitleChunker:GrumpyGarlicsBake",
					"data": {
						"label": "TitleChunker",
						"form": {"rules": [{"levels": [{"expression": "^#"}]}]}
					}
				},
				{
					"id": "TokenChunker:WarmBreadSmells",
					"data": {
						"label": "TokenChunker",
						"form": {"delimiter_mode": "token_size"}
					}
				}
			]
		}
	}`
	insertCanvasDSL(t, db, "user_canvas", "canvas", raw)
	insertCanvasDSL(t, db, "user_canvas_version", "version", raw)

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	for _, target := range []struct {
		table string
		id    string
	}{
		{"user_canvas", "canvas"},
		{"user_canvas_version", "version"},
	} {
		dsl := readCanvasDSL(t, db, target.table, target.id)
		components := dsl["components"].(map[string]any)
		extractor := components["Extractor:AutoExtractDefault"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
		if extractor["keywords"] == nil || extractor["keywords"].(map[string]any)["top_n"] != float64(3) {
			t.Fatalf("%s extractor params = %#v", target.table, extractor)
		}
		if _, remains := extractor["metadata_config"]; remains {
			t.Fatalf("%s metadata_config survived: %#v", target.table, extractor)
		}
		title := components["TitleChunker:GrumpyGarlicsBake"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
		if _, remains := title["rules"]; remains {
			t.Fatalf("%s title rules survived: %#v", target.table, title)
		}
		token := components["TokenChunker:WarmBreadSmells"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
		if token["delimiter_mode"] != "delimiter" {
			t.Fatalf("%s token delimiter_mode = %#v", target.table, token["delimiter_mode"])
		}
		parser := components["Parser:HipSignsRhyme"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
		if parser["pdf"].(map[string]any)["parse_method"] != "DeepDOC" {
			t.Fatalf("%s parser params changed: %#v", target.table, parser)
		}
		nodes := dsl["graph"].(map[string]any)["nodes"].([]any)
		for _, rawNode := range nodes {
			node := rawNode.(map[string]any)
			form := node["data"].(map[string]any)["form"].(map[string]any)
			if node["id"] == "Extractor:AutoExtractDefault" {
				if _, remains := form["auto_keywords"]; remains {
					t.Fatalf("%s extractor form still has auto_keywords: %#v", target.table, form)
				}
				if form["keywords"] == nil {
					t.Fatalf("%s extractor form missing keywords: %#v", target.table, form)
				}
			}
			if node["id"] == "TitleChunker:GrumpyGarlicsBake" {
				if _, remains := form["rules"]; remains {
					t.Fatalf("%s title form still has rules: %#v", target.table, form)
				}
				if form["levels"] == nil {
					t.Fatalf("%s title form missing levels: %#v", target.table, form)
				}
			}
			if node["id"] == "TokenChunker:WarmBreadSmells" && form["delimiter_mode"] != "delimiter" {
				t.Fatalf("%s token form delimiter_mode = %#v", target.table, form["delimiter_mode"])
			}
		}
		if changed, err := normalizeCanvasDSL(dsl, ingestionConfigRow{table: target.table, rowID: target.id}); err != nil || changed {
			t.Fatalf("%s second normalization changed=%v err=%v", target.table, changed, err)
		}
	}
	if got := databaseVersion(t, db); got != ingestionLegacyConfigTargetVersion {
		t.Fatalf("version = %q; want %q", got, ingestionLegacyConfigTargetVersion)
	}
}

func TestMigrateIngestionLegacyConfigsRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, db *gorm.DB)
		want  string
	}{
		{
			name: "extractor top_n",
			setup: func(t *testing.T, db *gorm.DB) {
				insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{"Extractor:AutoExtractDefault":{"auto_keywords":"many"}}`)
			},
			want: "auto_keywords",
		},
		{
			name: "extractor metadata_config",
			setup: func(t *testing.T, db *gorm.DB) {
				insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{"Extractor:AutoExtractDefault":{"metadata_config":"bad"}}`)
			},
			want: "metadata_config",
		},
		{
			name: "extractor fields",
			setup: func(t *testing.T, db *gorm.DB) {
				insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{"Extractor:AutoExtractDefault":{"fields":"bad"}}`)
			},
			want: "metadata",
		},
		{
			name: "extractor field_name",
			setup: func(t *testing.T, db *gorm.DB) {
				insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{"Extractor:AutoExtractDefault":{"field_name":5}}`)
			},
			want: "field_name",
		},
		{
			name: "extractor sys_prompt",
			setup: func(t *testing.T, db *gorm.DB) {
				insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{"Extractor:AutoExtractDefault":{"sys_prompt":5}}`)
			},
			want: "sys_prompt",
		},
		{
			name: "title rules",
			setup: func(t *testing.T, db *gorm.DB) {
				insertIngestionConfig(t, db, "knowledgebase", "kb", "general", `{"TitleChunker:X":{"rules":"bad"}}`)
			},
			want: "rules",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openIngestionMigrationDB(t)
			writeDatabaseVersion(t, db, "v1.0.0.1")
			tc.setup(t, db)
			err := migrateIngestionLegacyConfigs(t.Context(), db)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want containing %q", err, tc.want)
			}
			if got := databaseVersion(t, db); got != "v1.0.0.1" {
				t.Fatalf("failed migration advanced version to %q", got)
			}
		})
	}
}

func TestMigrateIngestionLegacyConfigsSkipsMalformedCanvas(t *testing.T) {
	db := openIngestionMigrationDB(t)
	writeDatabaseVersion(t, db, "v1.0.0.1")
	insertCanvasDSL(t, db, "user_canvas", "good", `{
		"components": {"Extractor:X": {"obj": {"component_name": "Extractor", "params": {"auto_keywords": 3}}}}
	}`)
	insertCanvasDSL(t, db, "user_canvas", "bad-json", "not-json")
	insertCanvasDSL(t, db, "user_canvas", "bad-param", `{
		"components": {"Extractor:X": {"obj": {"component_name": "Extractor", "params": {"auto_keywords": "many"}}}}
	}`)
	insertCanvasDSL(t, db, "user_canvas", "bad-form", `{
		"graph": {"nodes": [{"id": "TitleChunker:X", "data": {"label": "TitleChunker", "form": {"rules": "bad"}}}]}
	}`)

	if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
		t.Fatalf("migration must skip bad canvas rows, got %v", err)
	}
	if got := databaseVersion(t, db); got != ingestionLegacyConfigTargetVersion {
		t.Fatalf("version = %q; want %q", got, ingestionLegacyConfigTargetVersion)
	}

	good := readCanvasDSL(t, db, "user_canvas", "good")
	params := good["components"].(map[string]any)["Extractor:X"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if params["keywords"] == nil {
		t.Fatalf("good canvas was not migrated: %#v", params)
	}

	var rawDSL struct {
		DSL *string `gorm:"column:dsl"`
	}
	if err := db.Table("user_canvas").Select("dsl").Where("id = ?", "bad-json").Take(&rawDSL).Error; err != nil {
		t.Fatal(err)
	}
	if rawDSL.DSL == nil || *rawDSL.DSL != "not-json" {
		t.Fatalf("bad-json row = %v; want unchanged", rawDSL.DSL)
	}

	badParam := readCanvasDSL(t, db, "user_canvas", "bad-param")
	badParams := badParam["components"].(map[string]any)["Extractor:X"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if badParams["auto_keywords"] != "many" {
		t.Fatalf("bad-param row was modified: %#v", badParams)
	}

	badForm := readCanvasDSL(t, db, "user_canvas", "bad-form")
	form := badForm["graph"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["data"].(map[string]any)["form"].(map[string]any)
	if form["rules"] != "bad" {
		t.Fatalf("bad-form row was modified: %#v", form)
	}
}

func TestMigrateIngestionLegacyConfigsVersionGateAndEmptyRows(t *testing.T) {
	for _, tc := range []struct {
		version string
		skip    bool
	}{
		{"v1.0.0.2", true},
		{"v1.0.0.1", false},
		{"v1.0.0-rc2.dev1", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			db := openIngestionMigrationDB(t)
			writeDatabaseVersion(t, db, tc.version)
			for index, raw := range []string{"", "null", "{}"} {
				insertIngestionConfig(t, db, "knowledgebase", fmt.Sprint(index), "general", raw)
			}
			insertIngestionConfig(t, db, "knowledgebase", "legacy", "general", `{"Extractor:AutoExtractDefault":{"auto_keywords":1}}`)
			if err := migrateIngestionLegacyConfigs(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			config := readLegacyParserConfig(t, db, "knowledgebase", "legacy")
			_, migrated := config["Extractor:AutoExtractDefault"].(map[string]any)["keywords"]
			if migrated == tc.skip {
				t.Fatalf("migrated=%v; want %v", migrated, !tc.skip)
			}
			if tc.skip {
				if got := databaseVersion(t, db); got != tc.version {
					t.Fatalf("version = %q; want %q", got, tc.version)
				}
			} else if got := databaseVersion(t, db); got != ingestionLegacyConfigTargetVersion {
				t.Fatalf("version = %q; want %q", got, ingestionLegacyConfigTargetVersion)
			}
		})
	}
}
