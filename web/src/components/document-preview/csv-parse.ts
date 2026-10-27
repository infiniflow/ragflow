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

import Papa from 'papaparse';

export interface CSVData {
  /** First remaining record — the heading row. */
  headers: string[];
  /** Every other record, in file order. */
  rows: string[][];
}

/**
 * Candidate separators, in the order the Go parser tries them
 * (csvDelimiters in internal/parser/parser/csv_parser.go).
 */
const DelimitersToGuess = [',', ';', '\t', '|'];

/**
 * Blank lines before the first record. They are stripped before delimiter
 * detection because Papa samples only the first ten records of the file.
 */
const LeadingBlankLines = /^(?:\r\n|\n)+/;

/**
 * A record that is nothing but its line terminator, or the empty record a
 * trailing terminator makes Papa emit. Go's csv.Reader skips blank lines and
 * never yields either one.
 */
const isBlankRecord = (text: string, start: number, end: number): boolean => {
  for (let i = start; i < end; i++) {
    const ch = text[i];
    if (ch !== '\n' && ch !== '\r') return false;
  }
  return true;
};

/**
 * Pick the separator the same way the Go parser does: the candidate that
 * splits the sample into the most columns while keeping the row count steady.
 *
 * Only the detected separator is read back, so the parse stops after ten
 * records — Papa's own delimiter guess samples no more than that either.
 *
 * The sample skips blank lines on both ends of the comparison:
 *   - Leading blank lines are stripped because they consume the ten-record
 *     window without contributing a field count, and enough of them leave the
 *     probe with nothing to measure.
 *   - The probe uses the "greedy" rule for the rest, where a line of nothing
 *     but whitespace counts as blank. That is Go's isBlankCSVRow; Papa's
 *     default rule treats `  ` as a one-column record and would let a single
 *     such line pull the sample towards a wrong separator.
 *
 * The probe is not the Go detector. Papa's internal probe only samples the
 * first ten records and prefers a steady field count over Go's exact
 * all-rows-equal rule, so an ambiguous sample — irregular column counts, a
 * separator that also occurs inside quoted text — can still resolve to a
 * different separator than the Go parser chose at ingest time.
 */
const detectDelimiter = (csvText: string): string =>
  Papa.parse<string[]>(csvText.replace(LeadingBlankLines, ''), {
    header: false,
    skipEmptyLines: 'greedy',
    preview: 10,
    delimitersToGuess: DelimitersToGuess,
  }).meta.delimiter;

/**
 * Parse CSV text into the same records the Go parser produced at ingest time.
 *
 * Records are rebuilt from the raw text span behind each parsed row
 * (`meta.cursor`) so that a physically blank line can be told apart from a
 * record that merely parses to one empty field, e.g. `""` or `,`. Only blank
 * lines are dropped; everything else keeps Go's record numbering, where the
 * header is record 1 and data row i is record i + 2.
 */
export const parseCSV = (csvText: string): CSVData => {
  // Go returns an empty result for a file with no non-whitespace content
  // before it reads any record.
  if (csvText.trim() === '') {
    return { headers: [], rows: [] };
  }
  const records: string[][] = [];
  let recordStart = 0;
  Papa.parse<string[]>(csvText, {
    header: false,
    delimiter: detectDelimiter(csvText),
    skipEmptyLines: false,
    step: (result) => {
      const recordEnd = result.meta.cursor;
      if (!isBlankRecord(csvText, recordStart, recordEnd)) {
        records.push(result.data);
      }
      recordStart = recordEnd;
    },
  });
  const [headers = [], ...rows] = records;
  return { headers, rows };
};

/**
 * Widest row in the table, header included. Ragged rows may have more or fewer
 * cells than the header, so the "last column" is data-driven. Built with a
 * loop rather than Math.max(...lengths): spreading one argument per row
 * exceeds the engine's argument limit on files of a few hundred thousand rows.
 */
export const maxColumnCount = (data: CSVData): number => {
  let widest = data.headers.length;
  for (const row of data.rows) {
    if (row.length > widest) widest = row.length;
  }
  return widest;
};
