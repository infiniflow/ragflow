//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package dao

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	parserchunk "ragflow/internal/parser/chunk"
	parserpkg "ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

// This step follows rc2.dev2 (the extractor migration): database migrations
// share one version marker, so each step's target sorts above the previous one.
const ingestionParserConfigTargetVersion = "v1.0.0-rc2.dev3"

// Python v0.27.2 kept a dataset's Parser / Chunker / Tokenizer settings at the
// top level of parser_config, where rag/app/<parser>.py read them. Go reads
// component-scoped params only: internal/service/dataset/helpers.go
// DropUnscopedParserConfigKeys drops every flat key at the API boundary and
// internal/ingestion/pipeline.CleanComponentParams warns and drops them again on
// the write path. A row that still carries flat keys therefore silently loses
// its engine choice, page range, chunk size/delimiters/overlap, parent-child
// split and filename weight the next time it is written through the API.
//
// This step converts those keys into the components of the pipeline the row
// itself runs, producing the same component-scoped shape the current API would
// have stored for the equivalent request: the same derivations
// (parserchunk.ParseDelimiterField for delimiter lists,
// utility.NormalizePDFPages for page ranges, parent_child -> children_delimiters)
// and the same "an existing param wins" rule.
//
// The DAO layer cannot read the builtin templates to find those node ids:
// internal/ingestion/pipeline transitively imports this package, so importing it
// here would be an import cycle. The target table below is therefore literal and
// TestIngestionMigrationTargetsMatchBuiltinTemplates pins it against the
// embedded templates, so a renamed node, a changed file family or a new
// template-baked key fails a test instead of silently skipping or thinning rows.

// builtinIngestionTarget is where one builtin pipeline keeps the components the
// Python-era flat settings move to.
type builtinIngestionTarget struct {
	parserNode     string
	parserFamilies map[string]struct{}
	// parserFamilyCarried holds template-baked family params that the migration
	// must re-state whenever it writes that family. A run-level component
	// override replaces a whole family map (internal/agent/canvas
	// applyOverrideParams wins on the top-level key, then
	// component.NewParserComponent re-derives only its own defaults), so a
	// migration that wrote {"pdf": {"parse_method": ...}} onto a row whose
	// Parser node does not exist yet would drop everything else the template
	// bakes for that family. Only keys whose value differs from
	// component.defaultSetups() AND changes behaviour are listed; the rest are
	// either re-derived here, equal to the runtime default, or read by nobody.
	// TestIngestionMigrationTargetsMatchBuiltinTemplates asserts each entry
	// still matches its template and that no template key is left unaccounted
	// for.
	parserFamilyCarried map[string]map[string]any
	chunker             string
	chunkerNode         string
	tokenizerNode       string
}

func (t builtinIngestionTarget) hasFamily(family string) bool {
	_, ok := t.parserFamilies[family]
	return ok
}

// acceptsChunkParams reports whether the chunker accepts delimiter, overlap,
// context and parent/child parameters.
func (t builtinIngestionTarget) acceptsChunkParams() bool {
	return t.chunker == "GeneralChunker" || t.chunker == "TokenChunker"
}

