package dao

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"testing/fstest"

	"ragflow/internal/entity"
	builtintemplates "ragflow/internal/ingestion/pipeline/template"

	sqliteDriver "github.com/glebarez/go-sqlite"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var seedDatabaseFunction sync.Once
var seedDatabaseFunctionError error

// canvasSeedResourceDB isolates seeding in SQLite and supplies the database
// metadata expected by the startup migration check.
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

// writeCanvasResource creates fixtures in the relative directory layout used
// by startup resource discovery.
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

// embeddedCanvasCatalog reads independent expectations from the original JSON envelopes.
func embeddedCanvasCatalog(t *testing.T) map[string]map[string]any {
	t.Helper()
	entries, err := fs.ReadDir(builtintemplates.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	catalog := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		raw, err := fs.ReadFile(builtintemplates.FS(), entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&data); err != nil {
			t.Fatal(err)
		}
		catalog[fmt.Sprint(data["id"])] = data
	}
	if len(catalog) == 0 {
		t.Fatal("no embedded ingestion catalog")
	}
	return catalog
}

// assertEmbeddedCanvasCatalog checks persisted fields without using the catalog parser for expectations.
func assertEmbeddedCanvasCatalog(t *testing.T, db *gorm.DB) {
	t.Helper()
	var rows []entity.CanvasTemplate
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]entity.CanvasTemplate, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for id, want := range embeddedCanvasCatalog(t) {
		row, ok := byID[id]
		if !ok {
			t.Fatalf("embedded template %s not seeded", id)
		}
		for field, got := range map[string]any{"title": row.Title, "description": row.Description, "dsl": row.DSL} {
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			wantJSON, err := json.Marshal(want[field])
			if err != nil {
				t.Fatal(err)
			}
			var gotValue, wantValue any
			if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(wantJSON, &wantValue); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotValue, wantValue) {
				t.Errorf("template %s lost %s fields", id, field)
			}
		}
		if row.CanvasCategory != want["canvas_category"] {
			t.Errorf("template %s category=%q, want %v", id, row.CanvasCategory, want["canvas_category"])
		}
		if kind, ok := want["canvas_type"].(string); ok {
			if row.CanvasType == nil || *row.CanvasType != kind {
				t.Errorf("template %s lost canvas_type", id)
			}
			found := false
			for _, value := range row.CanvasTypes {
				if value == kind {
					found = true
				}
			}
			if !found {
				t.Errorf("template %s lost canvas_types", id)
			}
		}
	}
}

// TestSeedCanvasTemplatesPreservesRowsWithIncompleteResources keeps old rows while updating healthy sources.
func TestSeedCanvasTemplatesPreservesRowsWithIncompleteResources(t *testing.T) {
	for _, test := range []struct {
		name         string
		agent, empty bool
		bad          string
	}{
		{"missing_agent", false, false, ""}, {"empty_agent", false, true, ""},
		{"malformed_json", true, false, `invalid-json`},
		{"missing_identity", true, false, `{"title":{"en":"broken"},"dsl":{}}`},
		{"unreadable_file", true, false, ""},
		{"trailing_garbage", true, false, `{"id":"bad","dsl":{}}garbage`},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := canvasSeedResourceDB(t)
			root := t.TempDir()
			if test.agent {
				writeCanvasResource(t, root, "agent/templates", "valid.json", `{"id":"agent","title":{"en":"Agent"},"dsl":{}}`)
			}
			if test.empty {
				if err := os.MkdirAll(filepath.Join(root, "agent/templates"), 0700); err != nil {
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
					t.Error("healthy agent resource not seeded")
				}
			}
			assertEmbeddedCanvasCatalog(t, db)
		})
	}
}

