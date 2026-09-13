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
// Type-only: both names are used in annotations only, and a value import here
// makes the Babel module transform fail on this file ("imported binding used in
// a type annotation"), which breaks every suite that imports it.
import type { IMessage, Message } from '@/interfaces/database/chat';
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

/**
 * Stage banners the Agentic RAG pipeline forwards into the answer stream
 * (`rag/advanced_rag/think_log.py` forwards every INFO record starting with
 * `[`). Matched by prefix only, so new stages need no frontend change beyond
 * adding the tag here.
 */
export const AGENTIC_LOG_PREFIXES = [
  '[Agentic RAG]',
  '[Formalize',
  '[Keywords',
  '[Direct search',
  '[Hybrid search',
  '[Memory',
  '[Composing the answer]',
] as const;

/**
 * Progress chatter the tool loop prints around a call without a stage tag
 * ("Running the rag tool...", "Running tool..."). Anchored to the whole line so
 * a real sentence that merely starts with those words is never swallowed.
 */
const AGENTIC_PREAMBLE_RE =
  /^running\s+(?:the\s+)?(?:[\w-]+\s+)?tools?(?:\s*[.…]{1,3})?$/i;

/**
 * Anything that renders a citation, figure or image in the answer. A line like
 * this is never treated as a log even when it carries a stage tag: silently
 * hiding an image, a `Fig. N` reference or an `[ID:n]` citation would damage the
 * answer, whereas leaving a log line in the body is only cosmetic.
 */
const ANSWER_MEDIA_RE =
  /!\[|<img|<figure|<image|\[\s*ID:\s*\d+\s*\]|\bFig(?:ure)?\.?\s*\d/i;

// Fenced code blocks must never be rewritten: a shell snippet or a log sample
// can legitimately start a line with one of the prefixes above.
const CODE_FENCE_RE = /^\s*(```|~~~)/;
// Inline code spans and existing TeX must be left untouched by the caret pass.
const CODE_OR_MATH_SEGMENT_RE = /(`[^`]*`|\$\$[\s\S]*?\$\$|\$[^$\n]*\$)/g;
// `1.5mm^2`, `10^-6`, `m^{3}`: a unit/number exponent typed as plain text.
const BARE_CARET_EXPONENT_RE =
  /([0-9A-Za-z)\]])\^(\{[^}\s]{1,12}\}|[+-]?[0-9]{1,3}|[A-Za-z])/g;
// One think line arrives as Go's `…<br>` (trailing break) or Python's
// `<br>…\n` (leading break), so a log line can only be recognised after
// splitting the physical line on the break tags as well.
const BREAK_TAG_RE = /<br\s*\/?>/gi;
// Placeholder marking where the collapsed log panel belongs; substituted once
// the final line count is known.
const LOG_BLOCK_SENTINEL = '@@agentic-log-block@@';

/** Splits text into physical lines, then each line on `<br>` boundaries. */
const splitLogLines = (text: string = ''): string[] =>
  text
    .split(/\r?\n/)
    .flatMap((line) => line.split(BREAK_TAG_RE))
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0);

/** True when a line is untagged tool-progress chatter. */
export function isAgenticPreambleLine(line: string = ''): boolean {
  return AGENTIC_PREAMBLE_RE.test(line.trim());
}

/**
 * True when a line belongs in the collapsed progress panel. Answer-bearing
 * lines (figures, images, citations) are excluded first so extraction can never
 * remove content the user is meant to read.
 */
export function isAgenticLogLine(line: string = ''): boolean {
  const trimmed = line.trim().replace(/^[-*+]\s+/, '');

  if (ANSWER_MEDIA_RE.test(trimmed)) {
    return false;
  }

  return (
    AGENTIC_LOG_PREFIXES.some((prefix) => trimmed.startsWith(prefix)) ||
    isAgenticPreambleLine(trimmed)
  );
}

/** True when the first line of a block is an Agentic RAG log line. */
export function isAgenticLogText(text: string = ''): boolean {
  const firstLine = splitLogLines(text)[0];

  return firstLine !== undefined && isAgenticLogLine(firstLine);
}

/** Number of Agentic RAG log lines in a block of text. */
export function countAgenticLogLines(text: string = ''): number {
  return splitLogLines(text).filter((line) => isAgenticLogLine(line)).length;
}

const fillLogCount = (summary: string, count: number) =>
  summary.replace(/\{\{\s*num\s*\}\}/g, String(count));

// The leading stage tag is rendered as inline code: it keeps the tag visually
// distinct (and stops markdown from reading `[Tag]` as a link reference).
const stripTagToCode = (line: string) =>
  line.replace(/^(\[[^\]]{1,60}\])(\S?.*)$/, '`$1`$2');

const buildAgenticLogBlock = (summary: string, logs: string[]) =>
  [
    `<details class="agentic-log"><summary>${fillLogCount(
      summary,
      logs.length,
    )}</summary>`,
    '',
    // The blank lines are load-bearing: markdown nested in a raw HTML block is
    // only parsed once the block is interrupted, so the log lines below stay
    // real markdown (bold, code fences, links) instead of one unformatted blob.
    ...logs.map(stripTagToCode),
    '',
    '</details>',
  ].join('\n');