func ingestionFamilies(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

// removeTOCDefault is the pdf-family value the book and laws templates bake;
// component.defaultSetups() has remove_toc=false, so it must be carried or a
// migrated book/laws row would start indexing its table of contents.
func removeTOCDefault() map[string]any { return map[string]any{"remove_toc": true} }

// multiColumnDefault is the pdf-family value the paper template bakes;
// defaultSetups() has no enable_multi_column at all, so losing it would parse
// two-column papers in the wrong reading order.
func multiColumnDefault() map[string]any { return map[string]any{"enable_multi_column": true} }

// ingestionTargets maps a builtin parser_id to its component nodes. The family
// list mirrors the Parser node's params in each embedded template: writing into
// a family the template does not carry would be dropped by CleanComponentParams
// and never reach the parser.
var ingestionTargets = func() map[string]builtinIngestionTarget {
	// "naive" is the historical name of the general pipeline: the embedded
	// general template declares parser_ids ["naive"] and the registry aliases
	// "general" onto it.
	general := builtinIngestionTarget{
		parserNode:     "Parser:HipSignsRhyme",
		parserFamilies: ingestionFamilies("doc", "docx", "html", "markdown", "pdf", "spreadsheet", "text&code"),
		chunker:        "GeneralChunker",
		chunkerNode:    "GeneralChunker:SixApplesFall",
		tokenizerNode:  "Tokenizer:LegalReadersDecide",
	}
	return map[string]builtinIngestionTarget{
		"general": general,
		"naive":   general,
		"qa": {
			parserNode:     "Parser:HipSignsRhyme",
			parserFamilies: ingestionFamilies("docx", "markdown", "pdf", "spreadsheet", "text&code"),
			chunker:        "QAChunker",
			chunkerNode:    "QAChunker:TidyCloudsThink",
			tokenizerNode:  "Tokenizer:ColdCloudsDream",
		},
		"manual": {
			parserNode:     "Parser:HipSignsRhyme",
			parserFamilies: ingestionFamilies("doc", "docx", "pdf"),
			chunker:        "ManualChunker",
			chunkerNode:    "ManualChunker:NineInsectsFind",
			tokenizerNode:  "Tokenizer:FunnyBalloonsGrin",
		},
		"table": {
			parserNode:     "Parser:HipSignsRhyme",
			parserFamilies: ingestionFamilies("spreadsheet", "text&code"),
			chunker:        "TableChunker",
			chunkerNode:    "TableChunker:FastFoxesJump",
			tokenizerNode:  "Tokenizer:DeepLakesShine",
		},
		"paper": {
			parserNode:          "Parser:HipSignsRhyme",
			parserFamilies:      ingestionFamilies("pdf"),
			parserFamilyCarried: map[string]map[string]any{"pdf": multiColumnDefault()},
			chunker:             "TitleChunker",
			chunkerNode:         "TitleChunker:SparklySchoolsTravel",
			tokenizerNode:       "Tokenizer:GreatCarsWash",
		},
		"book": {
			parserNode:          "Parser:HipSignsRhyme",
			parserFamilies:      ingestionFamilies("doc", "docx", "html", "pdf", "text&code"),
			parserFamilyCarried: map[string]map[string]any{"pdf": removeTOCDefault()},
			chunker:             "TitleChunker",
			chunkerNode:         "TitleChunker:GrumpyGarlicsBake",
			tokenizerNode:       "Tokenizer:HotDonutsRing",
		},
		"laws": {
			parserNode:          "Parser:HipSignsRhyme",
			parserFamilies:      ingestionFamilies("doc", "docx", "html", "markdown", "pdf", "text&code"),
			parserFamilyCarried: map[string]map[string]any{"pdf": removeTOCDefault()},
			chunker:             "TitleChunker",
			chunkerNode:         "TitleChunker:SpicyKeysKick",
			tokenizerNode:       "Tokenizer:PublicJobsTake",
		},
		"presentation": {
			parserNode:     "Parser:HipSignsRhyme",
			parserFamilies: ingestionFamilies("pdf", "slides"),
			chunker:        "PageChunker",
			chunkerNode:    "PageChunker:HappyHillsGlow",
			tokenizerNode:  "Tokenizer:TallTreesDance",
		},
		"picture": {
			parserNode:     "Parser:ViewsCaptureLight",
			parserFamilies: ingestionFamilies("image", "video"),
			chunker:        "OneChunker",
			chunkerNode:    "OneChunker:BrightColorsGlow",
			tokenizerNode:  "Tokenizer:SharpLensFocus",
		},
		"one": {
			parserNode:     "Parser:HipSignsRhyme",
			parserFamilies: ingestionFamilies("doc", "docx", "html", "markdown", "pdf", "spreadsheet", "text&code"),
			chunker:        "OneChunker",
			chunkerNode:    "OneChunker:DryDrinksVisit",
			tokenizerNode:  "Tokenizer:FrankWeeksListen",
		},
		"audio": {
			parserNode:     "Parser:SongsFillAir",
			parserFamilies: ingestionFamilies("audio"),
			chunker:        "OneChunker",
			chunkerNode:    "OneChunker:BlueSkiesLaugh",
			tokenizerNode:  "Tokenizer:KindEyesWatch",
		},
		"email": {
			parserNode:     "Parser:BirdsFlutterHigh",
			parserFamilies: ingestionFamilies("email"),
			chunker:        "TokenChunker",
			chunkerNode:    "TokenChunker:WarmBreadSmells",
			tokenizerNode:  "Tokenizer:NiceWordsSpoken",
		},
	}
}()

// ingestionParserFlatKeys are the Python-era top level keys this step converts.
var (
	ingestionParserFlatKeys = []string{
		"layout_recognize",
		"mineru_parse_method",
		"mineru_lang",
		"pages",
		"table_result_type",
		"markdown_image_response_type",
		"table_column_mode",
		"table_column_roles",
		"table_column_names",
	}
	ingestionChunkerFlatKeys = []string{
		"chunk_token_num",
		"delimiter",
		"overlapped_percent",
		"table_context_size",
		"image_context_size",
		"parent_child",
		"enable_children",
		"children_delimiter",
	}
	ingestionTokenizerFlatKeys = []string{"filename_embd_weight"}
)

// ingestionRetiredFlatKeys are Python settings that have no Go consumer at all.
// They are deleted so the stored config stops advertising behaviour the Go
// runtime does not implement; every drop is logged with table, row and field.
var ingestionRetiredFlatKeys = []string{
	// Spreadsheets are always rendered as segmented HTML; the flag only warns
	// now (internal/parser/parser/office_table_render.go:deprecatedHTML4Excel).
	"html4excel",
	// Splits one parse into several ingestion jobs; it is not a page range and
	// not a chunk size.
	"task_page_size",
	// Follows hyperlinks and indexes the linked content; unimplemented in Go.
	"analyze_hyperlink",
	"hyperlink_urls",
	// Video parsing is rejected at dispatch
	// (internal/ingestion/component/media_dispatch.go:maybeDispatchVideo).
	"video_prompt",
	// These MinerU toggles have no Go reader; method and language are migrated.
	"mineru_formula_enable",
	"mineru_table_enable",
}

// ingestionOwnedFlatKeys is every flat key this step consumes or retires. A row
// carrying none of them is left completely untouched, which is what makes a
// second run a no-op.
func ingestionOwnedFlatKeys() []string {
	owned := make([]string, 0, len(ingestionParserFlatKeys)+len(ingestionChunkerFlatKeys)+
		len(ingestionTokenizerFlatKeys)+len(ingestionRetiredFlatKeys))
	owned = append(owned, ingestionParserFlatKeys...)
	owned = append(owned, ingestionChunkerFlatKeys...)
	owned = append(owned, ingestionTokenizerFlatKeys...)
	owned = append(owned, ingestionRetiredFlatKeys...)
	return owned
}

// ingestionConfigMigrationRow is one dataset or document row.
type ingestionConfigMigrationRow struct {
	ID           string  `gorm:"column:id"`
	ParserID     string  `gorm:"column:parser_id"`
	PipelineID   *string `gorm:"column:pipeline_id"`
	ParserConfig *string `gorm:"column:parser_config"`
}

func migrateIngestionParserConfigs(ctx context.Context, db *gorm.DB) error {
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return err
	}
	if shouldSkipMigration(version, ingestionParserConfigTargetVersion) {
		return nil
	}
	found := false
	for _, target := range []struct {
		table string
		model any
	}{{"knowledgebase", &entity.Knowledgebase{}}, {"document", &entity.Document{}}} {
		if !db.WithContext(ctx).Migrator().HasTable(target.model) {
			continue
		}
		found = true
		count, err := migrateIngestionConfigRows(ctx, db, target.model, target.table)
		if err != nil {
			return fmt.Errorf("migrate %s ingestion parser configs: %w", target.table, err)
		}
		common.Info("Migrated Python ingestion parser configs",
			zap.String("table", target.table), zap.Int("rows", count))
	}
	if !found {
		return nil
	}
	return setDatabaseMigrationVersion(ctx, db, ingestionParserConfigTargetVersion)
}

