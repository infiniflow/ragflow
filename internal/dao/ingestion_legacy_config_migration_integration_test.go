//go:build integration
// +build integration

package dao

import (
	"context"
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

// TestMigrateIngestionLegacyConfigs_Integration runs the legacy ingestion
// migration against real MySQL. The SQLite unit tests cannot catch dialect
// differences in the JSON columns or the per-batch transactions, so this seeds
// one row per table and verifies the same shape the unit tests expect.
//
// Run with:
//
//	RAGFLOW_TEST_MYSQL_DSN='root:infini_rag_flow@tcp(127.0.0.1:3306)/ragflow_test?charset=utf8mb4&parseTime=True&loc=Local' \
//	  bash build.sh --test-integration ./internal/dao/...
func TestMigrateIngestionLegacyConfigs_Integration(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	seedDSLVersionTables(t, db)

	if err := setDatabaseMigrationVersion(ctx, db, "v1.0.0.1"); err != nil {
		t.Fatalf("seed version: %v", err)
	}

	parserConfig := func(fieldName string) entity.JSONMap {
		return entity.JSONMap{
			"Extractor:AutoExtractDefault": map[string]any{
				"field_name": fieldName,
				"sys_prompt": fieldName + " prompt",
				"auto_tags":  1,
			},
			"TitleChunker:GrumpyGarlicsBake": map[string]any{
				"method": "group",
				"rules":  []any{map[string]any{"levels": []any{map[string]any{"expression": "^#"}}}},
			},
			"TokenChunker:WarmBreadSmells": map[string]any{
				"delimiter_mode": "token_size",
			},
		}
	}
	if err := db.WithContext(ctx).Create(&entity.Knowledgebase{
		ID: "kb-legacy", ParserID: "general", ParserConfig: parserConfig("summary"),
	}).Error; err != nil {
		t.Fatalf("seed knowledgebase: %v", err)
	}
	if err := db.WithContext(ctx).Create(&entity.Document{
		ID: "doc-legacy", KbID: "kb-legacy", ParserID: "general", ParserConfig: parserConfig("keywords"),
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}
	if err := db.WithContext(ctx).Create(&entity.UserCanvas{
		ID: "canvas-legacy", CanvasCategory: "dataflow_canvas", Title: strPtr("legacy"),
		DSL: entity.JSONMap{
			"components": map[string]any{
				"Extractor:AutoExtractDefault": map[string]any{
					"obj": map[string]any{
						"component_name": "Extractor",
						"params":         map[string]any{"field_name": "questions", "sys_prompt": "questions prompt"},
					},
				},
				"TokenChunker:WarmBreadSmells": map[string]any{
					"obj": map[string]any{
						"component_name": "TokenChunker",
						"params":         map[string]any{"delimiter_mode": "token_size"},
					},
				},
			},
			"graph": map[string]any{
				"nodes": []any{
					map[string]any{
						"id": "Extractor:AutoExtractDefault",
						"data": map[string]any{
							"label": "Extractor",
							"form":  map[string]any{"field_name": "questions", "sys_prompt": "questions prompt"},
						},
					},
					map[string]any{
						"id": "TokenChunker:WarmBreadSmells",
						"data": map[string]any{
							"label": "TokenChunker",
							"form":  map[string]any{"delimiter_mode": "token_size"},
						},
					},
				},
			},
		},
	}).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
	if err := db.WithContext(ctx).Create(&entity.UserCanvasVersion{
		ID: "version-legacy", UserCanvasID: "canvas-legacy",
		DSL: entity.JSONMap{
			"components": map[string]any{
				"Extractor:AutoExtractDefault": map[string]any{
					"obj": map[string]any{
						"component_name": "Extractor",
						"params":         map[string]any{"field_name": "metadata", "sys_prompt": "metadata prompt"},
					},
				},
			},
		},
	}).Error; err != nil {
		t.Fatalf("seed canvas version: %v", err)
	}
	if err := db.WithContext(ctx).Create(&entity.UserCanvas{
		ID: "canvas-bad", CanvasCategory: "dataflow_canvas", Title: strPtr("bad"),
		DSL: entity.JSONMap{
			"components": map[string]any{
				"Extractor:Bad": map[string]any{
					"obj": map[string]any{
						"component_name": "Extractor",
						"params":         map[string]any{"auto_keywords": "many"},
					},
				},
			},
		},
	}).Error; err != nil {
		t.Fatalf("seed bad canvas: %v", err)
	}

	if err := migrateIngestionLegacyConfigs(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got, err := GetDatabaseMigrationVersion(ctx, db); err != nil || got != ingestionLegacyConfigTargetVersion {
		t.Fatalf("version = %q, err = %v; want %q", got, err, ingestionLegacyConfigTargetVersion)
	}

	for _, tc := range []struct {
		table string
		id    string
		want  string
	}{
		{
			table: "knowledgebase", id: "kb-legacy",
			want: `{
				"Extractor:AutoExtractDefault": {"summary": {"enabled": true, "system_prompt": "summary prompt"}, "tags": {"top_n": 1}},
				"TitleChunker:GrumpyGarlicsBake": {"method": "group", "levels": [["^#"]]},
				"TokenChunker:WarmBreadSmells": {"delimiter_mode": "delimiter"}
			}`,
		},
		{
			table: "document", id: "doc-legacy",
			want: `{
				"Extractor:AutoExtractDefault": {"keywords": {"system_prompt": "keywords prompt"}, "tags": {"top_n": 1}},
				"TitleChunker:GrumpyGarlicsBake": {"method": "group", "levels": [["^#"]]},
				"TokenChunker:WarmBreadSmells": {"delimiter_mode": "delimiter"}
			}`,
		},
	} {
		if got := readLegacyParserConfig(t, db, tc.table, tc.id); !reflect.DeepEqual(got, decodeIngestionConfig(t, tc.want)) {
			t.Fatalf("%s config = %#v; want %s", tc.table, got, tc.want)
		}
	}

	canvas := readCanvasDSL(t, db, "user_canvas", "canvas-legacy")
	params := canvas["components"].(map[string]any)["Extractor:AutoExtractDefault"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if !reflect.DeepEqual(params, decodeIngestionConfig(t, `{"questions": {"system_prompt": "questions prompt"}}`)) {
		t.Fatalf("canvas extractor params = %#v", params)
	}
	token := canvas["components"].(map[string]any)["TokenChunker:WarmBreadSmells"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if token["delimiter_mode"] != "delimiter" {
		t.Fatalf("canvas token params = %#v", token)
	}
	form := canvas["graph"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["data"].(map[string]any)["form"].(map[string]any)
	if !reflect.DeepEqual(form, decodeIngestionConfig(t, `{"questions": {"system_prompt": "questions prompt"}}`)) {
		t.Fatalf("canvas graph form = %#v", form)
	}

	version := readCanvasDSL(t, db, "user_canvas_version", "version-legacy")
	metadata := version["components"].(map[string]any)["Extractor:AutoExtractDefault"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if _, remains := metadata["sys_prompt"]; remains {
		t.Fatalf("metadata node kept sys_prompt: %#v", metadata)
	}
	if _, remains := metadata["summary"]; remains {
		t.Fatalf("metadata node gained a summary group: %#v", metadata)
	}

	badCanvas := readCanvasDSL(t, db, "user_canvas", "canvas-bad")
	badParams := badCanvas["components"].(map[string]any)["Extractor:Bad"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if badParams["auto_keywords"] != "many" {
		t.Fatalf("bad canvas was modified: %#v", badParams)
	}

	// The marker makes the second run a no-op; clearing it proves the
	// converters themselves are idempotent on MySQL.
	before := encodeIngestionConfig(t, readLegacyParserConfig(t, db, "knowledgebase", "kb-legacy"))
	if err := setDatabaseMigrationVersion(ctx, db, "v1.0.0.1"); err != nil {
		t.Fatalf("reset version: %v", err)
	}
	if err := migrateIngestionLegacyConfigs(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	after := encodeIngestionConfig(t, readLegacyParserConfig(t, db, "knowledgebase", "kb-legacy"))
	if after != before {
		t.Fatalf("second run changed config:\nbefore %s\nafter  %s", before, after)
	}
}