// TestSeedCanvasTemplatesWithoutIngestionDirectory checks seeding, pruning and retries with no filesystem ingestion source.
func TestSeedCanvasTemplatesWithoutIngestionDirectory(t *testing.T) {
	db := canvasSeedResourceDB(t)
	root := t.TempDir()
	writeCanvasResource(t, root, "agent/templates", "valid.json", `{"id":"agent","title":{"en":"Agent"},"dsl":{}}`)
	writeCanvasResource(t, root, "agent/templates", "compiler.json", `{"components":{},"graph":{}}`)
	t.Chdir(root)
	if _, err := os.Stat("internal/ingestion/pipeline/template"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ingestion directory unexpectedly exists: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := SeedCanvasTemplates(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var rows []entity.CanvasTemplate
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	expected := embeddedCanvasCatalog(t)
	if len(rows) != len(expected)+1 {
		t.Fatalf("rows=%d, want %d", len(rows), len(expected)+1)
	}
	for _, row := range rows {
		if row.ID != "agent" && expected[row.ID] == nil {
			t.Errorf("unexpected template identity %q", row.ID)
		}
	}
	assertEmbeddedCanvasCatalog(t, db)
}

// failingCanvasTemplateFS injects a read failure while retaining a healthy file.
type failingCanvasTemplateFS struct {
	fs.FS
	blocked string
}

func (f failingCanvasTemplateFS) Open(name string) (fs.File, error) {
	if name == f.blocked {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.FS.Open(name)
}

// TestLoadCanvasTemplatesFromFS checks complete metadata, partial loads and underlying read errors.
func TestLoadCanvasTemplatesFromFS(t *testing.T) {
	raw := `{"id":99,"title":{"en":"General","zh":"通用","de":"Allgemein"},"description":{"en":"Description","zh":"说明"},"avatar":"icon","canvas_category":"dataflow_canvas","canvas_type":"Ingestion Pipeline","canvas_types":["Ingestion Pipeline","Other"],"dsl":{"components":{},"params":{"limit":100}}}`
	source := fstest.MapFS{"valid.json": {Data: []byte(raw)}, "component.json": {Data: []byte(`{"components":{},"graph":{}}`)}}
	templates, ids, err := loadTemplatesFromFS(source)
	if err != nil || len(templates) != 1 || !reflect.DeepEqual(ids, []string{"99"}) {
		t.Fatalf("templates=%v ids=%v error=%v", templates, ids, err)
	}
	row := templates[0]
	if row.Title["zh"] != "通用" || row.Title["en"] != "General" || row.Title["de"] != "Allgemein" || row.Description["zh"] != "说明" {
		t.Fatal("localized metadata lost")
	}
	if row.Avatar == nil || *row.Avatar != "icon" || row.CanvasType == nil || *row.CanvasType != "Ingestion Pipeline" || row.CanvasCategory != "dataflow_canvas" || !reflect.DeepEqual([]any(row.CanvasTypes), []any{"Ingestion Pipeline", "Other"}) {
		t.Fatalf("canvas metadata lost: %#v", row)
	}
	dslJSON, err := json.Marshal(row.DSL)
	if err != nil {
		t.Fatal(err)
	}
	if string(dslJSON) != `{"components":{},"params":{"limit":100}}` {
		t.Fatalf("DSL changed: %s", dslJSON)
	}
	for _, test := range []struct {
		name       string
		files      fs.FS
		want       int
		permission bool
	}{
		{"directory_read", failingCanvasTemplateFS{source, "."}, 0, true},
		{"file_read", failingCanvasTemplateFS{fstest.MapFS{"good.json": {Data: []byte(raw)}, "bad.json": {Data: []byte(raw)}}, "bad.json"}, 1, true},
		{"malformed_json", fstest.MapFS{"good.json": {Data: []byte(raw)}, "bad.json": {Data: []byte(`broken-json`)}}, 1, false},
		{"missing_identity", fstest.MapFS{"good.json": {Data: []byte(raw)}, "bad.json": {Data: []byte(`{"dsl":{}}`)}}, 1, false},
		{"trailing_json", fstest.MapFS{"good.json": {Data: []byte(raw)}, "bad.json": {Data: []byte(`{"id":"bad","dsl":{}}{}`)}}, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			templates, _, err := loadTemplatesFromFS(test.files)
			if err == nil || len(templates) != test.want {
				t.Fatalf("loaded=%d error=%v", len(templates), err)
			}
			if test.permission && !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("read error identity lost: %v", err)
			}
		})
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

// TestBuiltInCanvasResourcesSeedWithoutIngestionDirectory checks the actual shipped templates with only agent files deployed.
func TestBuiltInCanvasResourcesSeedWithoutIngestionDirectory(t *testing.T) {
	db := canvasSeedResourceDB(t)
	root := t.TempDir()
	expected := embeddedCanvasCatalog(t)
	files, err := filepath.Glob("../agent/templates/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("missing agent resources: %v", err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&data); err != nil {
			t.Fatal(err)
		}
		if id, ok := data["id"]; ok {
			expected[fmt.Sprint(id)] = data
		}
		writeCanvasResource(t, root, "agent/templates", filepath.Base(file), string(raw))
	}
	t.Chdir(root)
	if err := SeedCanvasTemplates(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	var rows []entity.CanvasTemplate
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(expected) {
		t.Fatalf("seeded %d templates, want %d", len(rows), len(expected))
	}
	for _, row := range rows {
		if expected[row.ID] == nil {
			t.Errorf("unexpected seeded identity %q", row.ID)
		}
	}
	assertEmbeddedCanvasCatalog(t, db)
}
