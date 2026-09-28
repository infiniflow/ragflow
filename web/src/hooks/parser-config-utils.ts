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

import { pickByBackend } from '@/utils/backend-variant';

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
 * / `parent_child`). The Go backend requires component-scoped keys, so the
 * dataset-level `metadata` is nested under `Extractor:AutoExtractDefault` and
 * `parent_child` under `GeneralChunker:SixApplesFall`; no flat `metadata` or
 * `parent_child` is emitted. All other (parser-level) keys are left flat for
 * both backends, matching what the Go backend preserves on the built-in path.
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

  const parentChild = enable_children
    ? {
        children_delimiter,
        use_parent_child: use_parent_child ?? enable_children,
      }
    : undefined;

  const flat: Record<string, any> = {
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
    enable_children,
    metadata,
    ...(parentChild ? { parent_child: parentChild } : {}),
    ...additionalParserConfig,
  };

  // Python backend: keep the legacy flat-key contract unchanged.
  if (!pickByBackend({ go: true, python: false })) {
    return flat;
  }

  // Go backend: scope dataset-level metadata/parent_child onto their owning
  // component nodes; never emit a flat `metadata`/`parent_child` key.
  const scoped: Record<string, any> = { ...flat };
  delete scoped.metadata;
  delete scoped.parent_child;
  if (metadata && typeof metadata === 'object') {
    scoped['Extractor:AutoExtractDefault'] = {
      ...(scoped['Extractor:AutoExtractDefault'] as Record<string, any> | undefined),
      metadata,
    };
  }
  if (parentChild) {
    scoped['GeneralChunker:SixApplesFall'] = {
      ...(scoped['GeneralChunker:SixApplesFall'] as Record<string, any> | undefined),
      parent_child: parentChild,
    };
  }
  return scoped;
};
