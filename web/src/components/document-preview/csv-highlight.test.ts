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

import { highlightedColumns, resolveCsvHighlight } from './csv-highlight';
import { parseCSV } from './csv-parse';

const table = parseCSV('h1,h2,h3\nr1a,r1b,r1c\nr2a,r2b,r2c\nr3a,r3b,r3c\n');

const columnsAt = (
  highlight: ReturnType<typeof resolveCsvHighlight>,
  recordNumber: number,
) => [...(highlightedColumns(highlight, recordNumber) ?? [])].sort();

describe('resolveCsvHighlight', () => {
  it('does not highlight without positions', () => {
    expect(resolveCsvHighlight(table)).toBeNull();
    expect(resolveCsvHighlight(table, [])).toBeNull();
    expect(highlightedColumns(null, 2)).toBeNull();
  });

  it('maps record numbers to rows: header is 1, data row i is i + 2', () => {
    const highlight = resolveCsvHighlight(table, [[1, 2, 2, 1, 2]]);
    expect(highlight).not.toBeNull();
    expect(columnsAt(highlight, 1)).toEqual([]);
    expect(columnsAt(highlight, 2)).toEqual([1, 2]);
    expect(columnsAt(highlight, 3)).toEqual([]);
    expect(highlight?.targetRowIndex).toBe(0);
  });

  it('treats the column range as closed on both ends', () => {
    const highlight = resolveCsvHighlight(table, [[1, 3, 3, 2, 2]]);
    expect(columnsAt(highlight, 3)).toEqual([2]);
  });

  it('highlights the header when a position covers record 1', () => {
    const highlight = resolveCsvHighlight(table, [[1, 1, 1, 1, 3]]);
    expect(columnsAt(highlight, 1)).toEqual([1, 2, 3]);
    // Record 1 is the header, which is not a data row: stay at the top.
    expect(highlight?.targetRowIndex).toBe(0);
  });

  it('scrolls to the first record of the position set', () => {
    const highlight = resolveCsvHighlight(table, [
      [1, 3, 3, 1, 1],
      [1, 4, 4, 2, 2],
    ]);
    expect(highlight?.targetRowIndex).toBe(1);
  });

  it('merges several ranges that cover the same record', () => {
    const highlight = resolveCsvHighlight(table, [
      [1, 2, 2, 1, 1],
      [1, 2, 2, 3, 3],
    ]);
    expect(columnsAt(highlight, 2)).toEqual([1, 3]);
  });

  it('highlights the whole closed row range across several records', () => {
    const highlight = resolveCsvHighlight(table, [[1, 2, 4, 1, 1]]);
    expect(columnsAt(highlight, 2)).toEqual([1]);
    expect(columnsAt(highlight, 3)).toEqual([1]);
    expect(columnsAt(highlight, 4)).toEqual([1]);
  });

  describe('rejects the whole set when one tuple cannot be applied', () => {
    const cases: [string, number[][]][] = [
      [
        'a row past the last record',
        [
          [1, 2, 2, 1, 1],
          [1, 9, 9, 1, 1],
        ],
      ],
      ['a column past the last column', [[1, 2, 2, 1, 4]]],
      ['a row range that runs backwards', [[1, 3, 2, 1, 1]]],
      ['a column range that runs backwards', [[1, 2, 2, 2, 1]]],
      ['a zero row', [[1, 0, 0, 1, 1]]],
      ['a zero column', [[1, 2, 2, 0, 1]]],
      ['a fractional coordinate', [[1, 2, 2, 1, 1.5]]],
      ['a sheet other than the one CSV is read as', [[2, 2, 2, 1, 1]]],
      ['a tuple that is not a cell range', [[2, 2, 2]]],
    ];

    it.each(cases)('%s', (_, positions) => {
      expect(resolveCsvHighlight(table, positions)).toBeNull();
    });
  });

  it('accepts a range that stops at the last record and column', () => {
    const highlight = resolveCsvHighlight(table, [[1, 4, 4, 3, 3]]);
    expect(columnsAt(highlight, 4)).toEqual([3]);
  });

  it('does not scroll past the table', () => {
    const highlight = resolveCsvHighlight(table, [[1, 4, 4, 1, 1]]);
    expect(highlight?.targetRowIndex).toBe(2);
  });

  it('cannot tell that the parsers disagreed when the coordinates still fit', () => {
    // An irregular column count makes Papa and the Go parser pick different
    // separators: Go reads one column per row, Papa two. The tuple is still
    // inside this table, so the bounds check passes and the highlight lands on
    // the wrong row's second column. This is where the guarantee ends — the
    // checks reject coordinates that cannot be applied, they do not prove the
    // two parsers agreed. Kept as a test so the limit stays visible.
    const raggedTable = parseCSV('a;b\nc,d\n1;2\n');
    const highlight = resolveCsvHighlight(raggedTable, [[1, 2, 2, 2, 2]]);
    expect(highlight).not.toBeNull();
    expect(columnsAt(highlight, 2)).toEqual([2]);
  });
});
