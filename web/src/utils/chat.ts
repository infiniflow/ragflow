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

import {
  ChatVariableEnabledField,
  EmptyConversationId,
} from '@/constants/chat';
import {
  IMessage,
  IReference,
  IReferenceObject,
  Message,
} from '@/interfaces/database/chat';
import type { CompletionChunk } from '@/services/chat-completion-stream';
import { v4 as uuid } from 'uuid';
import {
  buildCitationDisplayMap,
  citationMarkerReg,
  normalizeCitationDigits,
  parseCitationIndex,
  resolveCitationChunk,
} from './citation-utils';

export const isConversationIdExist = (conversationId: string) => {
  return conversationId !== EmptyConversationId && conversationId !== '';
};

export const buildMessageUuid = (message: Partial<Message | IMessage>) => {
  if ('id' in message && message.id) {
    return message.id;
  }
  return uuid();
};

export const buildMessageListWithUuid = (messages?: Message[]) => {
  return (
    messages?.map((x: Message | IMessage) => ({
      ...x,
      id: buildMessageUuid(x),
    })) ?? []
  );
};

export const buildReferenceObject = (
  reference?: IReference,
): IReferenceObject => ({
  chunks: Object.fromEntries(
    (reference?.chunks ?? []).map((chunk, index) => [
      chunk.citation_id ?? String(index),
      chunk,
    ]),
  ),
  doc_aggs: Object.fromEntries(
    Object.values(reference?.doc_aggs ?? {}).map((document) => [
      document.doc_id,
      document,
    ]),
  ),
});

export const generateConversationId = () => {
  return uuid().replace(/-/g, '');
};

// When rendering each message, add a prefix to the id to ensure uniqueness.
export const buildMessageUuidWithRole = (
  message: Partial<Message | IMessage>,
) => {
  return `${message.role}_${message.id}`;
};

/** Merges streamed deltas, replacing the visible text with a final answer. */
export function mergeAnswerChunk(
  previousAnswer: string,
  chunk: CompletionChunk,
): string {
  const currentAnswer = chunk.answer || '';

  let nextAnswer: string;
  if (chunk.final && currentAnswer) {
    // Final answers can add or repair citations anywhere in the text.
    // Keep streamed thinking when the final payload only contains the answer.
    const thinkEnd = previousAnswer.lastIndexOf('</think>');
    const thinking =
      thinkEnd >= 0 &&
      !currentAnswer.includes('<think>') &&
      !currentAnswer.includes('</think>')
        ? previousAnswer.slice(0, thinkEnd + '</think>'.length) + '\n\n'
        : '';
    nextAnswer = thinking + currentAnswer;
  } else if (previousAnswer && currentAnswer.startsWith(previousAnswer)) {
    nextAnswer = currentAnswer;
  } else {
    nextAnswer = previousAnswer + currentAnswer;
  }

  if (chunk.start_to_think === true) {
    nextAnswer = nextAnswer + '<think>';
  }

  if (chunk.end_to_think === true) {
    // Keep the answer's first Markdown heading on its own line.
    nextAnswer = nextAnswer + '</think>\n\n';
  }

  return nextAnswer;
}

