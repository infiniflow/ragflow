/**
 * Public API for the markdown pipeline.
 */

import { CORE_TRANSFORMERS } from './markdown-transformers';

import type { LexicalTransformer } from './enhanced-markdown-export';

export { $convertFromEnhancedMarkdownString } from './enhanced-markdown-import';
export type { EnhancedImportOptions } from './enhanced-markdown-import';

export {
  $convertNodeToEnhancedMarkdownString,
  $convertToEnhancedMarkdownString,
} from './enhanced-markdown-export';
export type {
  EnhancedExportOptions,
  LexicalTransformer,
} from './enhanced-markdown-export';

export {
  BOLD_ITALIC_STAR,
  BOLD_ITALIC_UNDERSCORE,
  BOLD_STAR,
  BOLD_UNDERSCORE,
  CHECK_LIST,
  CODE,
  CORE_TRANSFORMERS,
  ELEMENT_TRANSFORMERS,
  getListConfig,
  getMarkdownConfig,
  HEADING,
  HIGHLIGHT,
  INLINE_CODE,
  ITALIC_STAR,
  ITALIC_UNDERSCORE,
  LINK,
  MERMAID_TRANSFORMER,
  MULTILINE_ELEMENT_TRANSFORMERS,
  ORDERED_LIST,
  QUOTE,
  setListConfig,
  setMarkdownConfig,
  STRIKETHROUGH,
  TABLE_TRANSFORMER,
  TEXT_FORMAT_TRANSFORMERS,
  TEXT_MATCH_TRANSFORMERS,
  UNORDERED_LIST,
} from './markdown-transformers';
export type { ListConfig, MarkdownConfig } from './markdown-transformers';

// Re-export types from @lexical/markdown
export type {
  ElementTransformer,
  MultilineElementTransformer,
  TextFormatTransformer,
  TextMatchTransformer,
  Transformer,
} from '@lexical/markdown';

export {
  $createHorizontalRuleNode,
  $isHorizontalRuleNode,
  HorizontalRuleNode,
} from './horizontal-rule-node';
export type { SerializedHorizontalRuleNode } from './horizontal-rule-node';

/**
 * Returns the core set of transformers used for markdown import/export.
 * This function is used by plugins (e.g., TableTransformer) that need to
 * perform markdown conversion inside their own logic.
 */
export function getEditorTransformers(): LexicalTransformer[] {
  return CORE_TRANSFORMERS as LexicalTransformer[];
}
