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
//

package task

import (
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

// builtinComponentParamsGolden holds hardcoded golden values of
// pipeline.ComponentParamsDefaults for every built-in ingestion template.
// Each entry is the compact JSON serialization of the resolved default
// component params (the "outputs" wire key is already excluded).
//
// These are NOT derived at runtime: they pin the exact default params that
// each template's DSL currently bakes in. If a template's DSL default values
// change, the matching entry here MUST be updated in lockstep, otherwise the
// corresponding test method fails and flags the regression.
//
// Comparison is done per-component (see assertComponentsMatch), so the JSON
// serialization order inside each entry is irrelevant.
var builtinComponentParamsGolden = map[string]string{
	"audio":        "{\"File\": {}, \"Parser:SongsFillAir\": {\"audio\": {\"output_format\": \"text\", \"preprocess\": [\"main_content\"], \"suffix\": [\"aac\", \"aiff\", \"ape\", \"au\", \"da\", \"flac\", \"midi\", \"mp3\", \"ogg\", \"oggvorbis\", \"realaudio\", \"vqf\", \"wav\", \"wave\", \"wma\"]}}, \"TokenChunker:BlueSkiesLaugh\": {}, \"Tokenizer:KindEyesWatch\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"book":         "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"doc\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"doc\"]}, \"docx\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"docx\"], \"vlm\": {}}, \"html\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"htm\", \"html\"]}, \"pdf\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"remove_toc\": true, \"suffix\": [\"pdf\"], \"vlm\": {}}, \"text&code\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"txt\"]}}, \"TitleChunker:GrumpyGarlicsBake\": {\"hierarchy\": 5, \"include_heading_content\": true, \"levels\": [[\"^#[^#]\", \"^##[^#]\", \"^###[^#]\", \"^####[^#]\"], [\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\", \"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"第[零一二三四五六七八九十百0-9]+条\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"], [\"第[0-9]+章\", \"第[0-9]+节\", \"[0-9]{1,2}[\\\\. 、]\", \"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\", \"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"], [\"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"[零一二三四五六七八九十百]+[ 、]\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\", \"[\\\\(（][0-9]{,2}[\\\\)）]\"], [\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\", \"Chapter (I+V?|VI*|XI|IX|X)\", \"Section [0-9]+\", \"Article [0-9]+\"]], \"method\": \"hierarchy\"}, \"Tokenizer:HotDonutsRing\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"email":        "{\"File\": {}, \"Parser:BirdsFlutterHigh\": {\"email\": {\"fields\": [\"from\", \"to\", \"cc\", \"bcc\", \"date\", \"subject\", \"body\", \"attachments\"], \"output_format\": \"text\", \"preprocess\": [\"main_content\"], \"suffix\": [\"eml\"]}}, \"TokenChunker:WarmBreadSmells\": {\"children_delimiters\": [], \"chunk_token_size\": 512, \"delimiter_mode\": \"delimiter\", \"delimiters\": [\"\\n\", \"!\", \"?\", \"。\", \"；\", \"！\", \"？\"], \"image_context_size\": 0, \"overlapped_percent\": 0, \"table_context_size\": 0}, \"Tokenizer:NiceWordsSpoken\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"general":      "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"doc\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"doc\"]}, \"docx\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"docx\"], \"vlm\": {}}, \"html\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"htm\", \"html\"]}, \"markdown\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"md\", \"markdown\", \"mdx\"], \"vlm\": {}}, \"pdf\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"pages\": [[1, 100000]], \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"pdf\"], \"vlm\": {}}, \"spreadsheet\": {\"flatten_media_to_text\": false, \"output_format\": \"html\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"xls\", \"xlsx\", \"csv\"], \"vlm\": {}}, \"text&code\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"txt\", \"py\", \"js\", \"java\", \"c\", \"cpp\", \"h\", \"php\", \"go\", \"ts\", \"sh\", \"cs\", \"kt\", \"sql\"]}}, \"TokenChunker:SixApplesFall\": {\"children_delimiters\": [], \"chunk_token_size\": 512, \"delimiter_mode\": \"delimiter\", \"delimiters\": [\"\\n\", \"!\", \"?\", \"。\", \"；\", \"！\", \"？\"], \"image_context_size\": 0, \"overlapped_percent\": 0, \"table_context_size\": 0}, \"Tokenizer:LegalReadersDecide\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"laws":         "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"doc\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"doc\"]}, \"docx\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"docx\"], \"vlm\": {}}, \"html\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"htm\", \"html\"]}, \"markdown\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"md\", \"markdown\", \"mdx\"], \"vlm\": {}}, \"pdf\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"remove_toc\": true, \"suffix\": [\"pdf\"], \"vlm\": {}}, \"text&code\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"txt\"]}}, \"TitleChunker:SpicyKeysKick\": {\"hierarchy\": 2, \"include_heading_content\": false, \"levels\": [[\"^#[^#]\", \"^##[^#]\", \"^###[^#]\", \"^####[^#]\"], [\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\", \"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"第[零一二三四五六七八九十百0-9]+条\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"], [\"第[0-9]+章\", \"第[0-9]+节\", \"[0-9]{1,2}[\\\\. 、]\", \"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\", \"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"], [\"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"[零一二三四五六七八九十百]+[ 、]\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\", \"[\\\\(（][0-9]{,2}[\\\\)）]\"], [\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\", \"Chapter (I+V?|VI*|XI|IX|X)\", \"Section [0-9]+\", \"Article [0-9]+\"]], \"method\": \"hierarchy\"}, \"Tokenizer:PublicJobsTake\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"manual":       "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"doc\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"doc\"]}, \"docx\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"docx\"], \"vlm\": {}}, \"pdf\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"pdf\"], \"vlm\": {}}}, \"ManualChunker:NineInsectsFind\": {\"hierarchy\": 0, \"include_heading_content\": false, \"levels\": [[\"^#[^#]\", \"^##[^#]\", \"^###[^#]\", \"^####[^#]\"], [\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\", \"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"第[零一二三四五六七八九十百0-9]+条\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"], [\"第[0-9]+章\", \"第[0-9]+节\", \"[0-9]{1,2}[\\\\. 、]\", \"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\", \"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"], [\"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"[零一二三四五六七八九十百]+[ 、]\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\", \"[\\\\(（][0-9]{,2}[\\\\)）]\"], [\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\", \"Chapter (I+V?|VI*|XI|IX|X)\", \"Section [0-9]+\", \"Article [0-9]+\"]], \"method\": \"group\"}, \"Tokenizer:FunnyBalloonsGrin\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"one":          "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"doc\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"doc\"]}, \"docx\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"docx\"], \"vlm\": {}}, \"html\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"htm\", \"html\"]}, \"markdown\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"md\", \"markdown\", \"mdx\"], \"vlm\": {}}, \"pdf\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"pdf\"], \"vlm\": {}}, \"spreadsheet\": {\"flatten_media_to_text\": false, \"output_format\": \"html\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"xls\", \"xlsx\"], \"vlm\": {}}, \"text&code\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"txt\"]}}, \"OneChunker:DryDrinksVisit\": {}, \"Tokenizer:FrankWeeksListen\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"paper":        "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"pdf\": {\"enable_multi_column\": true, \"flatten_media_to_text\": false, \"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"pdf\"], \"vlm\": {}}}, \"TitleChunker:SparklySchoolsTravel\": {\"hierarchy\": 0, \"include_heading_content\": false, \"levels\": [[\"^#[^#]\", \"^##[^#]\", \"^###[^#]\", \"^####[^#]\"], [\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\", \"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"第[零一二三四五六七八九十百0-9]+条\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"], [\"第[0-9]+章\", \"第[0-9]+节\", \"[0-9]{1,2}[\\\\. 、]\", \"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\", \"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"], [\"第[零一二三四五六七八九十百0-9]+章\", \"第[零一二三四五六七八九十百0-9]+节\", \"[零一二三四五六七八九十百]+[ 、]\", \"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\", \"[\\\\(（][0-9]{,2}[\\\\)）]\"], [\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\", \"Chapter (I+V?|VI*|XI|IX|X)\", \"Section [0-9]+\", \"Article [0-9]+\"]], \"method\": \"group\"}, \"Tokenizer:GreatCarsWash\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"picture":      "{\"File\": {}, \"Parser:ViewsCaptureLight\": {\"image\": {\"output_format\": \"json\", \"parse_method\": \"ocr\", \"preprocess\": [\"main_content\"], \"suffix\": [\"bmp\", \"gif\", \"jpeg\", \"jpg\", \"png\", \"svg\", \"tif\", \"tiff\", \"webp\"]}, \"video\": {\"output_format\": \"text\", \"preprocess\": [\"main_content\"], \"suffix\": [\"3gp\", \"3gpp\", \"avi\", \"flv\", \"mkv\", \"mov\", \"mp4\", \"mpeg\", \"mpg\", \"webm\", \"wmv\"]}}, \"TokenChunker:BrightColorsGlow\": {}, \"Tokenizer:SharpLensFocus\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"presentation": "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"pdf\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"pdf\"], \"vlm\": {}}, \"slides\": {\"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"pptx\", \"ppt\"]}}, \"Tokenizer:TallTreesDance\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"PageChunker:HappyHillsGlow\": {}, \"Extractor:AutoExtractDefault\": {\"llm_id\": \"\", \"keywords\": {\"top_n\": 0}, \"questions\": {\"top_n\": 0}, \"tags\": {\"top_n\": 0, \"tag_file_id\": \"\"}, \"summary\": {\"enabled\": false}, \"metadata\": {\"enabled\": false, \"metadata\": [], \"built_in_metadata\": []}}}",
	"qa":           "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"docx\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"docx\"], \"vlm\": {}}, \"markdown\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"md\", \"markdown\", \"mdx\"], \"vlm\": {}}, \"pdf\": {\"flatten_media_to_text\": false, \"output_format\": \"json\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"pdf\"], \"vlm\": {}}, \"spreadsheet\": {\"flatten_media_to_text\": false, \"output_format\": \"html\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"xls\", \"xlsx\", \"csv\"], \"vlm\": {}}, \"text&code\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"txt\"]}}, \"Tokenizer:ColdCloudsDream\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}, \"QAChunker:TidyCloudsThink\": {}}",
	"table":        "{\"File\": {}, \"Parser:HipSignsRhyme\": {\"spreadsheet\": {\"flatten_media_to_text\": false, \"output_format\": \"html\", \"parse_method\": \"DeepDOC\", \"preprocess\": [\"main_content\"], \"suffix\": [\"xls\", \"xlsx\", \"csv\"], \"vlm\": {}, \"column_mode\": \"auto\", \"column_roles\": {}, \"column_names\": []}, \"text&code\": {\"output_format\": \"json\", \"preprocess\": [\"main_content\"], \"suffix\": [\"txt\"]}}, \"TableChunker:FastFoxesJump\": {}, \"Tokenizer:DeepLakesShine\": {\"fields\": \"text\", \"filename_embd_weight\": 0.1, \"search_method\": [\"embedding\", \"full_text\"]}}",
}

// Per-template test methods. Each resolves default component params from a
// built-in template DSL and verifies: (1) they match the hardcoded golden,
// (2) they survive a JSON round-trip through entity.JSONMap (DB storage).
func TestBuildComponentParams_Audio(t *testing.T)   { assertTemplateComponentParams(t, "audio") }
func TestBuildComponentParams_Book(t *testing.T)    { assertTemplateComponentParams(t, "book") }
func TestBuildComponentParams_Email(t *testing.T)   { assertTemplateComponentParams(t, "email") }
func TestBuildComponentParams_General(t *testing.T) { assertTemplateComponentParams(t, "general") }
func TestBuildComponentParams_Laws(t *testing.T)    { assertTemplateComponentParams(t, "laws") }
func TestBuildComponentParams_Manual(t *testing.T)  { assertTemplateComponentParams(t, "manual") }
func TestBuildComponentParams_One(t *testing.T)     { assertTemplateComponentParams(t, "one") }
func TestBuildComponentParams_Paper(t *testing.T)   { assertTemplateComponentParams(t, "paper") }
func TestBuildComponentParams_Picture(t *testing.T) { assertTemplateComponentParams(t, "picture") }
func TestBuildComponentParams_Presentation(t *testing.T) {
	assertTemplateComponentParams(t, "presentation")
}
func TestBuildComponentParams_Qa(t *testing.T)    { assertTemplateComponentParams(t, "qa") }
func TestBuildComponentParams_Table(t *testing.T) { assertTemplateComponentParams(t, "table") }

// assertTemplateComponentParams resolves the default component params for the
// given built-in template and verifies two layers per-component:
//
//  1. Layer 1 (DSL parse): pipeline.ComponentParamsDefaults returns exactly the
//     hardcoded golden.
//  2. Layer 2 (storage round-trip): the parsed result survives JSON
//     marshal→entity.JSONMap→unmarshal, simulating the GORM DB round-trip that
//     runPipelineWithDSL's parserConfig reads from Doc.ParserConfig.
func assertTemplateComponentParams(t *testing.T, ref string) {
	t.Helper()

	got, want := loadTemplateDefaults(t, ref)

	// Layer 1: parsed defaults match the golden per-component.
	assertComponentsMatch(t, "Layer1 parse", got, want)

	// Layer 2: defaults survive a JSON round-trip through entity.JSONMap
	// (the DB storage format — GORM deserializes into map[string]any, then
	// runPipelineWithDSL reads it back as map[string]interface{}).
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal %q: %v", ref, err)
	}
	var stored entity.JSONMap
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal into JSONMap %q: %v", ref, err)
	}
	var roundTripped map[string]map[string]any
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("unmarshal back %q: %v", ref, err)
	}
	assertComponentsMatch(t, "Layer2 round-trip", roundTripped, got)
}

