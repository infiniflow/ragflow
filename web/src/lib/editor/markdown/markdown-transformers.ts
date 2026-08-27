/**
 * Markdown transformers for Lexical import/export.
 * Ported from Nimbalyst — handles:
 * - HEADING, QUOTE, CODE, MERMAID
 * - LISTS (via ListTransformers)
 * - Text format (bold, italic, strikethrough, code, highlight)
 * - LINK, HR
 */

/* eslint-disable @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-object-type, no-useless-escape */

import { $createCodeNode, $isCodeNode, CodeNode } from '@lexical/code';
import {
  $createLinkNode,
  $isAutoLinkNode,
  $isLinkNode,
  LinkNode,
} from '@lexical/link';
import {
  BOLD_ITALIC_STAR as UPSTREAM_BOLD_ITALIC_STAR,
  BOLD_ITALIC_UNDERSCORE as UPSTREAM_BOLD_ITALIC_UNDERSCORE,
  BOLD_STAR as UPSTREAM_BOLD_STAR,
  BOLD_UNDERSCORE as UPSTREAM_BOLD_UNDERSCORE,
  HIGHLIGHT as UPSTREAM_HIGHLIGHT,
  INLINE_CODE as UPSTREAM_INLINE_CODE,
  ITALIC_STAR as UPSTREAM_ITALIC_STAR,
  ITALIC_UNDERSCORE as UPSTREAM_ITALIC_UNDERSCORE,
  STRIKETHROUGH as UPSTREAM_STRIKETHROUGH,
} from '@lexical/markdown';
import type { HeadingTagType } from '@lexical/rich-text';
import {
  $createHeadingNode,
  $createQuoteNode,
  $isHeadingNode,
  $isQuoteNode,
  HeadingNode,
  QuoteNode,
} from '@lexical/rich-text';
import {
  $createLineBreakNode,
  $createTextNode,
  type ElementNode,
  type LexicalNode,
} from 'lexical';

import type {
  ElementTransformer,
  MultilineElementTransformer,
  TextFormatTransformer,
  TextMatchTransformer,
  Transformer,
} from '@lexical/markdown';

import {
  CHECK_LIST,
  getListConfig,
  ORDERED_LIST,
  setListConfig,
  UNORDERED_LIST,
  type ListConfig,
} from './list-transformers';

export {
  CHECK_LIST,
  getListConfig,
  ORDERED_LIST,
  setListConfig,
  UNORDERED_LIST,
  type ListConfig,
};

import { MERMAID_TRANSFORMER } from '../mermaid-transformer';
import { TABLE_TRANSFORMER } from '../plugins/table-transformer';
import { HR_TRANSFORMER } from './horizontal-rule-transformer';

