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

import { decodeAngleBracketEntities } from '@/utils/chat';
import type { ElementContent } from 'hast';
import type { Root } from 'mdast';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import { visitParents } from 'unist-util-visit-parents';

function decodeMathText(children: ElementContent[]) {
  for (const child of children) {
    if (child.type === 'text') {
      child.value = decodeAngleBracketEntities(child.value);
    } else if (child.type === 'element') {
      decodeMathText(child.children);
    }
  }
}

function remarkDecodeMathEntities() {
  return (tree: Root) => {
    visitParents(tree, ['math', 'inlineMath'] as const, (node) => {
      // Keep angle brackets escaped in prose and HTML. The math parser
      // preserves entities literally, so restore them in its text nodes.
      node.value = decodeAngleBracketEntities(node.value);
      decodeMathText(node.data?.hChildren ?? []);
    });
  };
}

/**
 * GFM + line breaks only (no TeX). For surfaces that do not wire rehype-katex
 * (e.g. uploaded document preview).
 */
export const MarkdownRemarkPluginsLite = [remarkGfm, remarkBreaks];

/**
 * Shared Markdown pipeline for assistant-style content:
 * - remark-gfm: GFM tables, task lists, strikethrough, autolinks, etc.
 * - remark-math: TeX ($...$ / $$...$$); pair with rehype-katex on render.
 * - remark-breaks: treat single newlines as hard breaks (common in LLM chat).
 */
export const MarkdownRemarkPlugins = [
  remarkGfm,
  remarkMath,
  remarkDecodeMathEntities,
  remarkBreaks,
];