// loadTemplateDefaults loads the builtin DSL for ref, parses the default
// component params, and returns both the parsed result and the hardcoded
// golden expectation.
func loadTemplateDefaults(t *testing.T, ref string) (got, want map[string]map[string]any) {
	t.Helper()

	golden, ok := builtinComponentParamsGolden[ref]
	if !ok {
		t.Fatalf("missing golden entry for template %q", ref)
	}
	if err := json.Unmarshal([]byte(golden), &want); err != nil {
		t.Fatalf("unmarshal golden %q: %v", ref, err)
	}

	dsl, err := pipelinepkg.LoadBuiltinDSL(ref)
	if err != nil {
		t.Fatalf("load builtin DSL %q: %v", ref, err)
	}
	got, err = pipelinepkg.ComponentParamsDefaults([]byte(dsl))
	if err != nil {
		t.Fatalf("ComponentParamsDefaults %q: %v", ref, err)
	}
	return
}

func assertComponentsMatch(t *testing.T, label string, got, want map[string]map[string]any) {
	t.Helper()
	for cpnID := range got {
		if _, ok := want[cpnID]; !ok {
			t.Errorf("[%s] unexpected component %q", label, cpnID)
		}
	}
	for cpnID, wantParams := range want {
		t.Run(cpnID, func(t *testing.T) {
			gotParams, ok := got[cpnID]
			if !ok {
				t.Errorf("[%s] missing component %q", label, cpnID)
				return
			}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Errorf("[%s] component %q params mismatch\n got=%#v\nwant=%#v", label, cpnID, gotParams, wantParams)
			}
		})
	}
}

