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

import { maxColumnCount, parseCSV } from './csv-parse';

// Expected values come from the Go parser (internal/parser/parser/csv_parser.go)
// run over the same bytes: records after reading with the detected separator,
// with blank lines skipped and the first record taken as the header.
describe('parseCSV matches the Go parser record for record', () => {
  it('drops blank lines before, between and after records', () => {
    expect(parseCSV('\n\na,b\n1,2\n')).toEqual({
      headers: ['a', 'b'],
      rows: [['1', '2']],
    });
    expect(parseCSV('a,b\n\n1,2\n\n')).toEqual({
      headers: ['a', 'b'],
      rows: [['1', '2']],
    });
    expect(parseCSV('a,b\n\n')).toEqual({ headers: ['a', 'b'], rows: [] });
    expect(parseCSV('a,b\r\n\r\n1,2\r\n')).toEqual({
      headers: ['a', 'b'],
      rows: [['1', '2']],
    });
  });

  it('keeps records that merely look empty', () => {
    // A quoted empty field and an all-empty field list are real records; Go
    // yields them too, so they must keep their row number.
    expect(parseCSV('a\n""\nb\n')).toEqual({
      headers: ['a'],
      rows: [[''], ['b']],
    });
    expect(parseCSV('a,b\n,\nc,d\n')).toEqual({
      headers: ['a', 'b'],
      rows: [
        ['', ''],
        ['c', 'd'],
      ],
    });
  });

  it.each([
    ['comma', 'a,b\n1,2\n', ['a', 'b'], ['1', '2']],
    ['semicolon', 'a;b\n1;2\n', ['a', 'b'], ['1', '2']],
    ['tab', 'a\tb\n1\t2\n', ['a', 'b'], ['1', '2']],
    ['pipe', 'a|b\n1|2\n', ['a', 'b'], ['1', '2']],
  ])('detects the %s separator', (_, text, headers, row) => {
    expect(parseCSV(text)).toEqual({ headers, rows: [row] });
  });

  it('keeps a separator inside a quoted field out of the split', () => {
    expect(parseCSV('a,b\n"x,y",2\n')).toEqual({
      headers: ['a', 'b'],
      rows: [['x,y', '2']],
    });
  });

  it('keeps a newline inside a quoted field in the same record', () => {
    expect(parseCSV('a,b\n"x\ny",2\n')).toEqual({
      headers: ['a', 'b'],
      rows: [['x\ny', '2']],
    });
  });

  it('treats a file without a trailing newline like one with it', () => {
    expect(parseCSV('a,b\n1,2')).toEqual(parseCSV('a,b\n1,2\n'));
  });

  it('returns no records for an empty file', () => {
    expect(parseCSV('')).toEqual({ headers: [], rows: [] });
  });

  it('returns no records for a file with only whitespace', () => {
    // Go returns before reading any record when the text has no content.
    expect(parseCSV('\n\n  \r\n\t\n')).toEqual({ headers: [], rows: [] });
  });

  it('skips whitespace-only lines when sampling the separator', () => {
    // The probe counts `  ` as blank the way Go's isBlankCSVRow does; with
    // Papa's default rule the leading blanks would fill the ten-record sample
    // and the separator would fall back to comma.
    expect(parseCSV('\n  \n35a;YfYb\n cX9;X124\n"quoted 53";b9d5\n')).toEqual({
      headers: ['  '],
      rows: [
        ['35a', 'YfYb'],
        [' cX9', 'X124'],
        ['quoted 53', 'b9d5'],
      ],
    });
  });

  it('keeps whitespace-only lines, which Go also reads as a record', () => {
    // Go only skips a line that is nothing but its terminator; a line of
    // spaces becomes an empty field. The field's text differs (Go trims it),
    // but it occupies the same record number and so cannot shift a highlight.
    expect(parseCSV('a,b\n  \n1,2\n')).toEqual({
      headers: ['a', 'b'],
      rows: [['  '], ['1', '2']],
    });
  });
});

describe('maxColumnCount', () => {
  it('takes the widest row, header included', () => {
    expect(
      maxColumnCount({ headers: ['a', 'b'], rows: [['1'], ['1', '2', '3']] }),
    ).toBe(3);
    expect(maxColumnCount({ headers: ['a', 'b', 'c'], rows: [['1']] })).toBe(3);
  });

  it('handles a table with no rows and no header', () => {
    expect(maxColumnCount({ headers: [], rows: [] })).toBe(0);
  });

  it('handles a row count past the call-stack argument limit', () => {
    // Math.max(...lengths) throws RangeError at this size.
    const rows = Array.from({ length: 200_000 }, () => ['a', 'b', 'c']);
    expect(maxColumnCount({ headers: ['h'], rows })).toBe(3);
  });
});