func migrateIngestionConfigRows(ctx context.Context, db *gorm.DB, model any, table string) (int, error) {
	count := 0
	var rows []ingestionConfigMigrationRow
	result := db.WithContext(ctx).Model(model).
		Select("id", "parser_id", "pipeline_id", "parser_config").Order("id").
		FindInBatches(&rows, parserConfigMigrationBatchSize, func(batch *gorm.DB, _ int) error {
			return batch.Transaction(func(tx *gorm.DB) error {
				for _, row := range rows {
					config, ok := parseParserConfig(row.ParserConfig)
					if !ok {
						// An empty or absent payload carries nothing to migrate;
						// a malformed one is rejected so the batch rolls back
						// with the offending row named.
						if row.ParserConfig == nil || strings.TrimSpace(*row.ParserConfig) == "" {
							continue
						}
						return fmt.Errorf("row %s: invalid parser_config JSON", row.ID)
					}
					changed, err := migrateIngestionConfig(config, row.ParserID, row.PipelineID, table, row.ID)
					if err != nil {
						return fmt.Errorf("row %s: %w", row.ID, err)
					}
					if !changed {
						continue
					}
					if err := tx.Model(model).Where("id = ?", row.ID).
						Update("parser_config", entity.JSONMap(config)).Error; err != nil {
						return err
					}
					count++
				}
				return nil
			})
		})
	return count, result.Error
}

// migrateIngestionConfig rewrites one row's Python-era flat Parser / Chunker /
// Tokenizer settings into the components of the pipeline that row runs. Params
// the row already carries win; a setting with no Go consumer is deleted and
// logged.
func migrateIngestionConfig(config map[string]any, parserID string, pipelineID *string, table, rowID string) (bool, error) {
	// A custom canvas reads component-scoped params on the Python side too, and
	// a canvas can hold several nodes of the same kind, so no ownership rule for
	// a flat setting is safe: leave the row alone (the same call the extractor
	// migration makes).
	if pipelineID != nil && strings.TrimSpace(*pipelineID) != "" {
		return false, nil
	}
	target, ok := ingestionTargets[strings.ToLower(strings.TrimSpace(parserID))]
	if !ok {
		// resume / tag / knowledge_graph and unknown parser ids have no Go
		// template, so their settings are preserved instead of being dropped.
		return false, nil
	}
	owned := false
	for _, key := range ingestionOwnedFlatKeys() {
		if _, ok := config[key]; ok {
			owned = true
			break
		}
	}
	if !owned {
		return false, nil
	}

	row := ingestionConfigRow{table: table, rowID: rowID}
	writes := nodeWrites{}
	if err := convertIngestionParser(config, target, writes, row); err != nil {
		return false, err
	}
	if err := convertIngestionChunker(config, target, writes, row); err != nil {
		return false, err
	}
	if err := convertIngestionTokenizer(config, target, writes, row); err != nil {
		return false, err
	}
	changed, err := applyNodeWrites(config, writes)
	if err != nil {
		return false, err
	}
	for _, key := range ingestionRetiredFlatKeys {
		if _, exists := config[key]; exists {
			row.retire(key, "this Python setting has no Go consumer")
		}
	}
	for _, key := range ingestionOwnedFlatKeys() {
		if _, exists := config[key]; exists {
			delete(config, key)
			changed = true
		}
	}
	return changed, nil
}

// ingestionConfigRow carries the log context of the row being converted.
type ingestionConfigRow struct {
	table string
	rowID string
}

// retire records that a Python setting with an actual value has no Go target.
func (r ingestionConfigRow) retire(field, reason string) {
	common.Warn("Ingestion parser config: dropping setting with no Go consumer",
		zap.String("table", r.table),
		zap.String("row_id", r.rowID),
		zap.String("field", field),
		zap.String("reason", reason))
}

// nodeWrites accumulates the component-scoped params the converters derive.
type nodeWrites map[string]map[string]any

func (w nodeWrites) params(node string) map[string]any {
	if w[node] == nil {
		w[node] = map[string]any{}
	}
	return w[node]
}

func (w nodeWrites) set(node, param string, value any) {
	w.params(node)[param] = value
}

// setSetup writes a param inside a Parser file-family setup, the shape the
// Parser component reads (schema.ParserSetup). An existing family map is merged
// into, never replaced.
func (w nodeWrites) setSetup(node, family, param string, value any) {
	params := w.params(node)
	setup, _ := params[family].(map[string]any)
	if setup == nil {
		setup = map[string]any{}
		params[family] = setup
	}
	setup[param] = value
}

