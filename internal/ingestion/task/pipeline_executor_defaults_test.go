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
	"audio":        "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:SongsFillAir\":{\"audio\":{\"output_format\":\"text\",\"preprocess\":[\"main_content\"],\"suffix\":[\"aac\",\"aiff\",\"ape\",\"au\",\"da\",\"flac\",\"midi\",\"mp3\",\"ogg\",\"oggvorbis\",\"realaudio\",\"vqf\",\"wav\",\"wave\",\"wma\"]}},\"TokenChunker:BlueSkiesLaugh\":{},\"Tokenizer:KindEyesWatch\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"book":         "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:HipSignsRhyme\":{\"doc\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"doc\"]},\"docx\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"docx\"],\"vlm\":{}},\"html\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"htm\",\"html\"]},\"pdf\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"remove_toc\":true,\"suffix\":[\"pdf\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\"]}},\"TitleChunker:GrumpyGarlicsBake\":{\"hierarchy\":5,\"include_heading_content\":true,\"levels\":[[\"^#[^#]\",\"^##[^#]\",\"^###[^#]\",\"^####[^#]\"],[\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\",\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"第[零一二三四五六七八九十百0-9]+条\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"],[\"第[0-9]+章\",\"第[0-9]+节\",\"[0-9]{1,2}[\\\\. 、]\",\"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\",\"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"],[\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"[零一二三四五六七八九十百]+[ 、]\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\",\"[\\\\(（][0-9]{,2}[\\\\)）]\"],[\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\",\"Chapter (I+V?|VI*|XI|IX|X)\",\"Section [0-9]+\",\"Article [0-9]+\"]],\"method\":\"hierarchy\"},\"Tokenizer:HotDonutsRing\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"email":        "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:BirdsFlutterHigh\":{\"email\":{\"fields\":[\"from\",\"to\",\"cc\",\"bcc\",\"date\",\"subject\",\"body\",\"attachments\"],\"output_format\":\"text\",\"preprocess\":[\"main_content\"],\"suffix\":[\"eml\"]}},\"TokenChunker:WarmBreadSmells\":{\"children_delimiters\":[],\"chunk_token_size\":512,\"delimiter_mode\":\"token_size\",\"delimiters\":[\"\\n\",\"!\",\"?\",\"。\",\"；\",\"！\",\"？\"],\"image_context_size\":0,\"overlapped_percent\":0,\"table_context_size\":0},\"Tokenizer:NiceWordsSpoken\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"general":      "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:HipSignsRhyme\":{\"doc\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"doc\"]},\"docx\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"docx\"],\"vlm\":{}},\"html\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"htm\",\"html\"]},\"markdown\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"md\",\"markdown\",\"mdx\"],\"vlm\":{}},\"pdf\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}},\"spreadsheet\":{\"flatten_media_to_text\":false,\"output_format\":\"html\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"xls\",\"xlsx\",\"csv\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\",\"py\",\"js\",\"java\",\"c\",\"cpp\",\"h\",\"php\",\"go\",\"ts\",\"sh\",\"cs\",\"kt\",\"sql\"]}},\"TokenChunker:SixApplesFall\":{\"children_delimiters\":[],\"chunk_token_size\":512,\"delimiter_mode\":\"token_size\",\"delimiters\":[\"\\n\",\"!\",\"?\",\"。\",\"；\",\"！\",\"？\"],\"image_context_size\":0,\"overlapped_percent\":0,\"table_context_size\":0},\"Tokenizer:LegalReadersDecide\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"laws":         "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:HipSignsRhyme\":{\"doc\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"doc\"]},\"docx\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"docx\"],\"vlm\":{}},\"html\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"htm\",\"html\"]},\"markdown\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"md\",\"markdown\",\"mdx\"],\"vlm\":{}},\"pdf\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\"]}},\"TitleChunker:SpicyKeysKick\":{\"hierarchy\":2,\"include_heading_content\":false,\"levels\":[[\"^#[^#]\",\"^##[^#]\",\"^###[^#]\",\"^####[^#]\"],[\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\",\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"第[零一二三四五六七八九十百0-9]+条\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"],[\"第[0-9]+章\",\"第[0-9]+节\",\"[0-9]{1,2}[\\\\. 、]\",\"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\",\"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"],[\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"[零一二三四五六七八九十百]+[ 、]\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\",\"[\\\\(（][0-9]{,2}[\\\\)）]\"],[\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\",\"Chapter (I+V?|VI*|XI|IX|X)\",\"Section [0-9]+\",\"Article [0-9]+\"]],\"method\":\"hierarchy\"},\"Tokenizer:PublicJobsTake\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"manual":       "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:HipSignsRhyme\":{\"doc\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"doc\"]},\"docx\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"docx\"],\"vlm\":{}},\"pdf\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}}},\"TitleChunker:NineInsectsFind\":{\"hierarchy\":0,\"include_heading_content\":false,\"levels\":[[\"^#[^#]\",\"^##[^#]\",\"^###[^#]\",\"^####[^#]\"],[\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\",\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"第[零一二三四五六七八九十百0-9]+条\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"],[\"第[0-9]+章\",\"第[0-9]+节\",\"[0-9]{1,2}[\\\\. 、]\",\"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\",\"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"],[\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"[零一二三四五六七八九十百]+[ 、]\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\",\"[\\\\(（][0-9]{,2}[\\\\)）]\"],[\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\",\"Chapter (I+V?|VI*|XI|IX|X)\",\"Section [0-9]+\",\"Article [0-9]+\"]],\"method\":\"group\"},\"Tokenizer:FunnyBalloonsGrin\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"one":          "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"OneChunker:DryDrinksVisit\":{},\"Parser:HipSignsRhyme\":{\"doc\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"doc\"]},\"docx\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"docx\"],\"vlm\":{}},\"html\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"htm\",\"html\"]},\"markdown\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"md\",\"markdown\",\"mdx\"],\"vlm\":{}},\"pdf\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}},\"spreadsheet\":{\"flatten_media_to_text\":false,\"output_format\":\"html\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"xls\",\"xlsx\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\"]}},\"Tokenizer:FrankWeeksListen\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"paper":        "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:HipSignsRhyme\":{\"pdf\":{\"enable_multi_column\":true,\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}}},\"TitleChunker:SparklySchoolsTravel\":{\"hierarchy\":0,\"include_heading_content\":false,\"levels\":[[\"^#[^#]\",\"^##[^#]\",\"^###[^#]\",\"^####[^#]\"],[\"第[零一二三四五六七八九十百0-9]+(分?编|部分)\",\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"第[零一二三四五六七八九十百0-9]+条\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\"],[\"第[0-9]+章\",\"第[0-9]+节\",\"[0-9]{1,2}[\\\\. 、]\",\"[0-9]{1,2}\\\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])\",\"[0-9]{1,2}\\\\.[0-9]{1,2}\\\\.[0-9]{1,2}\"],[\"第[零一二三四五六七八九十百0-9]+章\",\"第[零一二三四五六七八九十百0-9]+节\",\"[零一二三四五六七八九十百]+[ 、]\",\"[\\\\(（][零一二三四五六七八九十百]+[\\\\)）]\",\"[\\\\(（][0-9]{,2}[\\\\)）]\"],[\"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)\",\"Chapter (I+V?|VI*|XI|IX|X)\",\"Section [0-9]+\",\"Article [0-9]+\"]],\"method\":\"group\"},\"Tokenizer:GreatCarsWash\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"picture":      "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:ViewsCaptureLight\":{\"image\":{\"output_format\":\"text\",\"parse_method\":\"ocr\",\"preprocess\":[\"main_content\"],\"suffix\":[\"bmp\",\"gif\",\"jpeg\",\"jpg\",\"png\",\"svg\",\"tif\",\"tiff\",\"webp\"]},\"video\":{\"output_format\":\"text\",\"preprocess\":[\"main_content\"],\"suffix\":[\"3gp\",\"3gpp\",\"avi\",\"flv\",\"mkv\",\"mov\",\"mp4\",\"mpeg\",\"mpg\",\"webm\",\"wmv\"]}},\"TokenChunker:BrightColorsGlow\":{},\"Tokenizer:SharpLensFocus\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"presentation": "{\"Extractor:AutoExtractDefault\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"\",\"llm_id\":\"\"},\"File\":{},\"Parser:HipSignsRhyme\":{\"pdf\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}},\"slides\":{\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pptx\",\"ppt\"]}},\"PresentationChunker:HappyHillsGlow\":{},\"Tokenizer:TallTreesDance\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"qa":           "{\"File\":{},\"Parser:HipSignsRhyme\":{\"docx\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"docx\"],\"vlm\":{}},\"markdown\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"md\",\"markdown\",\"mdx\"],\"vlm\":{}},\"pdf\":{\"flatten_media_to_text\":false,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}},\"spreadsheet\":{\"flatten_media_to_text\":false,\"output_format\":\"html\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"xls\",\"xlsx\",\"csv\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\"]}},\"QAChunker:TidyCloudsThink\":{},\"Tokenizer:ColdCloudsDream\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"resume":       "{\"Extractor:ThreeDrinksAct\":{\"auto_keywords\":0,\"auto_questions\":0,\"field_name\":\"metadata\",\"frequencyPenaltyEnabled\":true,\"frequency_penalty\":0.7,\"llm_id\":\"THUDM/GLM-4.1V-9B-Thinking@SILICONFLOW\",\"maxTokensEnabled\":false,\"max_tokens\":256,\"presencePenaltyEnabled\":true,\"presence_penalty\":0.4,\"prompts\":[{\"content\":\"Content: {TitleChunker:FlatMiceFix@chunks}\",\"role\":\"user\"}],\"sys_prompt\":\"Act as a precise resume metadata extractor. Extract stable, chunk-supported metadata from the provided resume content.\\n\\nRules:\\n1. Use only information explicitly stated in the content. Do not infer, guess, normalize, or add missing facts.\\n2. The input may be only one chunk of a resume. Extract only what this content directly supports.\\n3. Use only these field names:\\ncandidate_name, gender, phone, email, city, location, nationality, linkedin, github, website, highest_degree, degree_levels, school_names, majors, graduation_years, work_experience_years, current_job_title, job_titles, company_names, job_experience, industries, target_job_titles, target_locations, employment_types, skills, certificates, awards, summary_tags\\n4. Ignore detailed responsibilities, project descriptions, achievement narratives, self-evaluation, and other low-value local details.\\n5. Keep values in the same language as the source text whenever possible.\\n6. Remove duplicates and keep only concise, high-value metadata.\\n7. Return only fields that are explicitly supported by the content. Do not return empty or unsupported fields.\\n\\nField guidance:\\n- highest_degree: highest explicit degree level mentioned\\n- degree_levels: all explicit degree levels mentioned\\n- school_names: explicit school, college, or university names\\n- majors: explicit fields of study\\n- graduation_years: explicit graduation years only\\n- work_experience_years: only if explicitly stated\\n- current_job_title: only if explicitly current or most recent\\n- job_titles: explicit role titles\\n- company_names: explicit employer names\\n- job_experience: concise structured work entries explicitly supported by the content, preferably including title, company, and time information when available\\n- industries: explicit industry names only\\n- target_job_titles: explicit desired roles only\\n- target_locations: explicit desired work locations only\\n- skills: concise, core, search-useful skills explicitly mentioned\\n- certificates: explicit certificate names only\\n- awards: explicit award names only\\n- summary_tags: short, high-value tags strictly supported by the content\\n\\nReturn only the extracted metadata. Do not output explanatory text.\",\"temperature\":0.1,\"temperatureEnabled\":true,\"tenant_llm_id\":29,\"topPEnabled\":true,\"top_p\":0.3},\"File\":{},\"Parser:HipSignsRhyme\":{\"docx\":{\"flatten_media_to_text\":true,\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"docx\"],\"vlm\":{}},\"pdf\":{\"flatten_media_to_text\":true,\"output_format\":\"json\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"pdf\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\"]}},\"TitleChunker:FlatMiceFix\":{\"hierarchy\":1,\"include_heading_content\":false,\"levels\":[[\"^\\\\s*(?i:(?:\\\\d+[\\\\.\\\\)]\\\\s*)?(?:EDUCATION|ACADEMIC\\\\s*BACKGROUND|ACADEMIC\\\\s*HISTORY|EDUCATIONAL\\\\s*BACKGROUND|RELEVANT\\\\s*COURSEWORK|COURSEWORK|EXPERIENCE|WORK\\\\s*EXPERIENCE|PROFESSIONAL\\\\s*EXPERIENCE|RELEVANT\\\\s*EXPERIENCE|EMPLOYMENT\\\\s*HISTORY|CAREER\\\\s*HISTORY|INTERNSHIP\\\\s*EXPERIENCE|PROJECTS|PROJECT\\\\s*EXPERIENCE|ACADEMIC\\\\s*PROJECTS|PROFESSIONAL\\\\s*PROJECTS|SKILLS|TECHNICAL\\\\s*SKILLS|CORE\\\\s*COMPETENCIES|COMPETENCIES|QUALIFICATIONS|SUMMARY\\\\s*OF\\\\s*QUALIFICATIONS|CERTIFICATIONS|LICENSES|CERTIFICATES|AWARDS|HONORS|HONOURS|ACHIEVEMENTS|PUBLICATIONS|RESEARCH|RESEARCH\\\\s*EXPERIENCE|LEADERSHIP|LEADERSHIP\\\\s*EXPERIENCE|ACTIVITIES|EXTRACURRICULAR\\\\s*ACTIVITIES|ACTIVITIES\\\\s*(?:\\u0026|AND)\\\\s*SKILLS|INVOLVEMENT|CAMPUS\\\\s*INVOLVEMENT|VOLUNTEER\\\\s*EXPERIENCE|VOLUNTEERING|COMMUNITY\\\\s*SERVICE|LANGUAGES|INTERESTS|HOBBIES|PROFILE|PROFESSIONAL\\\\s*PROFILE|SUMMARY|PROFESSIONAL\\\\s*SUMMARY|CAREER\\\\s*SUMMARY|OBJECTIVE|CAREER\\\\s*OBJECTIVE|PERSONAL\\\\s*INFORMATION|CONTACT\\\\s*INFORMATION|ADDITIONAL\\\\s*INFORMATION|TRAINING))\\\\s*[:：]?\\\\s*$\"],[\"^\\\\s*(?:\\\\d+[\\\\.、\\\\)]\\\\s*)?(?:教育背景|教育经历|学历背景|学术背景|技术背景|工作经历|工作经验|实习经历|项目经历|项目经验|科研经历|研究经历|校园经历|实践经历|专业经历|职业经历|技能|专业技能|技能特长|核心技能|技术栈|个人技能|工作技能|职业技能|技能与评价|技能与自我评价|工作技能与自我评价|职业技能与自我评价|证书|资格证书|职业资格|资质证书|获奖情况|获奖经历|荣誉|荣誉奖项|奖项|科研成果|论文发表|发表论文|领导经历|学生工作|校园活动|社团经历|活动经历|志愿经历|志愿服务|社会实践|语言能力|语言|自我评价|个人评价|自我总结|个人总结|个人优势|个人简介|个人信息|基本信息|联系方式|求职意向|应聘意向|职业目标|求职目标|兴趣爱好|兴趣特长|培训经历|其他信息|附加信息)\\\\s*[:：]?\\\\s*$\"]],\"method\":\"hierarchy\",\"root_chunk_as_heading\":true},\"Tokenizer:KindHandsWin\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"table":        "{\"File\":{},\"Parser:HipSignsRhyme\":{\"spreadsheet\":{\"column_mode\":\"auto\",\"column_names\":[],\"column_roles\":{},\"flatten_media_to_text\":false,\"output_format\":\"html\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"xls\",\"xlsx\",\"csv\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\"]}},\"TableChunker:FastFoxesJump\":{},\"Tokenizer:DeepLakesShine\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
	"tag":          "{\"File\":{},\"Parser:HipSignsRhyme\":{\"spreadsheet\":{\"flatten_media_to_text\":false,\"output_format\":\"html\",\"parse_method\":\"DeepDOC\",\"preprocess\":[\"main_content\"],\"suffix\":[\"xls\",\"xlsx\",\"csv\"],\"vlm\":{}},\"text\\u0026code\":{\"output_format\":\"json\",\"preprocess\":[\"main_content\"],\"suffix\":[\"txt\"]}},\"TagChunker:NewNoonsGlow\":{},\"Tokenizer:OldOwlsWatch\":{\"fields\":\"text\",\"filename_embd_weight\":0.1,\"search_method\":[\"embedding\",\"full_text\"]}}",
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
func TestBuildComponentParams_Qa(t *testing.T)     { assertTemplateComponentParams(t, "qa") }
func TestBuildComponentParams_Resume(t *testing.T) { assertTemplateComponentParams(t, "resume") }
func TestBuildComponentParams_Table(t *testing.T)  { assertTemplateComponentParams(t, "table") }
func TestBuildComponentParams_Tag(t *testing.T)    { assertTemplateComponentParams(t, "tag") }

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
