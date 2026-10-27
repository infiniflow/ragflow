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

import { CSVData, maxColumnCount } from './csv-parse';

/** Closed interval of 1-based column numbers, inclusive on both ends. */
type ColumnSpan = { start: number; end: number };

export interface CsvHighlight {
  /** Record number (header = 1, data row i = i + 2) to its highlighted columns. */
  columnsByRecord: Map<number, ColumnSpan[]>;
  /** Index into data.rows the view scrolls to, clamped to the first row. */
  targetRowIndex: number;
}

const SheetIndex = 1;

/**
 * True when the whole position set can be trusted as spreadsheet coordinates.
 *
 * Every tuple must be a five-field spreadsheet range on the one sheet a CSV is
 * read as, inside the table the preview actually rendered. A single bad tuple
 * invalidates the whole set: a partly applied highlight would point at rows
 * picked by mismatched parsers, which is worse than no highlight.
 */
const isUsablePositionSet = (
  positions: number[][],
  lastRecordNumber: number,
  columnCount: number,
): boolean =>
  positions.every(
    (pos) =>
      Array.isArray(pos) &&
      pos.length === 5 &&
      pos.every((value) => Number.isInteger(value) && value > 0) &&
      pos[0] === SheetIndex &&
      pos[1] <= pos[2] &&
      pos[3] <= pos[4] &&
      pos[2] <= lastRecordNumber &&
      pos[4] <= columnCount,
  );

/**
 * Map chunk positions onto the previewed table, or null when they cannot be
 * applied.
 *
 * The positions were produced by the Go parser at ingest time from its own
 * record numbering; the rows here come from Papa parsing the same bytes in the
 * browser. The two agree for ordinary files but are not guaranteed to — a
 * separator Papa samples differently, a line Go accepts and Papa does not —
 * so the coordinates are only a hint: they are range-checked against the
 * rendered table, and any mismatch drops the highlight rather than painting
 * the wrong cells.
 *
 * The columns use closed intervals: with [sheet, 2, 3, 1, 2] both records 2
 * and 3 highlight columns 1 through 2.
 */
export const resolveCsvHighlight = (
  data: CSVData,
  positions?: number[][],
): CsvHighlight | null => {
  if (!positions?.length) return null;

  const lastRecordNumber = data.rows.length + 1;
  const columnCount = maxColumnCount(data);
  if (!isUsablePositionSet(positions, lastRecordNumber, columnCount)) {
    return null;
  }

  const columnsByRecord = new Map<number, ColumnSpan[]>();
  for (const [, firstRow, lastRow, firstColumn, lastColumn] of positions) {
    const span: ColumnSpan = { start: firstColumn, end: lastColumn };
    for (let record = firstRow; record <= lastRow; record++) {
      const spans = columnsByRecord.get(record);
      if (spans) {
        spans.push(span);
      } else {
        columnsByRecord.set(record, [span]);
      }
    }
  }

  // Record 1 is the header and row i + 2 is data row i, so a position on the
  // header (or on a row Papa no longer sees) has no data row to scroll to and
  // the view stays at the top with the header highlighted.
  return {
    columnsByRecord,
    targetRowIndex: Math.max(0, positions[0][1] - 2),
  };
};

/**
 * Highlighted column numbers of one record, as a set for the renderer.
 * Records outside the chunk — and a table with no usable positions at all —
 * come back null.
 */
export const highlightedColumns = (
  highlight: CsvHighlight | null,
  recordNumber: number,
): Set<number> | null => {
  const spans = highlight?.columnsByRecord.get(recordNumber);
  if (!spans) return null;
  const columns = new Set<number>();
  for (const span of spans) {
    for (let column = span.start; column <= span.end; column++) {
      columns.add(column);
    }
  }
  return columns;
};