// applyNodeWrites merges the derived params into the stored config. A param the
// row already carries always wins - including an explicit false/0/empty value -
// and a node or family is created only when it has something to receive.
func applyNodeWrites(config map[string]any, writes nodeWrites) (bool, error) {
	nodes := make([]string, 0, len(writes))
	for node := range writes {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	changed := false
	for _, node := range nodes {
		derived := writes[node]
		if len(derived) == 0 {
			continue
		}
		raw, exists := config[node]
		if !exists {
			params := make(map[string]any, len(derived))
			for key, value := range derived {
				params[key] = value
			}
			config[node] = params
			changed = true
			continue
		}
		params, ok := raw.(map[string]any)
		if !ok {
			return false, fmt.Errorf("%s is not an object", node)
		}
		for key, value := range derived {
			existing, present := params[key]
			if !present {
				params[key] = value
				changed = true
				continue
			}
			// A derived map is a Parser file family or the parent_child group;
			// merging it one level deep lets the migration add the param it
			// derived to a family the row already carries without replacing the
			// params that family already has. A sub-key the row carries wins,
			// like every other existing value.
			derivedMap, derivedIsMap := value.(map[string]any)
			existingMap, existingIsMap := existing.(map[string]any)
			if !derivedIsMap || !existingIsMap {
				continue
			}
			for subKey, subValue := range derivedMap {
				if _, exists := existingMap[subKey]; exists {
					continue
				}
				existingMap[subKey] = subValue
				changed = true
			}
		}
	}
	return changed, nil
}

// convertIngestionParser moves the flat PDF-parser settings onto the row's
// Parser node. An unsupported engine stops migration before the source is lost.
func convertIngestionParser(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) error {
	engine := ""
	if raw, ok := config["layout_recognize"]; ok {
		derived, err := convertIngestionLayoutRecognize(raw, config, target, writes, row)
		if err != nil {
			return err
		}
		engine = derived
	}
	for _, key := range []string{"mineru_parse_method", "mineru_lang"} {
		if raw, exists := config[key]; exists && raw != nil {
			value, ok := raw.(string)
			if !ok {
				return fmt.Errorf("%s must be a string, got %T", key, raw)
			}
			if target.hasFamily("pdf") {
				writes.setSetup(target.parserNode, "pdf", key, value)
			} else {
				row.retire(key, "the targeted pipeline has no PDF file family")
			}
		}
	}
	if err := convertIngestionParserPages(config, target, writes, row); err != nil {
		return err
	}
	if err := convertIngestionTCADPOptions(config, target, writes, row, engine); err != nil {
		return err
	}
	if err := convertIngestionTableColumns(config, target, writes, row); err != nil {
		return err
	}
	seedCarriedParserFamilyDefaults(target, writes)
	return nil
}

// seedCarriedParserFamilyDefaults re-states the template's behavioural family
// params on every family the migration is about to write. Without it a run-level
// override would drop them: the override replaces the whole family map
// (internal/agent/canvas applyOverrideParams wins on the top-level key) and
// component.NewParserComponent then re-derives only its own defaults. A value the
// row already carries still wins, because applyNodeWrites skips present keys.
func seedCarriedParserFamilyDefaults(target builtinIngestionTarget, writes nodeWrites) {
	for family, carried := range target.parserFamilyCarried {
		derived, ok := writes[target.parserNode][family].(map[string]any)
		if !ok {
			// This family is not being written, so the row keeps the template's
			// own map untouched.
			continue
		}
		for key, value := range carried {
			if _, exists := derived[key]; !exists {
				derived[key] = value
			}
		}
	}
}

// convertIngestionLayoutRecognize resolves the flat engine setting. It returns
// the canonical token it selected for the PDF family ("" when none), which the
// TCADP option keys key off.
func convertIngestionLayoutRecognize(raw any, config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) (string, error) {
	// An existing selector owns the engine choice as a whole: filling its
	// sibling could change which dispatcher wins, even if the old value stays.
	if node, ok := config[target.parserNode].(map[string]any); ok {
		if setup, ok := node["pdf"].(map[string]any); ok {
			_, hasMethod := setup["parse_method"]
			_, hasModel := setup["layout_recognizer"]
			if hasMethod || hasModel {
				// A PDF selector does not own the spreadsheet family's engine.
				if value, ok := raw.(string); ok && target.hasFamily("spreadsheet") {
					if method, known := parserpkg.CanonicalPDFParseMethod(value); known && method == "tcadp" {
						writes.setSetup(target.parserNode, "spreadsheet", "parse_method", method)
						return method, nil
					}
				}
				return "", nil
			}
		}
	}
	switch value := raw.(type) {
	case nil:
		// A null engine means "unset": nothing to derive, nothing lost.
		return "", nil
	case bool:
		// The dataset UI stored the DeepDOC toggle as a boolean.
		method := "plain_text"
		if value {
			method = "deepdoc"
		}
		if !target.hasFamily("pdf") {
			row.retire("layout_recognize", "the targeted pipeline has no PDF file family")
			return "", nil
		}
		writes.setSetup(target.parserNode, "pdf", "parse_method", method)
		return method, nil
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			// Python's ParserConfig declares layout_recognize str with
			// min_length 1, so an empty string is malformed rather than unset.
			return "", fmt.Errorf("layout_recognize must not be empty")
		}
		if ingestionLayoutModelReference(trimmed) {
			if !target.hasFamily("pdf") {
				row.retire("layout_recognize", "the targeted pipeline has no PDF file family")
				return "", nil
			}
			// These OCR selectors cannot be resolved by the Go ingestion dispatcher.
			// Preserve them by stopping rather than sending them to generic VLM chat.
			switch ingestionModelReferenceEngine(trimmed) {
			case "mistral ocr", "somark", "opendataloader", "paddleocr":
				return "", fmt.Errorf("layout_recognize %q has no supported Go model-reference dispatch", trimmed)
			}
			// Keep supported OCR and VLM model references whole.
			writes.setSetup(target.parserNode, "pdf", "layout_recognizer", trimmed)
			return "", nil
		}
		method, known := parserpkg.CanonicalPDFParseMethod(trimmed)
		if !known {
			return "", fmt.Errorf("layout_recognize %q is not a supported Go parse method", trimmed)
		}
		if target.hasFamily("pdf") {
			writes.setSetup(target.parserNode, "pdf", "parse_method", method)
		}
		if method == "tcadp" && target.hasFamily("spreadsheet") {
			// Python applied one layout_recognizer to every file type, and
			// Go's TCADP engine serves the spreadsheet family too.
			writes.setSetup(target.parserNode, "spreadsheet", "parse_method", "tcadp")
		}
		if !target.hasFamily("pdf") && !(method == "tcadp" && target.hasFamily("spreadsheet")) {
			row.retire("layout_recognize", "the targeted pipeline has no file family this engine serves")
		}
		return method, nil
	default:
		return "", fmt.Errorf("layout_recognize must be a string or a boolean, got %T", raw)
	}
}

// convertIngestionParserPages moves the flat page range onto the PDF family. An
// unparseable range fails the batch, exactly as the same invalid range would
// fail a write through the API, rather than being silently dropped.
func convertIngestionParserPages(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) error {
	raw, ok := config["pages"]
	if !ok {
		return nil
	}
	if !target.hasFamily("pdf") {
		row.retire("pages", "the targeted pipeline has no PDF file family")
		return nil
	}
	pages, err := utility.NormalizePDFPages(raw)
	if err != nil {
		return fmt.Errorf("pages: %w", err)
	}
	if len(pages) == 0 {
		// "no value" and an empty range list both mean "parse every page",
		// which is what the template already does.
		return nil
	}
	out := make([]any, 0, len(pages))
	for _, span := range pages {
		out = append(out, []any{span[0], span[1]})
	}
	writes.setSetup(target.parserNode, "pdf", "pages", out)
	return nil
}

// convertIngestionTCADPOptions moves the TCADP response options onto every
// family whose effective engine is TCADP. Both keys are read from the family
// setup a parser configures itself with
// (internal/parser/parser/{csv,xls,xlsx,ppt,pptx}_parser.go and
// pdf_parser_common.go), so they follow the engine rather than the file type.
func convertIngestionTCADPOptions(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow, derivedEngine string) error {
	keys := []string{"table_result_type", "markdown_image_response_type"}
	present := make(map[string]string, len(keys))
	for _, key := range keys {
		raw, ok := config[key]
		if !ok || raw == nil {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s must be a string, got %T", key, raw)
		}
		if text != "" {
			present[key] = text
		}
	}
	if len(present) == 0 {
		return nil
	}
	families := make([]string, 0, 2)
	for _, family := range []string{"pdf", "spreadsheet"} {
		if !target.hasFamily(family) {
			continue
		}
		if ingestionEffectiveFamilyEngine(config, writes, target.parserNode, family, derivedEngine) == "tcadp" {
			families = append(families, family)
		}
	}
	if len(families) == 0 {
		for key := range present {
			row.retire(key, "no file family of the targeted pipeline parses with TCADP")
		}
		return nil
	}
	for key, text := range present {
		for _, family := range families {
			writes.setSetup(target.parserNode, family, key, text)
		}
	}
	return nil
}