/**
 * Removes the whitespace and empty markup that extraction leaves at the edges of
 * the answer, so the body starts on real content instead of stray line breaks.
 */
export function trimExtractionResidue(text: string = '') {
  const LEADING = /^(?:\s|&nbsp;|<br\s*\/?>|<p>\s*<\/p>|<p><\/p>)+/i;
  const TRAILING = /(?:\s|&nbsp;|<br\s*\/?>|<p>\s*<\/p>|<p><\/p>)+$/i;

  return text.replace(LEADING, '').replace(TRAILING, '');
}

export function replaceThinkToSection(
  text: string = '',
  summary: string = 'Thinking...',
  logSummary?: string,
) {
  // The closing tag is optional on purpose: while an answer streams the block
  // is still open, and leaving it unhandled would print the raw reasoning and
  // pipeline logs into the answer until the closer arrives.
  const pattern = /<think>([\s\S]*?)(?:<\/think>|$)/g;

  const result = text.replace(pattern, (_match, thinkContent: string) => {
    const body = thinkContent.trim();
    if (body.length === 0) {
      return '';
    }
    // Agentic RAG progress logs reach the UI wrapped in <think> markers. They
    // are diagnostics, not reasoning, so they get their own summary and the
    // monospaced log list instead of the generic "Thought" panel.
    if (logSummary && isAgenticLogText(body)) {
      return buildAgenticLogBlock(logSummary, splitLogLines(body));
    }
    // Same blank-line rule as the log panel: without it the reasoning body is
    // treated as raw HTML and its markdown is never rendered.
    return `<details class="think"><summary>${summary}</summary>\n\n${body}\n\n</details>`;
  });

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

export function replaceRetrievingToSection(
  text: string = '',
  summary: string = 'Retrieving...',
) {
  const pattern = /<retrieving>([\s\S]*?)<\/retrieving>/g;

  const result = text.replace(
    pattern,
    (_match, retrievingContent: string) =>
      `<details class="retrieving"><summary>${summary}</summary>\n\n${retrievingContent.trim()}\n\n</details>`,
  );

  return result;
}

/**
 * Collapses bare Agentic RAG progress lines into one collapsed `<details>`
 * block, placed where the first line appeared, so the answer body only keeps
 * the parts the user should read. Lines inside fenced code blocks are kept, and
 * a line that mixes a log with real content keeps the content.
 */
export function replaceAgenticLogsToSection(
  text: string = '',
  summary: string = 'Agentic RAG log',
) {
  if (!text || !text.includes('[')) {
    return text;
  }

  const kept: string[] = [];
  const logs: string[] = [];
  let inserted = false;
  let inFence = false;

  text.split(/\r?\n/).forEach((line) => {
    if (CODE_FENCE_RE.test(line)) {
      inFence = !inFence;
      kept.push(line);
      return;
    }
    if (inFence) {
      kept.push(line);
      return;
    }

    const remaining: string[] = [];
    let sawLog = false;
    for (const segment of line.split(BREAK_TAG_RE)) {
      if (isAgenticLogLine(segment)) {
        sawLog = true;
        logs.push(segment.trim());
        continue;
      }
      remaining.push(segment);
    }

    if (!sawLog) {
      kept.push(line);
      return;
    }
    if (!inserted) {
      inserted = true;
      kept.push(LOG_BLOCK_SENTINEL);
    }
    // A pure log line leaves nothing behind; a mixed line keeps its text, with
    // the break tags that surrounded the removed segments trimmed away.
    const rest = remaining
      .join('<br>')
      .replace(/^(?:<br\s*\/?>|\s)+|(?:<br\s*\/?>|\s)+$/gi, '');
    if (rest.length > 0) {
      kept.push(rest);
    }
  });

  if (logs.length === 0) {
    return text;
  }

  return trimExtractionResidue(
    kept
      .join('\n')
      .replace(LOG_BLOCK_SENTINEL, buildAgenticLogBlock(summary, logs))
      .replace(/\n{3,}/g, '\n\n'),
  );
}

/**
 * Promotes caret exponents typed as plain text (`1.5mm^2`, `10^-6`) into inline
 * TeX so KaTeX renders real superscripts. Fenced code, inline code spans and
 * existing `$…$` math are left alone, which keeps this safe for both user
 * questions and model answers.
 */
export function promoteCaretExponentsToLaTeX(text: string = '') {
  if (!text || !text.includes('^')) {
    return text;
  }

  let inFence = false;

  return text
    .split('\n')
    .map((line) => {
      if (CODE_FENCE_RE.test(line)) {
        inFence = !inFence;
        return line;
      }
      if (inFence) {
        return line;
      }
      return line
        .split(CODE_OR_MATH_SEGMENT_RE)
        .map((segment, index) =>
          // Odd indices are the captured code spans / math segments.
          index % 2 === 1
            ? segment
            : segment.replace(
                BARE_CARET_EXPONENT_RE,
                (_match, base: string, exponent: string) => {
                  // `m^{3}` already carries its own braces; `mm^2` does not.
                  const value =
                    exponent.startsWith('{') && exponent.endsWith('}')
                      ? exponent.slice(1, -1)
                      : exponent;

                  return `${base}$^{${value}}$`;
                },
              ),
        )
        .join('');
    })
    .join('\n');
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