// TestBuildComponentParams_GoldenCoversAllTemplates keeps the hardcoded golden
// table in lockstep with the built-in template registry. A renamed or added
// template must get a matching golden entry, and a removed template must drop
// its entry, otherwise this test fails. ("naive" is an alias for "general" and
// is intentionally not a separate file/template.)
func TestBuildComponentParams_GoldenCoversAllTemplates(t *testing.T) {
	reg, err := pipelinepkg.DefaultRegistry()
	if err != nil {
		t.Fatalf("builtin registry: %v", err)
	}
	refs := reg.Refs()
	if len(refs) == 0 {
		t.Fatal("builtin registry returned no templates")
	}
	if len(refs) != len(builtinComponentParamsGolden) {
		t.Fatalf("golden table has %d entries but registry has %d templates: %v",
			len(builtinComponentParamsGolden), len(refs), refs)
	}
	refSet := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		refSet[r] = struct{}{}
	}
	for g := range builtinComponentParamsGolden {
		if _, ok := refSet[g]; !ok {
			t.Errorf("golden entry %q has no matching builtin template", g)
		}
	}
}

func TestBuiltInMetadataFromParserConfig_ExtractorNodeParams(t *testing.T) {
	pc := entity.JSONMap{
		"Extractor:AutoExtractDefault": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
				"built_in_metadata": []any{
					map[string]any{"key": "update_time", "type": "time"},
					map[string]any{"key": "file_name", "type": "string"},
				},
			},
		},
	}
	cfg, enabled := builtInMetadataFromParserConfig(pc)
	if !enabled {
		t.Fatal("auto metadata should be enabled from extractor node modular config")
	}
	if len(cfg) != 2 {
		t.Fatalf("built-in config len = %d, want 2", len(cfg))
	}
}