// ingestionEffectiveFamilyEngine resolves the engine a family will actually run
// with, mirroring the runtime precedence: an explicit param on the row wins over
// the value this migration derives.
func ingestionEffectiveFamilyEngine(config map[string]any, writes nodeWrites, parserNode, family, derivedEngine string) string {
	if node, ok := config[parserNode].(map[string]any); ok {
		if setup, ok := node[family].(map[string]any); ok {
			if existing, ok := setup["parse_method"].(string); ok && existing != "" {
				if canonical, known := parserpkg.CanonicalPDFParseMethod(existing); known {
					return canonical
				}
				return strings.ToLower(existing)
			}
			if model, ok := setup["layout_recognizer"].(string); ok && model != "" {
				return strings.ToLower(model)
			}
		}
	}
	if setup, ok := writes[parserNode][family].(map[string]any); ok {
		if derived, ok := setup["parse_method"].(string); ok && derived != "" {
			return strings.ToLower(derived)
		}
	}
	return strings.ToLower(derivedEngine)
}

// convertIngestionTableColumns moves the table column role settings onto the
// spreadsheet family. The root-level keys must go: ingestion reads
// parser_config's root-level table_column_* first
// (internal/ingestion/task/indexdoc/process.go:resolveTableColumnConfig) and
// would otherwise shadow anything written to the node.
func convertIngestionTableColumns(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) error {
	modeKey, rolesKey, namesKey := "table_column_mode", "table_column_roles", "table_column_names"
	_, hasMode := config[modeKey]
	_, hasRoles := config[rolesKey]
	_, hasNames := config[namesKey]
	if !hasMode && !hasRoles && !hasNames {
		return nil
	}
	if !target.hasFamily("spreadsheet") {
		row.retire(modeKey, "the targeted pipeline has no spreadsheet file family")
		return nil
	}
	if raw, ok := config[modeKey]; ok && raw != nil {
		mode, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s must be a string, got %T", modeKey, raw)
		}
		if mode != "" {
			// Python types this as Literal["auto", "manual"] | None; anything
			// else was rejected by the API, so a stored other value is garbage.
			if mode != "auto" && mode != "manual" {
				row.retire(modeKey, fmt.Sprintf("%q is not a mode the table parser understands", mode))
			} else {
				writes.setSetup(target.parserNode, "spreadsheet", "column_mode", mode)
			}
		}
	}
	if raw, ok := config[rolesKey]; ok && raw != nil {
		roles, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object, got %T", rolesKey, raw)
		}
		normalized := make(map[string]any, len(roles))
		for column, role := range roles {
			text, _ := role.(string)
			switch text {
			case "vectorize":
				// Python normalizes the retired role name at validation time
				// (api/utils/validation_utils.py:legacy_vectorize_table_column_role).
				normalized[column] = "indexing"
			case "indexing", "metadata", "both":
				normalized[column] = text
			default:
				row.retire(rolesKey, fmt.Sprintf("column %q has unsupported role %q", column, text))
			}
		}
		if len(normalized) > 0 {
			writes.setSetup(target.parserNode, "spreadsheet", "column_roles", normalized)
		}
	}
	if raw, ok := config[namesKey]; ok && raw != nil {
		names, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("%s must be a list, got %T", namesKey, raw)
		}
		out := make([]any, 0, len(names))
		for _, name := range names {
			text, ok := name.(string)
			if !ok || text == "" {
				row.retire(namesKey, fmt.Sprintf("dropping non-string column name %v", name))
				continue
			}
			out = append(out, text)
		}
		if len(out) > 0 {
			writes.setSetup(target.parserNode, "spreadsheet", "column_names", out)
		}
	}
	return nil
}

