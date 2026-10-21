package dao_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/chunker"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/parser/parser"
	"ragflow/internal/service"
)

// python027MaximalIngestionConfig carries every Python-era key the migration can
// convert, so one run exercises every converter and every target family.
const python027MaximalIngestionConfig = `{
	"layout_recognize":"TCADP Parser",
	"mineru_formula_enable":false,
	"mineru_table_enable":true,
	"pages":[[1,3]],
	"table_result_type":"2",
	"markdown_image_response_type":"2",
	"table_column_mode":"manual",
	"table_column_roles":{"a":"indexing"},
	"table_column_names":["a"],
	"chunk_token_num":256,
	"delimiter":"\n",
	"overlapped_percent":0.29,
	"table_context_size":10,
	"image_context_size":20,
	"parent_child":{"use_parent_child":true,"children_delimiter":"。"},
	"filename_embd_weight":0.2
}`

// TestMigratedIngestionConfigReachesRuntime drives the real row walk and then
// builds the real components from the stored config, so a migrated setting that
// the runtime cannot read fails here.
func TestMigratedIngestionConfigReachesRuntime(t *testing.T) {
	db := openIngestionRuntimeDB(t)
	insertIngestionRuntimeRow(t, db, "kb-general", "general",
		`{"layout_recognize":"plain text","pages":[[2,3]],"chunk_token_num":256,"delimiter":"\n","overlapped_percent":0.29,"filename_embd_weight":0.2}`)
	insertIngestionRuntimeRow(t, db, "kb-tcadp", "general", `{"layout_recognize":"TCADP Parser"}`)
	insertIngestionRuntimeRow(t, db, "kb-book", "book", `{"layout_recognize":true,"pages":[[1,2]],"chunk_token_num":300}`)
	if err := dao.MigrateIngestionParserConfigsForTest(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	t.Run("general", func(t *testing.T) {
		config := migratedConfig(t, db, "kb-general")
		if _, leftovers := config["layout_recognize"]; leftovers {
			t.Fatalf("flat key survived: %v", config)
		}
		effective := effectiveComponentParams(t, "general", config)

		pdf := parser.NewPDFParser()
		pdf.ConfigureFromSetup(effective["Parser:HipSignsRhyme"]["pdf"].(map[string]any))
		if pdf.ParseMethod != "plain_text" {
			t.Errorf("parse_method=%q; want plain_text", pdf.ParseMethod)
		}
		if !reflect.DeepEqual(pdf.Pages, [][]int{{2, 3}}) {
			t.Errorf("pages=%v; want [[2 3]]", pdf.Pages)
		}

		chunkerParams := effective["GeneralChunker:SixApplesFall"]
		if _, err := chunker.NewGeneralChunker(chunkerParams); err != nil {
			t.Fatalf("migrated chunker params are not runtime-acceptable: %v", err)
		}
		if got := schema.NormalizeOverlappedPercent(chunkerParams["overlapped_percent"]); got != 29 {
			t.Errorf("overlapped_percent normalizes to %v; want 29", got)
		}
		if got := chunkerParams["chunk_token_size"]; got != float64(256) {
			t.Errorf("chunk_token_size=%v; want 256", got)
		}

		tokenizerParams := effective["Tokenizer:LegalReadersDecide"]
		if _, err := component.NewTokenizerComponent(tokenizerParams); err != nil {
			t.Fatalf("migrated tokenizer params are not runtime-acceptable: %v", err)
		}
		if got := tokenizerParams["filename_embd_weight"]; got != 0.2 {
			t.Errorf("filename_embd_weight=%v; want 0.2", got)
		}
	})

	t.Run("TCADP matches the existing runtime", func(t *testing.T) {
		effective := effectiveComponentParams(t, "general", migratedConfig(t, db, "kb-tcadp"))
		node := effective["Parser:HipSignsRhyme"]
		pdf := node["pdf"].(map[string]any)["parse_method"].(string)
		if pdf != "tcadp parser" || !parser.IsPDFParseMethod(pdf) {
			t.Fatalf("migrated PDF engine would be dispatched as a VLM: %q", pdf)
		}
		if got := node["spreadsheet"].(map[string]any)["parse_method"]; got != "tcadp" {
			t.Fatalf("spreadsheet engine=%v; want tcadp", got)
		}
	})

	t.Run("book keeps the defaults its template bakes", func(t *testing.T) {
		config := migratedConfig(t, db, "kb-book")
		node, ok := config["TitleChunker:GrumpyGarlicsBake"].(map[string]any)
		if !ok {
			t.Fatalf("no TitleChunker node written: %v", config)
		}
		if node["chunk_token_cap"] != float64(300) {
			t.Fatalf("chunk_token_cap=%v; want 300", node["chunk_token_cap"])
		}
		effective := effectiveComponentParams(t, "book", config)
		pdf := parser.NewPDFParser()
		pdf.ConfigureFromSetup(effective["Parser:HipSignsRhyme"]["pdf"].(map[string]any))
		// The book template bakes remove_toc=true, which component.defaultSetups()
		// does not: writing the pdf family must carry it, or a migrated dataset
		// would start indexing its own table of contents.
		if !pdf.RemoveTOC {
			t.Error("remove_toc was lost; the migrated pdf family dropped a template default")
		}
		if pdf.EnableMultiColumn {
			t.Error("enable_multi_column leaked in from another template")
		}
		if !reflect.DeepEqual(pdf.Pages, [][]int{{1, 2}}) {
			t.Errorf("pages=%v; want [[1 2]]", pdf.Pages)
		}
	})
}

// TestMigratedMinerUControlsReachRequest runs SQLite migration, stored-config
// readback, real template cleaning and Parser invocation. Only the MinerU HTTP
// service is a fixture; no OCR inference runs. Local HTTP requires a scoped,
// process-wide SSRF bypass, so this test and its subtests must not run in parallel.
func TestMigratedMinerUControlsReachRequest(t *testing.T) {
	t.Setenv(common.EnvMineruBackend, "pipeline")
	previous := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = true
	t.Cleanup(func() { common.AllowAnyHostForTest = previous })

	var response bytes.Buffer
	archive := zip.NewWriter(&response)
	entry, err := archive.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`[{"type":"text","text":"Migrated MinerU controls reached the request."}]`)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	template, ok := registry.Get("general")
	if !ok {
		t.Fatal("general pipeline template missing")
	}
	dsl, err := json.Marshal(template.DSL)
	if err != nil {
		t.Fatal(err)
	}

	for _, formula := range []bool{false, true} {
		for _, table := range []bool{false, true} {
			t.Run(fmt.Sprintf("formula=%t/table=%t", formula, table), func(t *testing.T) {
				requests := make(chan [2]string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					if request.URL.Path != "/file_parse" || request.Method != http.MethodPost {
						http.NotFound(w, request)
						return
					}
					if err := request.ParseMultipartForm(1 << 20); err != nil {
						t.Errorf("parse multipart request: %v", err)
						http.Error(w, "invalid fixture request", http.StatusBadRequest)
						return
					}
					defer request.MultipartForm.RemoveAll()
					if request.Header.Get("Authorization") != "Bearer synthetic-mineru-key" {
						t.Error("selected instance credentials were lost")
					}
					select {
					case requests <- [2]string{request.FormValue("formula_enable"), request.FormValue("table_enable")}:
					default:
						t.Error("unexpected repeated MinerU submission")
					}
					w.Header().Set("Content-Type", "application/zip")
					_, _ = w.Write(response.Bytes())
				}))
				t.Cleanup(server.Close)
				db := openIngestionRuntimeDB(t)
				if err := db.AutoMigrate(&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
					t.Fatal(err)
				}
				extra, err := json.Marshal(map[string]string{"base_url": server.URL})
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range []any{
					&entity.TenantModelProvider{ID: "provider-mineru", TenantID: "tenant", ProviderName: "MinerU"},
					&entity.TenantModelInstance{ID: "instance-mineru", ProviderID: "provider-mineru", InstanceName: "default", APIKey: "synthetic-mineru-key", Status: "active", Extra: string(extra)},
					&entity.TenantModel{ID: "model-mineru", ProviderID: "provider-mineru", InstanceID: "instance-mineru", ModelName: "MinerU-model", ModelType: int(entity.ModelTypeOCR), Status: "active"},
				} {
					if err := db.Create(row).Error; err != nil {
						t.Fatal(err)
					}
				}
				input := fmt.Sprintf(`{"layout_recognize":"MinerU-model@default@MinerU","mineru_formula_enable":%t,"mineru_table_enable":%t}`, formula, table)
				insertIngestionRuntimeRow(t, db, "kb-mineru", "general", input)
				if err := dao.MigrateIngestionParserConfigsForTest(t.Context(), db); err != nil {
					t.Fatal(err)
				}
				config := pipeline.BuildParserConfig(dsl, migratedConfig(t, db, "kb-mineru"))
				effective := effectiveComponentParams(t, "general", config)
				parser, err := component.NewParserComponent(effective["Parser:HipSignsRhyme"])
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				out, err := parser.Invoke(ctx, db, map[string]any{
					"binary": []byte("%PDF-1.4"), "name": "synthetic.pdf", "file_type": "pdf", "tenant_id": "tenant",
				})
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(out)
				if err != nil || !strings.Contains(string(encoded), "Migrated MinerU controls reached the request.") {
					t.Fatalf("Parser output did not contain fixture text: %s, err=%v", encoded, err)
				}
				select {
				case got := <-requests:
					want := [2]string{strconv.FormatBool(formula), strconv.FormatBool(table)}
					if got != want {
						t.Errorf("migrated formula_enable/table_enable=%v, want %v", got, want)
					}
				default:
					t.Fatal("no MinerU request received")
				}
			})
		}
	}
}

