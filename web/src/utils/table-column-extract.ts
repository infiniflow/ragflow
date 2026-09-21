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

import { probeTableColumns } from '@/services/knowledge-service';
import Papa from 'papaparse';
import * as XLSX from 'xlsx';
import { getExtension } from './document-util';

// The two file families the table chunk method reads, split the way its pipeline
// is configured: the built-in table template takes xls/xlsx/csv/tsv through a
// spreadsheet block and txt through a text one
// (internal/ingestion/pipeline/template/ingestion_pipeline_table.json).
const DELIMITED_TABLE_EXTENSIONS = ['csv', 'tsv', 'txt'];
const SPREADSHEET_TABLE_EXTENSIONS = ['xlsx', 'xls'];

/**
 * Extracts column headers from a CSV, TSV, or Excel file.
 * Tries server-side schema probe first for streaming efficiency and parser parity;
 * falls back to client-side parsing if the server probe is unreachable.
 */
export async function extractTableColumns(file: File): Promise<string[]> {
  try {
    const formData = new FormData();
    formData.append('file', file);
    const body = await probeTableColumns(formData);
    if (body?.code === 0 && Array.isArray(body?.data?.columns)) {
      return body.data.columns;
    }
  } catch {
    // A probe that is unreachable or declines the format is not a failure the
    // user acted on: the local parse below answers the same question.
  }

  const ext = getExtension(file.name);

  if (DELIMITED_TABLE_EXTENSIONS.includes(ext)) {
    return extractCsvColumns(file, ext !== 'csv');
  }

  if (SPREADSHEET_TABLE_EXTENSIONS.includes(ext)) {
    return extractExcelColumns(file);
  }

  return [];
}

function extractCsvColumns(
  file: File,
  tabDelimited: boolean,
): Promise<string[]> {
  return new Promise((resolve) => {
    Papa.parse(file, {
      preview: 5, // Read initial rows to skip leading empties
      header: false,
      skipEmptyLines: false,
      // A delimited header is indexed exactly as written (Python reads tables
      // with csv.reader, whose skipinitialspace default keeps every field), so
      // trimming here would name a column the parser never creates.
      trimValues: false,
      // A .txt table is tab-separated, like a .tsv (rag/app/table.py).
      delimiter: tabDelimited ? '\t' : undefined,
      complete(results) {
        const rows = (results.data as string[][]) ?? [];
        for (const row of rows) {
          if (row.some((cell) => String(cell ?? '').trim().length > 0)) {
            resolve(tableColumnHeaderNames(row, 'delimited'));
            return;
          }
        }
        resolve([]);
      },
      error() {
        resolve([]);
      },
    });
  });
}

function extractExcelColumns(file: File): Promise<string[]> {
  return new Promise((resolve) => {
    const reader = new FileReader();
    reader.onload = (e) => {
      try {
        const data = new Uint8Array(e.target?.result as ArrayBuffer);
        const workbook = XLSX.read(data, { type: 'array', sheetRows: 5 });
        const firstSheetName = workbook.SheetNames[0];
        if (!firstSheetName) {
          resolve([]);
          return;
        }
        const sheet = workbook.Sheets[firstSheetName];
        const rows = XLSX.utils.sheet_to_json<string[]>(sheet, { header: 1 });
        for (const row of rows) {
          if (row.some((cell) => String(cell ?? '').trim().length > 0)) {
            resolve(tableColumnHeaderNames(row, 'spreadsheet'));
            return;
          }
        }
        resolve([]);
      } catch {
        resolve([]);
      }
    };
    reader.onerror = () => resolve([]);
    reader.readAsArrayBuffer(file);
  });
}

function deduplicateColumns(columns: string[]): string[] {
  const reserved = new Set(columns);
  const used = new Set<string>();
  const counts = new Map<string, number>();
  const unique: string[] = [];
  for (const col of columns) {
    counts.set(col, (counts.get(col) ?? 0) + 1);
    if (!used.has(col)) {
      unique.push(col);
      used.add(col);
      continue;
    }
    let suffix = counts.get(col)!;
    let newName = `${col}_${suffix}`;
    while (used.has(newName) || reserved.has(newName)) {
      suffix++;
      newName = `${col}_${suffix}`;
    }
    counts.set(col, suffix);
    used.add(newName);
    unique.push(newName);
  }
  return unique;
}

// Spreadsheet bookkeeping columns carry no content: the table parser deletes
// them before rendering (rag/app/table.py:539-541), so the client-side fallback
// must drop them as well — otherwise it would offer a role for a column that
// never reaches a chunk. Kept in sync with tableBookkeepingColumns in
// internal/parser/parser/table_row_render.go.
const BOOKKEEPING_COLUMNS = ['id', '_id', 'index', 'idx'];

// tableColumnHeaderNames applies the ingestion header rules of a file kind to a
// raw header row: drop bookkeeping columns and dedupe the survivors. Only a
// spreadsheet also trims each cell and names an empty one by position
// (_parse_simple_headers); a delimited header is indexed exactly as read, so a
// padded or empty cell keeps that spelling as its column name. Applies the same
// rules rag/app/table.py indexes with (`_parse_simple_headers` at :206-228, the
// first record as read at :507), which are the Go TableHeaderRule.
function tableColumnHeaderNames(
  row: unknown[],
  rule: 'spreadsheet' | 'delimited',
): string[] {
  const spreadsheet = rule === 'spreadsheet';
  const raw = row.map((cell, idx) => {
    if (!spreadsheet) {
      return String(cell ?? '');
    }
    const trimmed = String(cell ?? '').trim();
    return trimmed.length > 0 ? trimmed : `Column_${idx + 1}`;
  });
  return deduplicateColumns(
    raw.filter((name) => !BOOKKEEPING_COLUMNS.includes(name)),
  );
}

/**
 * Check if a file is a table file: every format the table chunk method reads,
 * which is what decides whether the upload dialog offers column settings. The
 * server schema probe covers most of this set and declines the rest — binary xls
 * among them — and every declined format is answered by the local parse above.
 */
export function isTableFile(file: File): boolean {
  const ext = getExtension(file.name);
  return (
    DELIMITED_TABLE_EXTENSIONS.includes(ext) ||
    SPREADSHEET_TABLE_EXTENSIONS.includes(ext)
  );
}