// ingestionLayoutModelReference reports whether a layout_recognize value names a
// VLM model rather than a parse engine: Python's "<llm>@<instance>@<provider>"
// composite (normalize_layout_recognizer) or a bare tenant model id.
func ingestionLayoutModelReference(raw string) bool {
	if strings.Contains(raw, "@") {
		return true
	}
	compact := strings.ReplaceAll(raw, "-", "")
	if len(compact) != 32 {
		return false
	}
	for _, r := range compact {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// ingestionModelReferenceEngine returns the engine segment of a composite model
// reference ("name@instance@provider" -> "provider"), or "" for a bare id.
func ingestionModelReferenceEngine(raw string) string {
	idx := strings.LastIndex(raw, "@")
	if idx < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(raw[idx+1:]))
}

// convertIngestionChunker moves the flat chunking settings onto the row's
// chunker node. Which settings have a home depends on the chunker: only
// GeneralChunker and TokenChunker read a token size, delimiter list, overlap and
// context sizes.
func convertIngestionChunker(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) error {
	convertIngestionChunkTokenNum(config, target, writes, row)
	if err := convertIngestionDelimiter(config, target, writes, row); err != nil {
		return err
	}
	convertIngestionOverlappedPercent(config, target, writes, row)
	convertIngestionContextSizes(config, target, writes, row)
	return convertIngestionParentChild(config, target, writes, row)
}

// convertIngestionChunkTokenNum maps the flat token count onto the target
// chunker's own param name. The meaning is not uniform across chunkers, so each
// case states its own mapping or refuses.
func convertIngestionChunkTokenNum(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) {
	raw, ok := config["chunk_token_num"]
	if !ok || raw == nil {
		// Absent or null means "the default", which the template already
		// supplies: nothing is lost by deleting the flat key.
		return
	}
	size, valid := common.ToInt(raw)
	if !valid {
		row.retire("chunk_token_num", fmt.Sprintf("%v is not an integer", raw))
		return
	}
	switch target.chunker {
	case "GeneralChunker":
		// 0 is legal here and means "no split window", the same thing Python
		// set for spreadsheet and TCADP files at parse time.
		if size < 0 {
			row.retire("chunk_token_num", fmt.Sprintf("%d is negative", size))
			return
		}
		writes.set(target.chunkerNode, "chunk_token_size", size)
	case "TokenChunker":
		// schema/chunker.go rejects a non-positive chunk_token_size.
		if size <= 0 {
			row.retire("chunk_token_num", fmt.Sprintf("%d is not a positive chunk size", size))
			return
		}
		writes.set(target.chunkerNode, "chunk_token_size", size)
	case "TitleChunker":
		// Known semantic difference: Python used chunk_token_num as the merge
		// target of rag/app/{book,laws,paper}.py while Go's chunk_token_cap is a
		// hard ceiling on an emitted chunk. 0 disables the ceiling in Go, which
		// is what Python's "no cap" guard did, so it carries over; a non-zero
		// value must be inside the range the chunker accepts or the component
		// would refuse to build.
		if size != 0 && (size < 128 || size > 8000) {
			row.retire("chunk_token_num", fmt.Sprintf("%d is outside the title chunker's accepted 128..8000 range", size))
			return
		}
		writes.set(target.chunkerNode, "chunk_token_cap", size)
	default:
		row.retire("chunk_token_num", fmt.Sprintf("%s has no token size parameter", target.chunker))
	}
}

// convertIngestionDelimiter maps the flat delimiter string onto the canonical
// delimiter list, using the same parser the write path uses
// (pipeline.normalizeGeneralComponentParams), so a migrated row and an
// API-written row carry byte-identical lists.
func convertIngestionDelimiter(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) error {
	raw, ok := config["delimiter"]
	if !ok || raw == nil {
		return nil
	}
	if !target.acceptsChunkParams() {
		row.retire("delimiter", fmt.Sprintf("%s splits on its own structure", target.chunker))
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("delimiter must be a string, got %T", raw)
	}
	if text == "" {
		// Python declares min_length 1; an empty value has no split points.
		return nil
	}
	delimiters := parserchunk.ParseDelimiterField(text)
	if len(delimiters) == 0 {
		return nil
	}
	out := make([]any, 0, len(delimiters))
	for _, delimiter := range delimiters {
		out = append(out, delimiter)
	}
	writes.set(target.chunkerNode, "delimiters", out)
	return nil
}

// convertIngestionOverlappedPercent copies the overlap across unchanged. The
// runtime normalizes it (schema.NormalizeOverlappedPercent turns a 0..1 fraction
// into a whole percentage and clamps it), which is where the API leaves the value
// too, so the stored number stays the user's own.
func convertIngestionOverlappedPercent(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) {
	raw, ok := config["overlapped_percent"]
	if !ok || raw == nil {
		return
	}
	if !target.acceptsChunkParams() {
		row.retire("overlapped_percent", fmt.Sprintf("%s has no overlap parameter", target.chunker))
		return
	}
	if _, ok := common.ToFloat(raw); !ok {
		if text, isText := raw.(string); !isText {
			row.retire("overlapped_percent", fmt.Sprintf("%v is not a number", raw))
			return
		} else if _, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err != nil {
			row.retire("overlapped_percent", fmt.Sprintf("%q is not a number", text))
			return
		}
	}
	writes.set(target.chunkerNode, "overlapped_percent", raw)
}

// convertIngestionContextSizes moves the table and image context windows. Only
// the general and token chunkers grew them in Go; the title, manual and one
// chunkers Python used them with have no such parameter.
func convertIngestionContextSizes(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) {
	for _, key := range []string{"table_context_size", "image_context_size"} {
		raw, ok := config[key]
		if !ok || raw == nil {
			continue
		}
		if !target.acceptsChunkParams() {
			row.retire(key, fmt.Sprintf("%s has no context size parameter", target.chunker))
			continue
		}
		size, valid := common.ToInt(raw)
		if !valid || size < 0 {
			row.retire(key, fmt.Sprintf("%v is not a non-negative integer", raw))
			continue
		}
		writes.set(target.chunkerNode, key, size)
	}
}

// convertIngestionParentChild folds the three shapes the parent/child setting was
// stored in into the component-scoped group the chunker reads, and derives the
// children_delimiters array from it. Both halves are needed: the group is what
// the write path keeps, children_delimiters is what the chunker reads at run time
// (pipeline.ApplyParentChildChunkerConfig derives it while writing, which never
// runs for a row this migration rewrites).
func convertIngestionParentChild(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) error {
	groupRaw, hasGroup := config["parent_child"]
	enabledRaw, hasEnabled := config["enable_children"]
	delimiterRaw, hasDelimiter := config["children_delimiter"]
	if !hasGroup && !hasEnabled && !hasDelimiter {
		return nil
	}
	if !target.acceptsChunkParams() {
		row.retire("parent_child", fmt.Sprintf("%s cannot split parent chunks into children", target.chunker))
		return nil
	}

	useParentChild := false
	useFromGroup := false
	childrenDelimiter := ""
	delimiterFromGroup := false
	if hasGroup && groupRaw != nil {
		group, ok := groupRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("parent_child must be an object, got %T", groupRaw)
		}
		if raw, exists := group["use_parent_child"]; exists && raw != nil {
			enabled, ok := raw.(bool)
			if !ok {
				return fmt.Errorf("parent_child.use_parent_child must be a boolean, got %T", raw)
			}
			useParentChild, useFromGroup = enabled, true
		}
		if text, ok := group["children_delimiter"].(string); ok {
			childrenDelimiter, delimiterFromGroup = text, true
		}
	}
	// The flat keys are the Python-era shape and rank below the group: on a row
	// that carries both, the current shape would otherwise be overruled by a
	// stale sibling.
	if hasEnabled && enabledRaw != nil && !useFromGroup {
		enabled, ok := enabledRaw.(bool)
		if !ok {
			return fmt.Errorf("enable_children must be a boolean, got %T", enabledRaw)
		}
		useParentChild = enabled
	}
	if hasDelimiter && delimiterRaw != nil && !delimiterFromGroup {
		text, ok := delimiterRaw.(string)
		if !ok {
			return fmt.Errorf("children_delimiter must be a string, got %T", delimiterRaw)
		}
		childrenDelimiter = text
	}
	if !delimiterFromGroup && (!hasDelimiter || delimiterRaw == nil) {
		childrenDelimiter = "\n"
	}
	if !useFromGroup && (!hasEnabled || enabledRaw == nil) && hasDelimiter {
		useParentChild = childrenDelimiter != ""
	}
	childrenDelimiter = decodePythonChildDelimiter(childrenDelimiter)
	group := map[string]any{"use_parent_child": useParentChild, "children_delimiter": childrenDelimiter}
	if node, ok := config[target.chunkerNode].(map[string]any); ok {
		if raw, present := node["parent_child"]; present {
			existing, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.parent_child is not an object", target.chunkerNode)
			}
			for key, value := range existing {
				group[key] = value
			}
		}
	}
	useParentChild, _ = group["use_parent_child"].(bool)
	childrenDelimiter, _ = group["children_delimiter"].(string)
	writes.set(target.chunkerNode, "parent_child", group)
	children := []any{}
	if useParentChild {
		for _, delimiter := range parserchunk.ParseDelimiterField(childrenDelimiter) {
			children = append(children, delimiter)
		}
	}
	writes.set(target.chunkerNode, "children_delimiters", children)
	return nil
}

