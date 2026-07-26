/**
 * MermaidTransformer - handles ```mermaid code blocks.
 * Ported from Nimbalyst. Creates MermaidNode instead of CodeNode.
 */

/* eslint-disable no-console */

import type { MultilineElementTransformer } from '@lexical/markdown';
import {
  $createMermaidNode,
  $isMermaidNode,
  MermaidNode,
} from './mermaid-node';

const MERMAID_START_REGEX = /^[ \t]*```mermaid/;
const MERMAID_END_REGEX = /[ \t]*```$/;

export const MERMAID_TRANSFORMER: MultilineElementTransformer = {
  dependencies: [MermaidNode],
  export: (node) => {
    if (!$isMermaidNode(node)) return null;
    return '```mermaid\n' + node.getContent() + '\n```';
  },
  regExpStart: MERMAID_START_REGEX,
  regExpEnd: { optional: true, regExp: MERMAID_END_REGEX },
  replace: (rootNode, _children, _startMatch, _endMatch, linesInBetween) => {
    const content = linesInBetween ? linesInBetween.join('\n').trim() : '';
    console.log(
      '[MermaidTransformer] Creating MermaidNode, content length:',
      content.length,
      'lines:',
      linesInBetween?.length,
    );
    const mermaidNode = $createMermaidNode({ content });
    rootNode.append(mermaidNode);
  },
  type: 'multiline-element',
};
