package dao

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	sqliteDriver "github.com/glebarez/go-sqlite"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"ragflow/internal/entity"
	"sync"
	"testing"
)

var seedDatabaseFunction sync.Once
var seedDatabaseFunctionError error

func canvasSeedResourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	seedDatabaseFunction.Do(func() {
		seedDatabaseFunctionError = sqliteDriver.RegisterDeterministicScalarFunction("database", 0, func(*sqliteDriver.FunctionContext, []driver.Value) (driver.Value, error) { return "main", nil })
	})
	if seedDatabaseFunctionError != nil {
		t.Fatal(seedDatabaseFunctionError)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.CanvasTemplate{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"ATTACH DATABASE ':memory:' AS INFORMATION_SCHEMA", "CREATE TABLE INFORMATION_SCHEMA.COLUMNS (TABLE_SCHEMA text,TABLE_NAME text,COLUMN_NAME text)", "INSERT INTO INFORMATION_SCHEMA.COLUMNS VALUES ('main','canvas_template','parser_ids')"} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"previous_agent", "previous_pipeline"} {
		if err := db.Create(&entity.CanvasTemplate{ID: id, Title: entity.JSONMap{"en": id}, DSL: entity.JSONMap{}}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func writeCanvasResource(t *testing.T, root, dir, name, body string) {
	t.Helper()
	target := filepath.Join(root, dir)
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestSeedCanvasTemplatesPreservesRowsWithIncompleteResources exercises the public startup path.
func TestSeedCanvasTemplatesPreservesRowsWithIncompleteResources(t *testing.T) {
	for _, test := range []struct {
		name            string
		agent, pipeline bool
		bad             string
	}{
		{"missing_agent", false, true, ""}, {"missing_pipeline", true, false, ""}, {"missing_both", false, false, ""},
		{"empty_pipeline", true, false, ""}, {"empty_agent", false, true, ""},
		{"malformed_json", true, true, `invalid-json`}, {"missing_identity", true, true, `{"title":{"en":"broken"},"dsl":{}}`},
		{"unreadable_file", true, true, ""},
		{"trailing_garbage", true, true, `{"id":"bad","dsl":{}}garbage`},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := canvasSeedResourceDB(t)
			root := t.TempDir()
			if test.agent {
				writeCanvasResource(t, root, "agent/templates", "valid.json", `{"id":"agent","title":{"en":"Agent"},"dsl":{}}`)
			}
			if test.pipeline {
				writeCanvasResource(t, root, "internal/ingestion/pipeline/template", "valid.json", `{"id":"pipeline","canvas_category":"ingestion_pipeline","dsl":{}}`)
			}
			if test.name == "empty_pipeline" || test.name == "empty_agent" {
				dir := "agent/templates"
				if test.name == "empty_pipeline" {
					dir = "internal/ingestion/pipeline/template"
				}
				if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if test.bad != "" {
				writeCanvasResource(t, root, "agent/templates", "bad.json", test.bad)
			}
			if test.name == "unreadable_file" {
				if err := os.Symlink(filepath.Join(root, "missing-resource"), filepath.Join(root, "agent/templates", "bad.json")); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(root)
			if err := SeedCanvasTemplates(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"previous_agent", "previous_pipeline"} {
				var count int64
				if err := db.Model(&entity.CanvasTemplate{}).Where("id = ?", id).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Errorf("existing %s deleted with incomplete resources", id)
				}
			}
			if test.agent {
				var count int64
				if err := db.Model(&entity.CanvasTemplate{}).Where("id = ?", "agent").Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Error("healthy resource not seeded")
				}
			}
		})
	}
}

// TestSeedCanvasTemplatesCompleteCatalogPrunesAndSkipsStandaloneDSL preserves valid cleanup semantics.
func TestSeedCanvasTemplatesCompleteCatalogPrunesAndSkipsStandaloneDSL(t *testing.T) {
	db := canvasSeedResourceDB(t)
	root := t.TempDir()
	writeCanvasResource(t, root, "agent/templates", "valid.json", `{"id":"agent","title":{"en":"Agent"},"dsl":{}}`)
	writeCanvasResource(t, root, "agent/templates", "compiler.json", `{"components":{},"graph":{}}`)
	writeCanvasResource(t, root, "internal/ingestion/pipeline/template", "valid.json", `{"id":"pipeline","canvas_category":"ingestion_pipeline","dsl":{}}`)
	t.Chdir(root)
	for i := 0; i < 2; i++ {
		if err := SeedCanvasTemplates(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var rows []entity.CanvasTemplate
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.ID != "agent" && row.ID != "pipeline" {
			t.Errorf("unexpected template identity %q", row.ID)
		}
	}
}

// TestParseCanvasTemplateIdentity accepts existing numeric/string catalogs and rejects invalid identities.
func TestParseCanvasTemplateIdentity(t *testing.T) {
	for _, test := range []struct {
		name, raw, want string
		invalid         bool
	}{
		{"string", `{"id":"42","dsl":{}}`, "42", false}, {"number", `{"id":42,"dsl":{}}`, "42", false},
		{"large_number", `{"id":9007199254740993,"dsl":{}}`, "9007199254740993", false},
		{"absent", `{"dsl":{}}`, "", true}, {"empty", `{"id":"","dsl":{}}`, "", true}, {"blank", `{"id":"  ","dsl":{}}`, "", true},
		{"null", `{"id":null,"dsl":{}}`, "", true}, {"object", `{"id":{},"dsl":{}}`, "", true},
		{"trailing", `{"id":"42","dsl":{}}{}`, "", true},
		{"null_envelope", `{"components":{},"dsl":null}`, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			row, err := parseCanvasTemplateFile([]byte(test.raw))
			if test.invalid {
				if err == nil {
					t.Fatalf("invalid identity accepted: %#v", row)
				}
				return
			}
			if err != nil || row == nil || row.ID != test.want {
				t.Fatalf("row=%#v err=%v", row, err)
			}
		})
	}
}

// TestBuiltInCanvasResourcesSeedFromRuntimeLayout loads the actual shipped resources through runtime paths.
func TestBuiltInCanvasResourcesSeedFromRuntimeLayout(t *testing.T) {
	db := canvasSeedResourceDB(t)
	root := t.TempDir()
	expected := map[string]bool{}
	for _, source := range []struct{ src, dst string }{{"../agent/templates", "agent/templates"}, {"../ingestion/pipeline/template", "internal/ingestion/pipeline/template"}} {
		files, err := filepath.Glob(filepath.Join(source.src, "*.json"))
		if err != nil || len(files) == 0 {
			t.Fatalf("missing resources: %v", err)
		}
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var catalog map[string]any
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			if err := decoder.Decode(&catalog); err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			if id, present := catalog["id"]; present {
				expected[fmt.Sprint(id)] = true
			}
			writeCanvasResource(t, root, source.dst, filepath.Base(file), string(raw))
		}
	}
	t.Chdir(root)
	if err := SeedCanvasTemplates(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&entity.CanvasTemplate{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(len(expected)) || len(expected) == 0 {
		t.Fatalf("seeded %d templates, want %d", count, len(expected))
	}
	var rows []entity.CanvasTemplate
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if !expected[row.ID] {
			t.Errorf("unexpected seeded identity %q", row.ID)
		}
	}
	var empty int64
	if err := db.Model(&entity.CanvasTemplate{}).Where("id = ?", "").Count(&empty).Error; err != nil {
		t.Fatal(err)
	}
	if empty != 0 {
		t.Fatal("standalone DSL inserted with an empty ID")
	}
}