// Python decodes escaped control characters before splitting child chunks.
// Unknown escapes retain their backslash, as unicode_escape does.
func decodePythonChildDelimiter(text string) string {
	var out strings.Builder
	for len(text) > 0 {
		if text[0] == '\\' {
			char, _, tail, err := strconv.UnquoteChar(text, '"')
			if err == nil {
				out.WriteRune(char)
				text = tail
				continue
			}
		}
		out.WriteByte(text[0])
		text = text[1:]
	}
	return out.String()
}

// convertIngestionTokenizer moves the filename weight onto the row's tokenizer.
// A null weight is left alone: Go's default (0.1) is Python's default too, and
// the flat key is deleted either way.
func convertIngestionTokenizer(config map[string]any, target builtinIngestionTarget, writes nodeWrites, row ingestionConfigRow) error {
	raw, ok := config["filename_embd_weight"]
	if !ok {
		return nil
	}
	// search_method and fields belong to the flow Tokenizer component in Python
	// too; the flat config never carried them, so they are neither migrated nor
	// deleted here (the extractor migration leaves topn_tags/tag_kb_ids alone for
	// the same reason).
	if raw == nil {
		return nil
	}
	weight, ok := common.ToFloat(raw)
	if !ok {
		if text, isText := raw.(string); !isText {
			return fmt.Errorf("filename_embd_weight must be a number, got %T", raw)
		} else if _, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err != nil {
			return fmt.Errorf("filename_embd_weight %q is not a number", text)
		}
	}
	if _, isText := raw.(string); isText {
		// A numeric string reached the field; store the number the runtime will
		// actually use rather than the string it happens to parse.
		writes.set(target.tokenizerNode, "filename_embd_weight", weight)
		return nil
	}
	writes.set(target.tokenizerNode, "filename_embd_weight", raw)
	return nil
}

// This step follows rc2.dev1 because database migrations share one version marker.
const extractorParserConfigTargetVersion = "v1.0.0-rc2.dev2"
const builtinExtractorNodeID = "Extractor:AutoExtractDefault"

var builtinExtractorPipelines = map[string]struct{}{
	"general":      {},
	"naive":        {},
	"audio":        {},
	"book":         {},
	"email":        {},
	"laws":         {},
	"manual":       {},
	"one":          {},
	"paper":        {},
	"picture":      {},
	"presentation": {},
}
var extractorFlatKeys = []string{"auto_keywords", "auto_questions", "enable_metadata", "metadata", "built_in_metadata"}

type extractorConfigMigrationRow struct {
	ID           string  `gorm:"column:id"`
	ParserID     string  `gorm:"column:parser_id"`
	PipelineID   *string `gorm:"column:pipeline_id"`
	ParserConfig *string `gorm:"column:parser_config"`
}

func migrateExtractorParserConfigs(ctx context.Context, db *gorm.DB) error {
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return err
	}
	if shouldSkipMigration(version, extractorParserConfigTargetVersion) {
		return nil
	}
	found := false
	for _, target := range []struct {
		table string
		model any
	}{{"knowledgebase", &entity.Knowledgebase{}}, {"document", &entity.Document{}}} {
		if !db.WithContext(ctx).Migrator().HasTable(target.model) {
			continue
		}
		found = true
		count, err := migrateExtractorConfigRows(ctx, db, target.model)
		if err != nil {
			return fmt.Errorf("migrate %s extractor configs: %w", target.table, err)
		}
		common.Info("Migrated Python extractor configs", zap.String("table", target.table), zap.Int("rows", count))
	}
	if !found {
		return nil
	}
	return setDatabaseMigrationVersion(ctx, db, extractorParserConfigTargetVersion)
}

func migrateExtractorConfigRows(ctx context.Context, db *gorm.DB, model any) (int, error) {
	count := 0
	var rows []extractorConfigMigrationRow
	result := db.WithContext(ctx).Model(model).Select("id", "parser_id", "pipeline_id", "parser_config").Order("id").FindInBatches(&rows, parserConfigMigrationBatchSize, func(batch *gorm.DB, _ int) error {
		return batch.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				if row.ParserConfig == nil || strings.TrimSpace(*row.ParserConfig) == "" {
					continue
				}
				config, ok := parseParserConfig(row.ParserConfig)
				if !ok {
					return fmt.Errorf("row %s: invalid parser_config JSON", row.ID)
				}
				changed, err := migrateExtractorConfig(config, row.ParserID, row.PipelineID)
				if err != nil {
					return fmt.Errorf("row %s: %w", row.ID, err)
				}
				if !changed {
					continue
				}
				if err := tx.Model(model).Where("id = ?", row.ID).Update("parser_config", entity.JSONMap(config)).Error; err != nil {
					return err
				}
				count++
			}
			return nil
		})
	})
	return count, result.Error
}

