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

export interface CsvHighlight {
  /** Record number (header = 1, data row i = i + 2) to its highlighted columns. */
  columnsByRecord: Map<number, Set<number>>;
  /** Index into data.rows the view scrolls to, clamped to the first row. */
  targetRowIndex: number;
}

const SheetIndex = 1;

/**
 * Map chunk positions onto the previewed table, or null when they cannot be
 * applied.
 *
 * The positions were produced by the Go parser at ingest time from its own
 * record numbering; the rows here come from Papa parsing the same bytes in the
 * browser. The two agree for ordinary files but are not guaranteed to — a
 * separator Papa samples differently, an irregular column count Go rejects —
 * so the coordinates are only a hint. Every tuple must be a five-field
 * spreadsheet range on the one sheet a CSV is read as, inside the table this
 * preview actually rendered; one that is not invalidates the whole set,
 * because a partly applied highlight would point at rows chosen by parsers
 * that disagreed. Passing these checks does not prove the parsers agreed — it
 * only proves the coordinates land on real cells.
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
  const usable = positions.every(
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
  if (!usable) return null;

  const columnsByRecord = new Map<number, Set<number>>();
  for (const [, firstRow, lastRow, firstColumn, lastColumn] of positions) {
    for (let record = firstRow; record <= lastRow; record++) {
      const columns = columnsByRecord.get(record) ?? new Set<number>();
      for (let column = firstColumn; column <= lastColumn; column++) {
        columns.add(column);
      }
      columnsByRecord.set(record, columns);
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

/** Highlighted 1-based column numbers of one record, or null for none. */
export const highlightedColumns = (
  highlight: CsvHighlight | null,
  recordNumber: number,
): Set<number> | null => highlight?.columnsByRecord.get(recordNumber) ?? null;
