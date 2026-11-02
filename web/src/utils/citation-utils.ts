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

import type { IReferenceChunk } from '@/interfaces/database/chat';

export const normalizeCitationDigits = (text: string) => {
  if (!text) return text;
  return text.replace(/[٠-٩۰-۹]/g, (char) => {
    const code = char.charCodeAt(0);
    if (code >= 0x0660 && code <= 0x0669) {
      return String.fromCharCode(code - 0x0660 + 0x30);
    }
    if (code >= 0x06f0 && code <= 0x06f9) {
      return String.fromCharCode(code - 0x06f0 + 0x30);
    }
    return char;
  });
};

export type CitationKey = number | string;

// Knowledge-base citation markers are positional indices produced by
// strconv.Itoa, so they stay numeric and small. Sixteen-digit hex agent IDs
// exceed Number.MAX_SAFE_INTEGER and must keep their string form; leading-zero
// values are opaque ids for the same reason.
const maxNumericCitationIndexDigits = 15;

const isNumericCitationIndex = (value: string) =>
  /^\d+$/.test(value) &&
  value.length <= maxNumericCitationIndexDigits &&
  (value.length === 1 || value[0] !== '0');

export const parseCitationIndex = (value: string): CitationKey => {
  const normalized = normalizeCitationDigits(value);
  const markerMatch = normalized.match(
    /\[(?:ID:)?([0-9a-fA-F\u0660-\u0669\u06F0-\u06F9]+)\]/,
  );
  if (markerMatch) {
    const captured = markerMatch[1];
    if (/^\s*\[?ID:/i.test(normalized)) {
      return isNumericCitationIndex(captured) ? Number(captured) : captured;
    }
    return /^\d+$/.test(captured) ? Number(captured) : captured;
  }
  if (/^\d+$/.test(normalized)) return Number(normalized);
  if (/^[0-9a-fA-F]+$/.test(normalized)) return normalized;
  return Number.NaN;
};
export const citationMarkerReg =
  /\[(?:ID:)?([0-9a-fA-F\u0660-\u0669\u06F0-\u06F9]+)\]/g;

// react-string-replace hands the callback the last capture group, not the whole
// match. citationMarkerReg groups only the id, so feeding it to
// reactStringReplace would strip the brackets and turn an opaque all-digit id
// into a positional index. This expression captures the whole marker instead;
// the renderer can then pass it to parseCitationIndex unchanged.
export const citationWholeMarkerReg =
  /(\[(?:ID:)?[0-9a-fA-F\u0660-\u0669\u06F0-\u06F9]+\])/g;

// Resolve the chunk a citation marker points at. A numeric key is the
// chunk's position in the payload — the plain-chat contract, where the
// backend numbers each marker by its index into reference.chunks. A string
// key is the chunk id an agent answer embeds in its [ID:<chunkId>] markers;
// match it against the chunk's own id, never positionally: the payload
// ships in retrieval order, which is unrelated to the order the answer
// cites passages in, so a positional read permutes the sources. A 16-char
// hex chunk id stays a string even when all its characters are digits;
// parseCitationIndex only treats short decimal markers as positions.
export const resolveCitationChunk = (
  chunks:
    | IReferenceChunk[]
    | Record<string, IReferenceChunk>
    | null
    | undefined,
  key: CitationKey,
): IReferenceChunk | undefined => {
  const list = Array.isArray(chunks) ? chunks : Object.values(chunks ?? {});
  if (typeof key === 'number') {
    return list[key];
  }
  return list.find(
    (chunk) =>
      chunk?.id === key ||
      (chunk as { chunk_id?: string } | undefined)?.chunk_id === key,
  );
};

// Map each unique citation marker ([n] or [ID:<chunkId>]) to its stable
// 0-based display position by first occurrence; the chip renders
// position + 1. This numbering is display-only — the chunk behind a marker
// is resolved by resolveCitationChunk, never by this position.
export const buildCitationDisplayMap = (
  content: string,
): Map<CitationKey, number> => {
  const map = new Map<CitationKey, number>();
  const reg = new RegExp(citationMarkerReg.source, citationMarkerReg.flags);
  let match: RegExpExecArray | null;
  while ((match = reg.exec(content ?? '')) !== null) {
    const key = parseCitationIndex(match[0]);
    if (Number.isNaN(key)) continue;
    if (!map.has(key)) {
      map.set(key, map.size);
    }
  }
  return map;
};
