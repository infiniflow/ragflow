/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

/**
 * Pipeline parser configs are keyed by operator id (e.g. "Parser:xxx"), so a
 * top-level key containing ":" marks the pipeline structure, which must be
 * sent as-is instead of being reshaped by normalizeParserConfig.
 */
export const isPipelineParserConfig = (
  parserConfig: Record<string, any> | undefined,
): boolean => {
  if (!parserConfig || typeof parserConfig !== 'object') {
    return false;
  }
  return Object.keys(parserConfig).some((key) => key.includes(':'));
};

const MinerUOptionKeys = [
  'mineru_parse_method',
  'mineru_formula_enable',
  'mineru_table_enable',
  'mineru_lang',
] as const;

const isMinerULayoutRecognize = (layoutRecognize: unknown): boolean =>
  typeof layoutRecognize === 'string' &&
  layoutRecognize.toLowerCase().includes('mineru');

/**
 * Normalizes parser configuration before it is sent to the API.
 *
 * The Python backend keeps the legacy flat-key contract (top-level `metadata`
 * / `parent_child` and all parser-level keys as-is).
 *
 * The Go backend rejects every flat (non-component-scoped) key, so this
 * normalizer drops all parser-level flat keys (`chunk_token_num`, `delimiter`,
 * `auto_keywords`, `layout_recognize`, ...) and emits only component-scoped
 * keys (those containing `:`). The two legacy flat values the Go backend still
 * understands are re-homed onto their owning nodes:
 *   - `metadata`          → `Extractor:AutoExtractDefault.metadata`
 *   - `parent_child`      → `GeneralChunker:SixApplesFall.parent_child`
 *   - `chunk_token_num`   → `GeneralChunker:SixApplesFall.chunk_token_size`
 * (the built-in path reads `chunk_token_size` off the chunker node, never a
 * flat `chunk_token_num`). Any key already containing `:` is passed through
 * untouched.
 * @param parserConfig - The parser configuration object
 * @returns Processed parser config
 */
export const normalizeParserConfig = (
  parserConfig: Record<string, any> | undefined,
) => {
  if (!parserConfig) return parserConfig;
  const {
    auto_keywords,
    auto_questions,
    chunk_token_num,
    delimiter,
    html4excel,
    layout_recognize,
    tag_kb_ids,
    topn_tags,
    filename_embd_weight,
    task_page_size,
    pages,
    children_delimiter,
    use_parent_child,
    enable_children,
    metadata,
    ...additionalParserConfig
  } = parserConfig;
  delete additionalParserConfig.graphrag;
  delete additionalParserConfig.raptor;
  // Do not persist MinerU-only options when another layout recognizer is
  // selected; leftover mineru_* keys used to falsely trigger MinerU fallback.
  if (!isMinerULayoutRecognize(layout_recognize)) {
    for (const key of MinerUOptionKeys) {
      delete additionalParserConfig[key];
    }
  }

  // When children are enabled we forward the delimiter and the explicit
  // use_parent_child override (defaulting to enabled). When they are explicitly
  // disabled we must still emit a parent_child payload with use_parent_child:
  // false, otherwise a pre-existing node's parent_child (e.g. {use_parent_child:
  // true}) would pass through untouched and silently keep children parsing ON.
  // When enable_children is absent we leave any existing parent_child untouched.
  const parentChild =
    enable_children === true
      ? {
          children_delimiter,
          use_parent_child: use_parent_child ?? true,
        }
      : enable_children === false
        ? { use_parent_child: false }
        : undefined;

  // Emit ONLY component-scoped keys. The legacy flat parser-level keys have no
  // consumer (the built-in path reads component nodes, e.g.
  // GeneralChunker:SixApplesFall.chunk_token_size), so they are dropped rather
  // than forwarded. Already-scoped keys pass through untouched.
  const scoped: Record<string, any> = {};
  for (const [key, value] of Object.entries(additionalParserConfig)) {
    if (key.includes(':')) {
      scoped[key] = value;
    }
  }
  if (chunk_token_num !== undefined && chunk_token_num !== null) {
    scoped['GeneralChunker:SixApplesFall'] = {
      ...(scoped['GeneralChunker:SixApplesFall'] as
        | Record<string, any>
        | undefined),
      chunk_token_size: chunk_token_num,
    };
  }
  if (metadata && typeof metadata === 'object') {
    scoped['Extractor:AutoExtractDefault'] = {
      ...(scoped['Extractor:AutoExtractDefault'] as
        | Record<string, any>
        | undefined),
      metadata,
    };
  }
  if (parentChild) {
    scoped['GeneralChunker:SixApplesFall'] = {
      ...(scoped['GeneralChunker:SixApplesFall'] as
        | Record<string, any>
        | undefined),
      parent_child: parentChild,
    };
  }
  return scoped;
};