// TestMigratedParentChildMatchesWritePath asserts the migration derives
// children_delimiters exactly as the API's own ApplyParentChildChunkerConfig
// does for the same setting, so a migrated row and a row saved through the UI
// agree. children_delimiters is the only half the chunker reads at run time.
func TestMigratedParentChildMatchesWritePath(t *testing.T) {
	const nodeID = "GeneralChunker:SixApplesFall"
	for _, tc := range []struct {
		name  string
		input string
		want  []any
	}{
		{"group with delimiter", `{"parent_child":{"use_parent_child":true,"children_delimiter":"。"}}`, []any{"。"}},
		{"group with multi delimiter", `{"parent_child":{"use_parent_child":true,"children_delimiter":"。；"}}`, []any{"。", "；"}},
		{"legacy flat keys", `{"enable_children":true,"children_delimiter":"。"}`, []any{"。"}},
		{"disabled", `{"enable_children":false,"children_delimiter":"。"}`, []any{}},
		{"disabled group", `{"parent_child":{"use_parent_child":false}}`, []any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := decodeIngestionConfig(t, tc.input)
			if _, err := dao.MigrateParserChunkerTokenizerConfigForTest(config, "general", nil, "knowledgebase", "row-1"); err != nil {
				t.Fatal(err)
			}
			node, ok := config[nodeID].(map[string]any)
			if !ok {
				t.Fatalf("no chunker node written: %v", config)
			}
			got := node["children_delimiters"]
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("children_delimiters=%v; want %v", got, tc.want)
			}
			// The write path, given the same component config, must derive the
			// same list. It only ever reads the "parent_child" group.
			apiSided := entity.JSONMap{nodeID: cloneConfig(node)}
			pipeline.ApplyParentChildChunkerConfig(apiSided, nil)
			derived := apiSided[nodeID].(map[string]any)["children_delimiters"]
			if !reflect.DeepEqual(jsonNormalize(t, got), jsonNormalize(t, derived)) {
				t.Fatalf("migration wrote %v; the write path derives %v from the same group", got, derived)
			}
			if _, ok := node["parent_child"].(map[string]any); !ok {
				t.Fatalf("the parent_child group was not written: %v", node)
			}
		})
	}
}

