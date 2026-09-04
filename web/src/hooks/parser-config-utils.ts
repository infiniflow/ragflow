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
 * Utility functions for extracting parser config extensions.
 * These functions extract known fields from parser config objects and merge
 * unknown fields into the `ext` field for flexible configuration.
 */

/**
 * Pipeline parser configs are keyed by operator id (e.g. "Parser:xxx"), so a
 * top-level key containing ":" marks the pipeline structure, which must be
 * sent as-is instead of being reshaped by extractParserConfigExt.
 */
export const isPipelineParserConfig = (
  parserConfig: Record<string, any> | undefined,
): boolean => {
  if (!parserConfig || typeof parserConfig !== 'object') {
    return false;
  }
  return Object.keys(parserConfig).some((key) => key.includes(':'));
};

/**
 * Extracts Parser configuration with extra fields merged into ext.
 * @param parserConfig - The parser configuration object
 * @returns Processed parser config with extra fields in ext
 */
export const extractParserConfigExt = (
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
    ext,
    ...parserExt
  } = parserConfig;
  delete parserExt.graphrag;
  delete parserExt.raptor;
  return {
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
    parent_child: enable_children
      ? {
          children_delimiter,
          use_parent_child: use_parent_child ?? enable_children,
        }
      : undefined,
    ext: { ...ext, ...parserExt },
  };
};
