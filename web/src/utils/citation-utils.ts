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

export const parseCitationIndex = (value: string): CitationKey => {
  const normalized = normalizeCitationDigits(value);
  const markerMatch = normalized.match(
    /\[(?:ID:)?([0-9a-fA-F\u0660-\u0669\u06F0-\u06F9]+)\]/,
  );
  if (markerMatch) {
    const captured = markerMatch[1];
    // Explicit [ID:...] markers are opaque chunk ids, including all-digit
    // hexadecimal ids that would lose leading zeros or precision as numbers.
    if (/^\s*\[?ID:/i.test(normalized)) return captured;
    return /^\d+$/.test(captured) ? Number(captured) : captured;
  }
  if (/^\d+$/.test(normalized)) return Number(normalized);
  if (/^[0-9a-fA-F]+$/.test(normalized)) return normalized;
  return Number.NaN;
};
export const citationMarkerReg =
  /\[(?:ID:)?([0-9a-fA-F\u0660-\u0669\u06F0-\u06F9]+)\]/g;

// Map each unique citation marker ([n] or [ID:<chunkId>]) to its stable
// 0-based display position by first occurrence. The chip renders position + 1
// and the chunk is resolved from the marker key (see findCitationChunk).
// Works for both plain chat (numeric markers) and agents ([ID:<chunkId>]).
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

// findCitationChunk resolves a citation marker to its reference chunk. Agent
// markers carry chunk ids ("[ID:9ac9b07178c3951a]"), so match by id; fall
// back to the marker's position for references keyed by display index.
export const findCitationChunk = <T extends { id?: string | number }>(
  chunks: T[] | Record<string, T> | undefined,
  key: CitationKey,
  position: number,
): T | undefined => {
  if (!chunks) return undefined;
  if (!Array.isArray(chunks)) {
    const direct = chunks[String(key)];
    if (direct) return direct;
  }
  const list = Array.isArray(chunks) ? chunks : Object.values(chunks);
  return (
    list.find((chunk) => String(chunk?.id) === String(key)) ?? list[position]
  );
};