// TestMigratedParentChildEnabledWithoutDelimiterDefaultsToNewline pins the one
// place the migration deliberately differs from ApplyParentChildChunkerConfig:
// Python's ParentChildConfig defaults children_delimiter to "\n" and its
// flattening writes that default into the config, while the Go write path
// derives an empty list when the delimiter is absent. Keeping Python's default
// preserves the children the Python-era run produced.
func TestMigratedParentChildEnabledWithoutDelimiterDefaultsToNewline(t *testing.T) {
	config := decodeIngestionConfig(t, `{"parent_child":{"use_parent_child":true}}`)
	if _, err := dao.MigrateParserChunkerTokenizerConfigForTest(config, "general", nil, "knowledgebase", "row-1"); err != nil {
		t.Fatal(err)
	}
	node := config["GeneralChunker:SixApplesFall"].(map[string]any)
	group := node["parent_child"].(map[string]any)
	if group["children_delimiter"] != "\n" {
		t.Fatalf("children_delimiter=%v; want the Python default \"\\n\"", group["children_delimiter"])
	}
	if !reflect.DeepEqual(node["children_delimiters"], []any{"\n"}) {
		t.Fatalf("children_delimiters=%v; want [\"\\n\"]", node["children_delimiters"])
	}
}

// TestIngestionMigrationMatchesBuiltinTemplates pins the migration's literal
// target table against the embedded templates by reading the node IDs and file
// families out of the templates themselves: a renamed component, a dropped
// family, or a new chunker type fails here instead of silently skipping a row or
// thinning a family.
func TestIngestionMigrationMatchesBuiltinTemplates(t *testing.T) {
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// targetFamilies are the only Parser families the migration can write.
	const targetFamilies = "pdf,spreadsheet"
	for _, ref := range append(registry.Refs(), "naive") {
		t.Run(ref, func(t *testing.T) {
			template, ok := registry.Get(ref)
			if !ok {
				t.Fatal("template missing")
			}
			components := templateComponents(t, template.DSL)
			parserCpn := soleComponent(t, components, "Parser")
			chunkerCpn := soleComponentSuffix(t, components, "Chunker")
			tokenizerCpn := soleComponent(t, components, "Tokenizer")
			templateFamilies := parserFamilies(t, parserCpn.params)
			rawDSL, err := json.Marshal(template.DSL)
			if err != nil {
				t.Fatal(err)
			}

			for _, tc := range []struct {
				name string
				// config is the Python-era row being migrated.
				config string
				// families are the template families the config must land on;
				// every other target family must be left alone.
				families map[string]bool
				// chunker reports whether the config carries a chunker-owned key.
				chunker bool
				// tokenizer reports whether the config carries a tokenizer-owned key.
				tokenizer bool
			}{
				{
					name:      "maximal",
					config:    python027MaximalIngestionConfig,
					families:  map[string]bool{"pdf": true, "spreadsheet": true},
					chunker:   true,
					tokenizer: true,
				},
				{
					name:     "engine",
					config:   `{"layout_recognize":"TCADP Parser"}`,
					families: map[string]bool{"pdf": true, "spreadsheet": true},
				},
				{
					name:     "columns",
					config:   `{"table_column_mode":"manual","table_column_roles":{"a":"indexing"},"table_column_names":["a"]}`,
					families: map[string]bool{"spreadsheet": true},
				},
				{
					name:     "MinerU controls",
					config:   `{"mineru_formula_enable":false,"mineru_table_enable":true}`,
					families: map[string]bool{"pdf": true},
				},
				{
					name:     "chunking",
					config:   `{"chunk_token_num":256,"delimiter":"\n","overlapped_percent":0.29,"enable_children":true}`,
					families: map[string]bool{},
					chunker:  true,
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					config := decodeIngestionConfig(t, tc.config)
					changed, err := dao.MigrateParserChunkerTokenizerConfigForTest(config, ref, nil, "knowledgebase", "row-1")
					if err != nil || !changed {
						t.Fatalf("changed=%v err=%v", changed, err)
					}

					if tc.name == "maximal" && len(config) == 0 {
						t.Fatal("migration wrote nothing for the maximal config")
					}
					// Snapshot before cleaning so mutations cannot hide a lost parameter.
					expected := jsonNormalize(t, config)
					cleaned := pipeline.CleanComponentParams(rawDSL, config)
					if !reflect.DeepEqual(jsonNormalize(t, cleaned), expected) {
						t.Fatalf("API cleaning changed migrated params: got %v; want %v", cleaned, expected)
					}

					// Parser: exactly the template's families the config targets,
					// never a family the template does not declare.
					wantFamilies := map[string]bool{}
					for family := range tc.families {
						if _, present := templateFamilies[family]; present {
							wantFamilies[family] = true
						}
					}
					gotFamilies := map[string]bool{}
					node, nodeWritten := config[parserCpn.cpnID].(map[string]any)
					for family, raw := range node {
						familyParams, isMap := raw.(map[string]any)
						if !isMap {
							t.Fatalf("family %q is not an object: %v", family, raw)
						}
						if !strings.Contains(targetFamilies, family) {
							t.Fatalf("migration wrote family %q, which is not a migration target: %v", family, familyParams)
						}
						gotFamilies[family] = true
					}
					if !reflect.DeepEqual(gotFamilies, wantFamilies) {
						t.Fatalf("Parser families %v; want %v (config=%s)", gotFamilies, wantFamilies, tc.config)
					}
					if len(wantFamilies) == 0 && nodeWritten {
						t.Fatalf("Parser node written for a config that targets no family: %v", node)
					}
					if nodeWritten {
						assertFamiliesPreserved(t, node, templateFamilies, wantFamilies)
					}
					if (tc.name == "maximal" || tc.name == "MinerU controls") && wantFamilies["pdf"] {
						pdf := node["pdf"].(map[string]any)
						for key, want := range map[string]bool{"mineru_formula_enable": false, "mineru_table_enable": true} {
							if got, exists := pdf[key]; !exists || got != want {
								t.Errorf("migrated %s=%v, present=%v; want %v", key, got, exists, want)
							}
						}
					}
					// The Parser the run compiles must accept the override. The
					// merge is applyOverrideParams': the template params with the
					// stored component params on top.
					if _, err := component.NewParserComponent(mergeComponentParams(parserCpn.params, node)); err != nil {
						t.Fatalf("migrated Parser params do not compile: %v", err)
					}

					// Chunker: only the chunkers that own the migrated params
					// may receive a node, on the template's own node ID.
					chunkerNode, chunkerWritten := config[chunkerCpn.cpnID].(map[string]any)
					switch {
					case !tc.chunker:
						if chunkerWritten {
							t.Fatalf("chunker node written for a chunker-agnostic config: %v", chunkerNode)
						}
					case chunkerCpn.name == "GeneralChunker" || chunkerCpn.name == "TokenChunker":
						if !chunkerWritten {
							t.Fatalf("no %s node written", chunkerCpn.cpnID)
						}
						if _, err := chunkerNodeAccepts(chunkerCpn.name, mergeComponentParams(chunkerCpn.params, chunkerNode)); err != nil {
							t.Fatalf("migrated chunker params do not compile: %v", err)
						}
					case chunkerCpn.name == "TitleChunker":
						if !chunkerWritten {
							t.Fatalf("no %s node written", chunkerCpn.cpnID)
						}
						if value, ok := schema.NumericFromAny(chunkerNode["chunk_token_cap"]); !ok || value != 256 {
							t.Fatalf("chunk_token_cap=%v; want 256", chunkerNode["chunk_token_cap"])
						}
					default:
						if chunkerWritten {
							t.Fatalf("%s must not receive migrated chunker params: %v", chunkerCpn.name, chunkerNode)
						}
					}

					// Tokenizer: every template has one, and the filename weight
					// is the only param the migration puts on it.
					tokenizerNode, tokenizerWritten := config[tokenizerCpn.cpnID].(map[string]any)
					if !tc.tokenizer {
						if tokenizerWritten {
							t.Fatalf("tokenizer node written for a config with no tokenizer param: %v", tokenizerNode)
						}
					} else {
						if !tokenizerWritten {
							t.Fatalf("no %s node written", tokenizerCpn.cpnID)
						}
						if value, ok := schema.NumericFromAny(tokenizerNode["filename_embd_weight"]); !ok || value != 0.2 {
							t.Fatalf("filename_embd_weight=%v; want 0.2", tokenizerNode["filename_embd_weight"])
						}
						if _, err := component.NewTokenizerComponent(mergeComponentParams(tokenizerCpn.params, tokenizerNode)); err != nil {
							t.Fatalf("migrated tokenizer params do not compile: %v", err)
						}
					}

					// No node outside the migration's three targets.
					for cpnID := range config {
						if cpnID == parserCpn.cpnID || cpnID == chunkerCpn.cpnID || cpnID == tokenizerCpn.cpnID {
							continue
						}
						t.Fatalf("migration wrote an unexpected component: %q", cpnID)
					}
				})
			}
		})
	}
}

