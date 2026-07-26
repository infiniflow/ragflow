/**
 * Editor module public API.
 */

export { default as theme } from './editor-theme';
export { default as LexicalEditor } from './lexical-editor';
export { default as nodes } from './nodes';
export { default as RawMarkdownEditor } from './raw-markdown-editor';

export {
  $convertFromEnhancedMarkdownString,
  $convertToEnhancedMarkdownString,
  CORE_TRANSFORMERS,
} from './markdown';

export {
  $createMermaidNode,
  $isMermaidNode,
  MermaidNode,
} from './mermaid-node';
export type { MermaidPayload } from './mermaid-node';

export { MERMAID_TRANSFORMER } from './mermaid-transformer';
