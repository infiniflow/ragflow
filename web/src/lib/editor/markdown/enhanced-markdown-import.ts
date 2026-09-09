/**
 * Enhanced markdown import with CRLF normalization and tab collapse.
 * Ported from Nimbalyst.
 *
 * Uses placeholder-based approach for mermaid blocks:
 * 1. Replace ```mermaid blocks with unique placeholders in the raw text
 * 2. Standard Lexical import (placeholders become TextNodes)
 * 3. Walk tree, replace placeholder TextNodes with MermaidNode
 *
 * This preserves the original position of mermaid diagrams in the document.
 */

import { $isCodeNode } from '@lexical/code';
import type {
  ElementTransformer,
  MultilineElementTransformer,
  TextFormatTransformer,
  TextMatchTransformer,
} from '@lexical/markdown';
import { $convertFromMarkdownString } from '@lexical/markdown';
import {
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  $isElementNode,
  $isTabNode,
  $isTextNode,
  type ElementNode,
  type LexicalNode,
} from 'lexical';
import { $createMermaidNode } from '../mermaid-node';
type LexicalTransformer =
  | ElementTransformer
  | MultilineElementTransformer
  | TextFormatTransformer
  | TextMatchTransformer;

export interface EnhancedImportOptions {
  preserveNewLines?: boolean;
}

// Unique placeholder prefix — must not appear in normal markdown text
const PREFIX = '%%MERMAID_PLACEHOLDER_';
const SUFFIX = '%%';

export function $convertFromEnhancedMarkdownString(
  markdown: string,
  transformers?: LexicalTransformer[],
  node?: ElementNode,
  preserveNewLines: boolean = true,
): void {
  if (!markdown) {
    const root = node ?? $getRoot();
    root.append($createParagraphNode());
    return;
  }

  const root = node ?? $getRoot();

  // Step 1: Normalize line endings
  const normalizedMarkdown = markdown
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n');

  // Step 2: Replace ```mermaid ... ``` blocks with unique placeholders
  // Matches both ```mermaid and ``` mermaid (with optional space after backticks)
  const MERMAID_REGEX = /```\s*mermaid\s*\n([\s\S]*?)```/g;
  const mermaidContent: string[] = [];
  let placeholderIdx = 0;
  const markdownWithPlaceholders = normalizedMarkdown.replace(
    MERMAID_REGEX,
    (_match, content) => {
      mermaidContent.push(content.trim());
      const ph = `${PREFIX}${placeholderIdx}${SUFFIX}`;
      placeholderIdx++;
      return ph;
    },
  );

  // Step 3: Standard Lexical import (placeholders are just text)
  $convertFromMarkdownString(
    markdownWithPlaceholders,
    transformers || [],
    root,
    preserveNewLines,
  );

  // Step 4: Walk tree and replace placeholder TextNodes with MermaidNode
  $replacePlaceholders(root, mermaidContent);

  // Step 5: Fallback — also handle CodeNode(language='mermaid') if any
  $replaceMermaidCodeNodes(root);

  // Step 6: Collapse TabNodes
  $collapseTabNodes(root);
}

/**
 * Walks the subtree and replaces TextNodes containing %%MERMAID_PLACEHOLDER_N%%
 * with MermaidNode containing the corresponding content.
 */
function $replacePlaceholders(
  root: ElementNode,
  mermaidContent: string[],
): void {
  const visit = (node: LexicalNode): void => {
    if ($isTextNode(node)) {
      const text = node.getTextContent();
      if (text.startsWith(PREFIX)) {
        const idxStr = text.slice(PREFIX.length, -SUFFIX.length);
        const idx = parseInt(idxStr, 10);
        if (!isNaN(idx) && idx >= 0 && idx < mermaidContent.length) {
          const content = mermaidContent[idx];
          if (content) {
            const mermaidNode = $createMermaidNode({ content });
            node.replace(mermaidNode);
            return;
          }
        }
      }
    }
    if ($isElementNode(node)) {
      // Walk children in reverse to maintain indices during mutation
      const children = node.getChildren();
      for (let i = children.length - 1; i >= 0; i--) {
        visit(children[i]);
      }
    }
  };
  visit(root);
}

/**
 * Fallback: replaces CodeNode(language='mermaid') with MermaidNode.
 */
function $replaceMermaidCodeNodes(root: ElementNode): void {
  const visit = (node: LexicalNode): void => {
    if ($isCodeNode(node) && node.getLanguage() === 'mermaid') {
      const text = node.getTextContent();
      const mermaidNode = $createMermaidNode({ content: text });
      node.replace(mermaidNode);
      return;
    }
    if ($isElementNode(node)) {
      for (const child of node.getChildren()) {
        visit(child);
      }
    }
  };
  visit(root);
}

function $collapseTabNodes(root: ElementNode): void {
  const visit = (node: LexicalNode): void => {
    if ($isTabNode(node)) {
      node.replace($createTextNode('\t'));
      return;
    }
    if ($isElementNode(node)) {
      for (const child of node.getChildren()) {
        visit(child);
      }
    }
  };
  visit(root);
}