func TestBuiltInMetadataFromParserConfig_None(t *testing.T) {
	cfg, enabled := builtInMetadataFromParserConfig(entity.JSONMap{})
	if enabled || len(cfg) != 0 {
		t.Fatalf("got enabled=%v cfg=%v, want disabled empty", enabled, cfg)
	}
}

func TestDocNameValue(t *testing.T) {
	name := "report.pdf"
	if got := docNameValue(&name); got != "report.pdf" {
		t.Errorf("docNameValue = %q, want report.pdf", got)
	}
	if got := docNameValue(nil); got != "" {
		t.Errorf("docNameValue(nil) = %q, want empty", got)
	}
}

func TestBuiltInMetadataFromParserConfig_MultiExtractorDeterministic(t *testing.T) {
	// Multiple Extractor nodes with different built_in_metadata.
	// Since keys are sorted alphabetically ("Extractor:A" before "Extractor:B"),
	// Extractor:A must deterministically win regardless of Go map iteration order.
	pc := entity.JSONMap{
		"Extractor:B": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
				"built_in_metadata": []any{
					map[string]any{"key": "file_name", "type": "string"},
				},
			},
		},
		"Extractor:A": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
				"built_in_metadata": []any{
					map[string]any{"key": "update_time", "type": "time"},
				},
			},
		},
	}

	for i := 0; i < 20; i++ {
		cfg, enabled := builtInMetadataFromParserConfig(pc)
		if !enabled {
			t.Fatalf("iteration %d: expected enabled=true", i)
		}
		if len(cfg) != 1 {
			t.Fatalf("iteration %d: expected 1 item, got %d", i, len(cfg))
		}
		item, ok := cfg[0].(map[string]any)
		if !ok || item["key"] != "update_time" {
			t.Fatalf("iteration %d: expected Extractor:A item update_time, got %v", i, cfg[0])
		}
	}
}