// migrateExtractorConfig rewrites only Python's top-level extraction settings.
// Existing node fields win over flat Python values. A modular metadata transport
// object replaces node metadata, matching ApplyComponentScopedParserConfig.
// Unsupported inputs return an error before the config is mutated.
func migrateExtractorConfig(config map[string]any, parserID string, pipelineID *string) (bool, error) {
	present := false
	for _, key := range extractorFlatKeys {
		if _, ok := config[key]; ok {
			present = true
			break
		}
	}
	if !present {
		return false, nil
	}
	var targets []map[string]any
	var newNode map[string]any
	if pipelineID == nil || strings.TrimSpace(*pipelineID) == "" {
		_, builtin := builtinExtractorPipelines[strings.ToLower(strings.TrimSpace(parserID))]
		if !builtin {
			return false, nil
		}
		if raw, exists := config[builtinExtractorNodeID]; exists {
			node, ok := raw.(map[string]any)
			if !ok {
				return false, fmt.Errorf("%s is not an object", builtinExtractorNodeID)
			}
			targets = append(targets, node)
		} else {
			newNode = map[string]any{}
			targets = append(targets, newNode)
		}
	} else {
		for id, raw := range config {
			id = strings.ToLower(id)
			if strings.HasPrefix(id, "extractor:") || strings.HasPrefix(id, "extractor_") {
				if node, ok := raw.(map[string]any); ok {
					targets = append(targets, node)
				}
			}
		}
		if len(targets) == 0 {
			return false, nil
		}
	}
	groups, replaceMetadata, err := convertPythonExtractorGroups(config)
	if err != nil {
		return false, err
	}
	for _, node := range targets {
		for name := range groups {
			if raw, exists := node[name]; exists && !(name == "metadata" && replaceMetadata) {
				if _, ok := raw.(map[string]any); !ok {
					return false, fmt.Errorf("%s group is not an object", name)
				}
			}
		}
	}
	for _, node := range targets {
		for name, defaults := range groups {
			group, ok := node[name].(map[string]any)
			if !ok || (name == "metadata" && replaceMetadata) {
				node[name] = maps.Clone(defaults)
				continue
			}
			for key, value := range defaults {
				if _, exists := group[key]; !exists {
					group[key] = value
				}
			}
		}
	}
	if newNode != nil {
		config[builtinExtractorNodeID] = newNode
	}
	for _, key := range extractorFlatKeys {
		delete(config, key)
	}
	return true, nil
}

func convertPythonExtractorGroups(config map[string]any) (map[string]map[string]any, bool, error) {
	groups := map[string]map[string]any{}
	for _, field := range []struct{ key, group string }{{"auto_keywords", "keywords"}, {"auto_questions", "questions"}} {
		if raw, exists := config[field.key]; exists {
			value, ok := raw.(float64)
			if raw == nil {
				value, ok = 0, true
			}
			if !ok || value < 0 || value != float64(int(value)) {
				return nil, false, fmt.Errorf("%s must be a non-negative integer", field.key)
			}
			groups[field.group] = map[string]any{"top_n": int(value)}
		}
	}
	source := config
	replaceMetadata := false
	_, hasToggle := config["enable_metadata"]
	_, hasFields := config["metadata"]
	_, hasBuiltIn := config["built_in_metadata"]
	if obj, ok := config["metadata"].(map[string]any); ok {
		if len(obj) == 0 && !hasToggle && !hasBuiltIn {
			source, replaceMetadata = obj, true
		}
		for _, key := range []string{"enabled", "metadata", "built_in_metadata"} {
			if _, exists := obj[key]; exists {
				source = obj
				replaceMetadata = true
				break
			}
		}
	}
	if hasToggle || hasFields || hasBuiltIn {
		flag := source["enable_metadata"]
		if replaceMetadata {
			flag = source["enabled"]
		}
		if flag != nil {
			switch value := flag.(type) {
			case bool:
			case float64:
				if value != 0 && value != 1 {
					return nil, false, fmt.Errorf("metadata enabled must be bool or 0/1")
				}
			default:
				return nil, false, fmt.Errorf("metadata enabled must be bool or 0/1")
			}
		}
		fields, err := convertExtractorMetadataFields(source["metadata"])
		if err != nil {
			return nil, false, fmt.Errorf("metadata: %w", err)
		}
		builtIn, err := convertExtractorMetadataFields(source["built_in_metadata"])
		if err != nil {
			return nil, false, fmt.Errorf("built_in_metadata: %w", err)
		}
		groups["metadata"] = map[string]any{"enabled": common.ParserConfigBool(flag), "metadata": fields, "built_in_metadata": builtIn}
	}
	return groups, replaceMetadata, nil
}

// convertExtractorMetadataFields accepts field lists and flat JSON Schemas that
// fit MetadataFieldDef. Constraints the target cannot express are rejected.
func convertExtractorMetadataFields(raw any) ([]common.MetadataFieldDef, error) {
	if raw == nil {
		return []common.MetadataFieldDef{}, nil
	}
	if schema, ok := raw.(map[string]any); ok {
		if len(schema) == 0 {
			return []common.MetadataFieldDef{}, nil
		}
		for key := range schema {
			if key != "type" && key != "properties" && key != "additionalProperties" {
				return nil, fmt.Errorf("unsupported JSON Schema keyword %s", key)
			}
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok || schema["type"] != "object" || schema["additionalProperties"] != false {
			return nil, fmt.Errorf("expected flat object schema with additionalProperties=false")
		}
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		fields := make([]any, 0, len(names))
		for _, name := range names {
			prop, ok := properties[name].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("property %s is not an object", name)
			}
			for key := range prop {
				if key != "type" && key != "description" && key != "enum" {
					return nil, fmt.Errorf("unsupported property constraint %s.%s", name, key)
				}
			}
			field := maps.Clone(prop)
			field["key"] = name
			fields = append(fields, field)
		}
		raw = fields
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected field list")
	}
	normalized := make([]any, 0, len(list))
	for _, rawField := range list {
		field, ok := rawField.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field is not an object")
		}
		key, ok := field["key"].(string)
		if !ok || key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("field requires a non-empty key without surrounding whitespace")
		}
		for _, name := range []string{"type", "description", "descriptions"} {
			if value := field[name]; value != nil {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("%s.%s must be a string", key, name)
				}
			}
		}
		if value := field["enum"]; value != nil {
			values, ok := value.([]any)
			if !ok {
				return nil, fmt.Errorf("%s.enum must be a string list", key)
			}
			for _, value := range values {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("%s.enum must be a string list", key)
				}
			}
		}
		item := maps.Clone(field)
		if description, _ := item["description"].(string); description == "" {
			item["description"] = item["descriptions"]
		}
		normalized = append(normalized, item)
	}
	return common.MetadataFieldDefsFromRaw(normalized), nil
}