func chunkerNodeAccepts(name string, params map[string]any) (any, error) {
	if name == "TokenChunker" {
		return chunker.NewTokenChunker(params)
	}
	return chunker.NewGeneralChunker(params)
}

// mergeComponentParams overlays the component-scoped overrides on the template's
// baked params the way internal/agent/canvas.applyOverrideParams does at run
// time: the override wins on the top-level key, so an override replaces a whole
// Parser family.
func mergeComponentParams(templateParams, overrides map[string]any) map[string]any {
	out := make(map[string]any, len(templateParams)+len(overrides))
	for key, value := range templateParams {
		out[key] = value
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

// templateComponent is one component of a builtin template.
type templateComponent struct {
	cpnID  string
	name   string
	params map[string]any
}

func templateComponents(t *testing.T, dsl map[string]any) []templateComponent {
	t.Helper()
	raw, err := json.Marshal(dsl)
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := pipeline.ExtractAllComponentParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]templateComponent, 0, len(schemas))
	for _, s := range schemas {
		out = append(out, templateComponent{cpnID: s.CpnID, name: s.ComponentName, params: s.ParamsDefaults})
	}
	return out
}

func soleComponent(t *testing.T, components []templateComponent, name string) templateComponent {
	t.Helper()
	found := templateComponent{}
	count := 0
	for _, c := range components {
		if c.name == name {
			found, count = c, count+1
		}
	}
	if count != 1 {
		t.Fatalf("template has %d %s components; want 1", count, name)
	}
	return found
}