const HEADING_REGEX = /^(#{1,6})\s/;
const QUOTE_REGEX = /^>\s/;
const CODE_START_REGEX = /^[ \t]*```([\w-]+)?/;
const CODE_END_REGEX = /[ \t]*```$/;

const createBlockNode = (
  createNode: (match: Array<string>) => ElementNode,
): ElementTransformer['replace'] => {
  return (parentNode, children, match, isImport) => {
    const node = createNode(match);
    node.append(...children);
    parentNode.replace(node);
    if (!isImport) node.select(0, 0);
  };
};

export interface MarkdownConfig {}

export function setMarkdownConfig(
  config: Partial<MarkdownConfig & ListConfig>,
): void {
  if (
    'exportIndentSize' in config ||
    'importMinIndentSize' in config ||
    'importMaxIndentSize' in config ||
    'autoDetectIndent' in config
  ) {
    setListConfig(config);
  }
}

export function getMarkdownConfig(): MarkdownConfig & ListConfig {
  return { ...getListConfig() };
}

export const HEADING: ElementTransformer = {
  dependencies: [HeadingNode],
  export: (node, exportChildren) => {
    if (!$isHeadingNode(node)) return null;
    const level = Number(node.getTag().slice(1));
    return '#'.repeat(level) + ' ' + exportChildren(node);
  },
  regExp: HEADING_REGEX,
  replace: createBlockNode((match) => {
    const tag = ('h' + match[1].length) as HeadingTagType;
    return $createHeadingNode(tag);
  }),
  type: 'element',
};

export const QUOTE: ElementTransformer = {
  dependencies: [QuoteNode],
  export: (node, exportChildren) => {
    if (!$isQuoteNode(node)) return null;
    const lines = exportChildren(node).split('\n');
    return lines.map((line) => '> ' + line).join('\n');
  },
  regExp: QUOTE_REGEX,
  replace: (parentNode, children, _match, isImport) => {
    if (isImport) {
      const previousNode = parentNode.getPreviousSibling();
      if ($isQuoteNode(previousNode)) {
        previousNode.splice(previousNode.getChildrenSize(), 0, [
          $createLineBreakNode(),
          ...children,
        ]);
        parentNode.remove();
        return;
      }
    }
    const node = $createQuoteNode();
    node.append(...children);
    parentNode.replace(node);
    if (!isImport) node.select(0, 0);
  },
  type: 'element',
};

const NO_LANGUAGE_MARKER = 'plain';

export const CODE: MultilineElementTransformer = {
  dependencies: [CodeNode],
  export: (node: LexicalNode) => {
    if (!$isCodeNode(node)) return null;
    const textContent = node.getTextContent();
    const language = node.getLanguage();
    const langOutput = language === NO_LANGUAGE_MARKER ? '' : language || '';
    return (
      '```' +
      langOutput +
      (textContent ? '\n' + textContent : '') +
      '\n' +
      '```'
    );
  },
  regExpEnd: { optional: true, regExp: CODE_END_REGEX },
  regExpStart: CODE_START_REGEX,
  replace: (
    rootNode,
    children,
    startMatch,
    endMatch,
    linesInBetween,
    isImport,
  ) => {
    let codeBlockNode: CodeNode;
    let code: string;

    if (!children && linesInBetween) {
      if (linesInBetween.length === 1) {
        if (endMatch) {
          codeBlockNode = $createCodeNode(NO_LANGUAGE_MARKER);
          code = startMatch[1] + linesInBetween[0];
        } else {
          codeBlockNode = $createCodeNode(startMatch[1] || NO_LANGUAGE_MARKER);
          code = linesInBetween[0].startsWith(' ')
            ? linesInBetween[0].slice(1)
            : linesInBetween[0];
        }
      } else {
        codeBlockNode = $createCodeNode(startMatch[1] || NO_LANGUAGE_MARKER);
        if (linesInBetween[0].trim().length === 0) {
          while (linesInBetween.length > 0 && !linesInBetween[0].length)
            linesInBetween.shift();
        } else {
          linesInBetween[0] = linesInBetween[0].startsWith(' ')
            ? linesInBetween[0].slice(1)
            : linesInBetween[0];
        }
        while (
          linesInBetween.length > 0 &&
          !linesInBetween[linesInBetween.length - 1].length
        )
          linesInBetween.pop();
        code = linesInBetween.join('\n');
      }
      const textNode = $createTextNode(code);
      codeBlockNode.append(textNode);
      rootNode.append(codeBlockNode);
    } else if (children) {
      createBlockNode((match) => {
        return $createCodeNode(
          match && match[1] ? match[1] : NO_LANGUAGE_MARKER,
        );
      })(rootNode, children, startMatch, isImport);
    }
  },
  type: 'multiline-element',
};

// Re-export upstream text-format transformers
export const INLINE_CODE: TextFormatTransformer = UPSTREAM_INLINE_CODE;
export const HIGHLIGHT: TextFormatTransformer = UPSTREAM_HIGHLIGHT;
export const BOLD_ITALIC_STAR: TextFormatTransformer =
  UPSTREAM_BOLD_ITALIC_STAR;
export const BOLD_ITALIC_UNDERSCORE: TextFormatTransformer =
  UPSTREAM_BOLD_ITALIC_UNDERSCORE;
export const BOLD_STAR: TextFormatTransformer = UPSTREAM_BOLD_STAR;
export const BOLD_UNDERSCORE: TextFormatTransformer = UPSTREAM_BOLD_UNDERSCORE;
export const STRIKETHROUGH: TextFormatTransformer = UPSTREAM_STRIKETHROUGH;
export const ITALIC_STAR: TextFormatTransformer = UPSTREAM_ITALIC_STAR;
export const ITALIC_UNDERSCORE: TextFormatTransformer =
  UPSTREAM_ITALIC_UNDERSCORE;

export const LINK: TextMatchTransformer = {
  dependencies: [LinkNode],
  export: (node, exportChildren, _exportFormat) => {
    if (!$isLinkNode(node) || $isAutoLinkNode(node)) return null;
    const title = node.getTitle();
    const textContent = exportChildren(node);
    return title
      ? `[${textContent}](${node.getURL()} "${title}")`
      : `[${textContent}](${node.getURL()})`;
  },
  importRegExp:
    /(?<!!)(?:\[(?!!\[)([^\]]+)\])(?:\(([^\s\)]+)(?:\s+"([^"]*)")?\))/,
  regExp: /(?<!!)(?:\[(?!!\[)([^\]]+)\])(?:\(([^\s\)]+)(?:\s+"([^"]*)")?\))$/,
  replace: (textNode, match) => {
    const [, linkText, rawUrl, linkTitle] = match;
    const linkUrl =
      rawUrl && rawUrl.startsWith('<') && rawUrl.endsWith('>')
        ? rawUrl.slice(1, -1)
        : rawUrl;
    const linkNode = $createLinkNode(linkUrl, { title: linkTitle });
    const linkTextNode = $createTextNode(linkText);
    linkTextNode.setFormat(textNode.getFormat());
    linkNode.append(linkTextNode);
    textNode.replace(linkNode);
    return linkTextNode;
  },
  trigger: ')',
  type: 'text-match',
};

// Aggregated core transformer arrays
export const ELEMENT_TRANSFORMERS: Transformer[] = [
  HEADING,
  QUOTE,
  UNORDERED_LIST,
  ORDERED_LIST,
  TABLE_TRANSFORMER,
];

// MERMAID must come BEFORE CODE to intercept ```mermaid blocks first
export const MULTILINE_ELEMENT_TRANSFORMERS: Transformer[] = [
  MERMAID_TRANSFORMER,
  CODE,
];

export const TEXT_FORMAT_TRANSFORMERS: Transformer[] = [
  INLINE_CODE,
  BOLD_ITALIC_STAR,
  BOLD_ITALIC_UNDERSCORE,
  BOLD_STAR,
  BOLD_UNDERSCORE,
  HIGHLIGHT,
  ITALIC_STAR,
  ITALIC_UNDERSCORE,
  STRIKETHROUGH,
];
export const TEXT_MATCH_TRANSFORMERS: Transformer[] = [LINK];

export const CORE_TRANSFORMERS: Transformer[] = [
  HR_TRANSFORMER,
  CHECK_LIST,
  ...ELEMENT_TRANSFORMERS,
  ...MULTILINE_ELEMENT_TRANSFORMERS,
  ...TEXT_FORMAT_TRANSFORMERS,
  ...TEXT_MATCH_TRANSFORMERS,
];

export { MERMAID_TRANSFORMER } from '../mermaid-transformer';
export { TABLE_TRANSFORMER } from '../plugins/table-transformer';
