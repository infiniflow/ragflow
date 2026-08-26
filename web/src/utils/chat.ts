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
import { IMessage, Message } from '@/interfaces/database/chat';
import { omit } from 'lodash';
import { v4 as uuid } from 'uuid';
import {
  citationMarkerReg,
  normalizeCitationDigits,
  parseCitationIndex,
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
      ...omit(x, 'reference'),
      id: buildMessageUuid(x),
    })) ?? []
  );
};

export const generateConversationId = () => {
  return uuid().replace(/-/g, '');
};

// When rendering each message, add a prefix to the id to ensure uniqueness.
export const buildMessageUuidWithRole = (
  message: Partial<Message | IMessage>,
) => {
  return `${message.role}_${message.id}`;
};

// Preprocess LaTeX equations to be rendered by KaTeX
// ref: https://github.com/remarkjs/react-markdown/issues/785
//
// Delimiter matching: we only treat \] and \) as block/inline endings when they
// are not part of a LaTeX command (e.g. \right], \big), \left)). Use a negative
// lookbehind (?<![a-zA-Z]) so that \] or \) preceded by a letter (command name)
// is not considered the closing delimiter. Use greedy matching so we match up to
// the last valid delimiter and avoid cutting at the first \] or \) inside the
// equation (e.g. \frac{1}{|y|} or \right]).

const BLOCK_MATH_RE = /\\\[([\s\S]*?)(?<![a-zA-Z])\\\]/g;
const INLINE_MATH_RE = /\\\(([\s\S]*?)(?<![a-zA-Z])\\\)/g;

export const preprocessLaTeX = (content: string) => {
  const normalizedContent = content
    .replace(/\\\\\[/g, '\\[')
    .replace(/\\\\\(/g, '\\(')
    .replace(/\\\\\]/g, '\\]')
    .replace(/\\\\\)/g, '\\)')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');

  const blockProcessedContent = normalizedContent.replace(
    BLOCK_MATH_RE,
    (_, equation) => `$$${equation}$$`,
  );

  const inlineProcessedContent = blockProcessedContent.replace(
    INLINE_MATH_RE,
    (_, equation) => `$${equation}$`,
  );

  return inlineProcessedContent;
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
export { normalizeCitationDigits, parseCitationIndex };

// To be compatible with the old index matching mode
export const replaceTextByOldReg = (text: string) => {
  return text?.replace(oldReg, (substring: string) => {
    return `[ID:${substring.slice(2, -2)}]`;
  });
};