func soleComponentSuffix(t *testing.T, components []templateComponent, suffix string) templateComponent {
	t.Helper()
	found := templateComponent{}
	count := 0
	for _, c := range components {
		if strings.HasSuffix(c.name, suffix) {
			found, count = c, count+1
		}
	}
	if count != 1 {
		t.Fatalf("template has %d *%s components; want 1", count, suffix)
	}
	return found
}

func parserFamilies(t *testing.T, params map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, key := range []string{"pdf", "spreadsheet"} {
		if raw, exists := params[key]; exists {
			family, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("Parser family %q is not an object", key)
			}
			out[key] = family
		}
	}
	return out
}

// overriddenFamilyKeys are family params the migration overwrites by design
// whenever the config supplies them, so the migrated value may differ from the
// template's.
var overriddenFamilyKeys = map[string]bool{
	"parse_method":                 true,
	"pages":                        true,
	"column_mode":                  true,
	"column_roles":                 true,
	"column_names":                 true,
	"layout_recognizer":            true,
	"table_result_type":            true,
	"markdown_image_response_type": true,
}

// inertFamilyKeys are template-baked family keys the migration may leave out,
// with the reason dropping them is harmless. component.NewParserComponent seeds
// every family from defaultSetups() and then overlays the params, and the
// run-level override replaces a family wholesale, so whatever a written family
// leaves out comes from defaultSetups() — equal to the template's value, or
// read by nobody, for every entry below.
//
// A key may appear here and in overriddenFamilyKeys: the two answer different
// questions (this one, "is the template's value safe to lose"; that one, "may
// the value the migration writes differ from the template's").
var inertFamilyKeys = map[string]string{
	"output_format":         "normalizeParserOutputFormats forces JSON on every family",
	"preprocess":            "the value has no Go reader; the preprocessing stage is selected by the chunker's own options",
	"suffix":                "no Go reader; file routing uses the document's own suffix column, not the family list",
	"vlm":                   "empty in every template; absent falls back to the family llm_id and then the tenant model, and only Markdown and audio read a family vlm at all",
	"flatten_media_to_text": "false in every template, which is also defaultSetups()'s default",
	"pages":                 "the only template that bakes it lists every page ([[1,100000]]), which is what an absent pages means",
	"parse_method":          "a written pdf family always restates it; for the spreadsheet family defaultSetups() re-derives the same deepdoc engine",
	"column_mode":           "the table template bakes auto, which is what an absent column_mode means to indexdoc's resolveTableColumnConfig",
	"column_roles":          "the table template bakes the empty role map, which is what an absent column_roles means",
	"column_names":          "the table template bakes the empty name list, which is what an absent column_names means",
}