func TestBuiltInMetadataFromParserConfig_FirstMetadataNodeWins(t *testing.T) {
	// The alphabetically-first Extractor node that declares metadata is the
	// authoritative source, even when its built_in_metadata is empty.
	pc := entity.JSONMap{
		"Extractor:A": map[string]any{
			"metadata": map[string]any{
				"enabled":           true,
				"built_in_metadata": []any{},
			},
		},
		"Extractor:B": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
				"built_in_metadata": []any{
					map[string]any{"key": "file_name", "type": "string"},
				},
			},
		},
	}

	cfg, enabled := builtInMetadataFromParserConfig(pc)
	if !enabled {
		t.Fatalf("expected enabled=true from first metadata node, got false")
	}
	if len(cfg) != 0 {
		t.Fatalf("expected empty built-in config from first metadata node, got %#v", cfg)
	}
}

func TestBuiltInMetadataFromParserConfig_MapSliceBuiltIn(t *testing.T) {
	pc := entity.JSONMap{
		"Extractor:A": map[string]any{
			"metadata": map[string]any{
				"enabled": true,
				"built_in_metadata": []map[string]any{
					{"key": "update_time", "type": "time"},
				},
			},
		},
	}

	cfg, enabled := builtInMetadataFromParserConfig(pc)
	if !enabled {
		t.Fatalf("expected enabled=true, got false")
	}
	if len(cfg) != 1 {
		t.Fatalf("expected 1 built-in item from []map[string]any, got %d", len(cfg))
	}
	item, ok := cfg[0].(map[string]any)
	if !ok || item["key"] != "update_time" {
		t.Fatalf("expected update_time item, got %#v", cfg[0])
	}
}