// A math delimiter can immediately follow a letter, as in \(t\). Only
// TeX delimiter-sizing commands consume an escaped bracket or parenthesis.
// Stop at a new opener or a message-section boundary if the closer is missing.
const BLOCK_MATH_RE =
  /\\\[((?:(?!\\\[|<\/?(?:details|think|retrieving)\b)[\s\S])*?)(?<!\\(?:left|middle|right|[bB]igg?[lmr]?))\\\]/g;
const INLINE_MATH_RE =
  /\\\(((?:(?!\\\(|<\/?(?:details|think|retrieving)\b)[\s\S])*?)(?<!\\(?:left|middle|right|[bB]igg?[lmr]?))\\\)/g;
// Fenced code and inline code can show LaTeX delimiters as literal examples.
const MARKDOWN_CODE_RE =
  /^[ \t]*(?<fence>(?<mark>`|~)\k<mark>{2,})(?!\k<mark>)[^\r\n]*\r?\n[\s\S]*?(?:^[ \t]*\k<fence>\k<mark>*[ \t]*(?=\r?$)|(?![\s\S]))|(?<![\\`])(?<ticks>`+)(?!`)(?:[^\r\n]|\r?\n(?![ \t]*\r?\n))*?(?<!`)\k<ticks>(?!`)/gm;

function normalizeLatexDelimiters(text: string) {
  return text
    .replace(/\\\\([()[\]])/g, '\\$1')
    .replace(BLOCK_MATH_RE, (_, equation) => `$$${equation}$$`)
    .replace(INLINE_MATH_RE, (_, equation) => `$${equation}$`);
}

export const preprocessLaTeX = (content: string) => {
  const normalizedContent = content
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');

  const parts: string[] = [];
  let start = 0;
  for (const match of normalizedContent.matchAll(MARKDOWN_CODE_RE)) {
    parts.push(
      normalizeLatexDelimiters(normalizedContent.slice(start, match.index)),
    );
    parts.push(match[0]);
    start = match.index + match[0].length;
  }
  parts.push(normalizeLatexDelimiters(normalizedContent.slice(start)));
  return parts.join('');
};

export function replaceThinkToSection(
  text: string = '',
  summary: string = 'Thinking...',
) {
  const pattern = /<think>([\s\S]*?)<\/think>/g;

  // An empty think section (the model replied without reasoning) must not
  // render as a bare "Thinking..." strip above the answer.
  const result = text.replace(pattern, (_match, thinkContent: string) =>
    thinkContent.trim().length === 0
      ? ''
      : `<details class="think"><summary>${summary}</summary>${thinkContent}</details>`,
  );

  return result;
}

// Strip <think> reasoning blocks so only the answer text remains.
// The second replace handles a streaming message whose block is still unclosed.
export function removeThinkSection(text: string = '') {
  return text
    .replace(/<think>[\s\S]*?<\/think>/g, '')
    .replace(/<think>[\s\S]*$/, '')
    .trim();
}

export function replaceRetrievingToSection(text: string = '') {
  const pattern = /<retrieving>([\s\S]*?)<\/retrieving>/g;

  const result = text.replace(
    pattern,
    '<details class="retrieving"><summary>Retrieving...</summary>$1</details>',
  );

  return result;
}

// Placeholder markers used internally to protect standalone < and > from
// DOMPurify stripping. These Unicode symbols (U+27E8/U+27E9) are extremely
// unlikely to appear in normal user input.
const LT_MARKER = '\u27E8LT\u27E9';
const GT_MARKER = '\u27E8GT\u27E9';

/**
 * Escape standalone < and > that are NOT part of a matched <...> pair,
 * so that DOMPurify won't strip them as HTML tags.
 * Only brackets inside text segments (outside complete tags) are escaped;
 * matched <...> tags are left intact for DOMPurify to handle.
 */
export function escapeUnmatchedAngleBrackets(content: string): string {
  if (!content) return content;

  const segments: string[] = [];
  const tags: string[] = [];
  let lastIndex = 0;

  const regex = /<[^>]*>/g;
  let match: RegExpExecArray | null;

  while ((match = regex.exec(content)) !== null) {
    segments.push(content.slice(lastIndex, match.index));
    tags.push(match[0]);
    lastIndex = regex.lastIndex;
  }
  segments.push(content.slice(lastIndex));

  const escapedSegments = segments.map((seg) =>
    seg.replace(/</g, LT_MARKER).replace(/>/g, GT_MARKER),
  );

  return escapedSegments
    .map((seg, i) => (i < tags.length ? seg + tags[i] : seg))
    .join('');
}

/**
 * Restore escaped angle bracket markers back to HTML entities (&lt;/&gt;).
 * Must be called *after* preprocessLaTeX (which would otherwise convert
 * &lt;/&gt; back to raw <, >).
 */
export function unescapeAngleBrackets(content: string): string {
  if (!content) return content;
  return content
    .replace(new RegExp(LT_MARKER, 'g'), '&lt;')
    .replace(new RegExp(GT_MARKER, 'g'), '&gt;');
}

/**
 * Decode the &lt;/&gt; entities restored by unescapeAngleBrackets.
 * react-markdown decodes character references in prose, but code blocks and
 * inline code are literal, so the entities would render as-is there. Code
 * renderers call this to show the original < and > the model emitted.
 */
export function decodeAngleBracketEntities(content: string): string {
  if (!content) return content;
  return content.replace(/&lt;/g, '<').replace(/&gt;/g, '>');
}

export function setInitialChatVariableEnabledFieldValue(
  field: ChatVariableEnabledField,
) {
  return field !== ChatVariableEnabledField.MaxTokensEnabled;
}

const ShowImageFields = ['image', 'table'];

export function showImage(filed?: string) {
  return ShowImageFields.some((x) => x === filed);
}

export function setChatVariableEnabledFieldValuePage() {
  const variableCheckBoxFieldMap = Object.values(
    ChatVariableEnabledField,
  ).reduce<Record<string, boolean>>((pre, cur) => {
    pre[cur] = cur !== ChatVariableEnabledField.MaxTokensEnabled;
    return pre;
  }, {});

  return variableCheckBoxFieldMap;
}

const oldReg = /(#{2}[0-9\u0660-\u0669\u06F0-\u06F9]+\${2})/g;
export const currentReg = citationMarkerReg;
export {
  buildCitationDisplayMap,
  normalizeCitationDigits,
  parseCitationIndex,
  resolveCitationChunk,
};
export type { CitationKey } from './citation-utils';

// To be compatible with the old index matching mode
export const replaceTextByOldReg = (text: string) => {
  return text?.replace(oldReg, (substring: string) => {
    return `[ID:${substring.slice(2, -2)}]`;
  });
};