// assertFamiliesPreserved checks that no template-baked key of a written family
// is left unaccounted for: a key that would simply disappear from the effective
// config must be either carried or documented above.
func assertFamiliesPreserved(t *testing.T, node map[string]any, templateFamilies map[string]map[string]any, written map[string]bool) {
	t.Helper()
	for family := range written {
		templateFamily, ok := templateFamilies[family]
		if !ok {
			t.Fatalf("migration wrote family %q, which the template does not declare", family)
		}
		familyParams, _ := node[family].(map[string]any)
		for key, templateValue := range templateFamily {
			got, present := familyParams[key]
			if !present {
				if _, inert := inertFamilyKeys[key]; !inert {
					t.Fatalf("family %s: template key %q would be dropped by the override and is not accounted for; carry it or document it as inert", family, key)
				}
				continue
			}
			if overriddenFamilyKeys[key] {
				continue
			}
			if !reflect.DeepEqual(got, templateValue) {
				t.Fatalf("family %s: key %q = %v, but the template bakes %v", family, key, got, templateValue)
			}
		}
	}
}

// effectiveComponentParams reproduces the run-level merge: the template's baked
// component params with the stored parser_config overlaid, the stored value
// winning on the top-level key (internal/agent/canvas.applyOverrideParams), then
// component.NewParserComponent's own defaults underneath.
func effectiveComponentParams(t *testing.T, parserID string, config map[string]any) map[string]map[string]any {
	t.Helper()
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	template, ok := registry.Get(parserID)
	if !ok {
		t.Fatal("template missing")
	}
	out := map[string]map[string]any{}
	for _, c := range templateComponents(t, template.DSL) {
		out[c.cpnID] = c.params
	}
	for cpnID, raw := range config {
		params, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("stored cpnID %q is not an object", cpnID)
		}
		out[cpnID] = mergeComponentParams(out[cpnID], params)
	}
	return out
}

func openIngestionRuntimeDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertIngestionRuntimeRow(t *testing.T, db *gorm.DB, id, parserID, config string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO knowledgebase (id,tenant_id,name,embd_id,created_by,parser_id,parser_config) VALUES (?,?,?,?,?,?,?)`,
		id, "tenant-1", "dataset", "embedding", "user-1", parserID, config).Error
	if err != nil {
		t.Fatal(err)
	}
}

func migratedConfig(t *testing.T, db *gorm.DB, id string) map[string]any {
	t.Helper()
	var raw string
	if err := db.Table("knowledgebase").Where("id = ?", id).Pluck("parser_config", &raw).Error; err != nil {
		t.Fatal(err)
	}
	return decodeIngestionConfig(t, raw)
}

func decodeIngestionConfig(t *testing.T, raw string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	return config
}

// jsonNormalize round-trips a value through JSON so two structurally equal
// values that differ only in Go slice type ([]string vs []any, which is what a
// DB round-trip produces either way) compare equal.
func jsonNormalize(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// cloneConfig deep-copies a parser_config so a helper that mutates its argument
// cannot corrupt the caller's fixture.
func cloneConfig(config map[string]any) map[string]any {
	raw, err := json.Marshal(config)
	if err != nil {
		return config
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return config
	}
	return out
}

func TestMigratedExtractorConfigReachesRuntime(t *testing.T) {
	db := openIngestionRuntimeDB(t)
	input := `{"auto_keywords":8,"auto_questions":2,"enable_metadata":true,"metadata":[{"key":"author","type":"string"}],"built_in_metadata":[{"key":"file_name","type":"string"}],"Extractor:AutoExtractDefault":{"tags":{"top_n":5,"tag_file_id":"tag-file"},"keywords":{"system_prompt":"custom keywords"},"summary":{"enabled":true,"system_prompt":"custom summary"}}}`
	insertIngestionRuntimeRow(t, db, "kb-1", "general", input)
	if err := dao.MigrateIngestionParserConfigsForTest(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	config := migratedConfig(t, db, "kb-1")
	got := extractorFromMigratedConfig(t, config).Param
	if !got.Keywords.Enabled || got.Keywords.TopN != 8 || got.Keywords.SystemPrompt != "custom keywords" || !got.Questions.Enabled || got.Questions.TopN != 2 {
		t.Fatalf("keyword/question settings lost: %#v", got)
	}
	if !got.Metadata.Enabled || len(got.Metadata.Metadata) != 1 || got.Metadata.Metadata[0].Key != "author" || len(got.Metadata.BuiltInMetadata) != 1 || got.Metadata.BuiltInMetadata[0].Key != "file_name" {
		t.Fatalf("metadata=%#v", got.Metadata)
	}
	if !got.Tags.Enabled || got.Tags.TagFileID != "tag-file" || got.Tags.TopN != 5 || !got.Summary.Enabled || got.Summary.SystemPrompt != "custom summary" || got.LLMID != "tenant-llm" {
		t.Fatalf("existing scoped settings lost: %#v", got)
	}
}

func TestExtractorMigrationMatchesBuiltinTemplates(t *testing.T) {
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range append(registry.Refs(), "naive") {
		t.Run(ref, func(t *testing.T) {
			template, ok := registry.Get(ref)
			if !ok {
				t.Fatal("template missing")
			}
			components := template.DSL["components"].(map[string]any)
			_, hasExtractor := components["Extractor:AutoExtractDefault"]
			config := map[string]any{"auto_keywords": float64(8)}
			changed, err := dao.MigrateExtractorConfigForTest(config, ref, nil)
			if err != nil {
				t.Fatal(err)
			}
			if changed != hasExtractor {
				t.Fatalf("changed=%v; template has Extractor=%v", changed, hasExtractor)
			}
			if hasExtractor {
				if node, ok := config["Extractor:AutoExtractDefault"].(map[string]any); !ok || node["keywords"].(map[string]any)["top_n"] != 8 {
					t.Fatalf("config=%v", config)
				}
			} else if config["auto_keywords"] != float64(8) {
				t.Fatal("settings without target were discarded")
			}
		})
	}
}

func TestExtractorMigrationPreservesRuntimeSettings(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		check       func(*testing.T, *component.ExtractorComponent)
	}{
		{
			name:  "empty Python metadata keeps builtins",
			input: `{"enable_metadata":true,"metadata":{},"built_in_metadata":[{"key":"file_name"}]}`,
			check: func(t *testing.T, extractor *component.ExtractorComponent) {
				got := extractor.Param.Metadata
				if !got.Enabled || len(got.BuiltInMetadata) != 1 || got.BuiltInMetadata[0].Key != "file_name" {
					t.Fatalf("metadata=%#v", got)
				}
			},
		},
		{
			name:  "builtin uses active extractor despite stale node",
			input: `{"auto_keywords":8,"Extractor:Old":{"summary":{"enabled":true}}}`,
			check: func(t *testing.T, extractor *component.ExtractorComponent) {
				got := extractor.Param.Keywords
				if !got.Enabled || got.TopN != 8 {
					t.Fatalf("keywords=%#v", got)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var config map[string]any
			if err := json.Unmarshal([]byte(tt.input), &config); err != nil {
				t.Fatal(err)
			}
			if changed, err := dao.MigrateExtractorConfigForTest(config, "general", nil); err != nil || !changed {
				t.Fatalf("changed=%v, err=%v", changed, err)
			}
			tt.check(t, extractorFromMigratedConfig(t, config))
		})
	}
}

func extractorFromMigratedConfig(t *testing.T, config map[string]any) *component.ExtractorComponent {
	t.Helper()
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	template, ok := registry.Get("general")
	if !ok {
		t.Fatal("general template missing")
	}
	dsl, err := json.Marshal(template.DSL)
	if err != nil {
		t.Fatal(err)
	}
	cleaned := service.ApplyComponentScopedParserConfig(pipeline.BuildParserConfig(dsl, config), "tenant-llm")
	c, err := component.NewExtractorComponent(cleaned["Extractor:AutoExtractDefault"].(map[string]any))
	if err != nil {
		t.Fatal(err)
	}
	return c.(*component.ExtractorComponent)
}
