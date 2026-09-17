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

  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';

  if (ext === 'csv' || ext === 'tsv' || ext === 'txt') {
    return extractCsvColumns(file, ext !== 'csv');
  }

  if (['xlsx', 'xls'].includes(ext)) {
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
// them before rendering (rag/app/table.py, mirrored by the Go
// parser.TableColumnHeaderNames), so the client-side fallback must drop them as
// well — otherwise it would offer a role for a column that never reaches a
// chunk. Kept in sync with tableBookkeepingColumns in
// internal/parser/parser/table_row_render.go.
const BOOKKEEPING_COLUMNS = ['id', '_id', 'index', 'idx'];

// tableColumnHeaderNames applies the ingestion header rules of a file kind to a
// raw header row: drop bookkeeping columns and dedupe the survivors. Only a
// spreadsheet also trims each cell and names an empty one by position
// (_parse_simple_headers); a delimited header is indexed exactly as read, so a
// padded or empty cell keeps that spelling as its column name. Mirrors
// rag/app/table.py table_column_header_names and the Go TableHeaderRule.
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
 * Check if a file is a table file (CSV, TSV, or Excel).
 */
export function isTableFile(file: File): boolean {
  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';
  return ['csv', 'xlsx', 'xls', 'tsv'].includes(ext);
}

export type DatasetTableColumnSettings = {
  mode: 'auto' | 'manual';
  roles: Record<string, 'indexing' | 'metadata' | 'both'>;
};

// normalizeRole canonicalises a stored role for display only. The comparison is
// exact because the runtime compares exactly: the chunk body and the dataset
// field_map test the stored string with `role in (...)`, without trimming or
// case-folding (rag/app/table.py:701-706, and Go common.NormalizeColumnRole
// ports the same rule), so a value like " Indexing " excludes the column rather
// than honouring it. The dialog must not show a match ingestion will not make.
function normalizeRole(raw: unknown): 'indexing' | 'metadata' | 'both' {
  const role = String(raw ?? '');
  if (role === 'indexing' || role === 'vectorize') {
    return 'indexing';
  }
  if (role === 'metadata') {
    return 'metadata';
  }
  return 'both';
}

function collectRoles(raw: unknown): DatasetTableColumnSettings['roles'] {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return {};
  }
  const roles: DatasetTableColumnSettings['roles'] = {};
  // Keep every stored key: the lookup is by exact column name
  // (`column_roles.get(col, "both")`, rag/app/table.py:701), and a delimited
  // header is the first record as read (rag/app/table.py:582), which can
  // legitimately name a column "" or " ". Dropping a blank key would hide a
  // role ingestion honours.
  for (const [column, role] of Object.entries(raw as Record<string, unknown>)) {
    roles[column] = normalizeRole(role);
  }
  return roles;
}

// storedMode reads a persisted column mode the way the runtime does: an absent
// or non-string value is unset, and every other value is taken verbatim, because
// only the exact "manual" selects manual
// (internal/common/table_column.go, NormalizeTableColumnMode).
function storedMode(raw: unknown): string {
  return typeof raw === 'string' ? raw : '';
}

// hasStoredColumnNames counts the names the runtime keeps: any string entry,
// blank included, because a delimited header can name a column "" and that is a
// real column of the published schema
// (internal/ingestion/task/indexdoc, parseTableColumnNames).
function hasStoredColumnNames(raw: unknown): boolean {
  return Array.isArray(raw) && raw.some((name) => typeof name === 'string');
}

/**
 * Effective dataset-level table column settings. Root-level keys come first —
 * that is what the dataset settings page saves — then the component-shaped
 * entry a canvas or the document dialog writes. Mirrors the backend's
 * ResolveTableProfile ordering, including its rule that any one of mode, roles
 * or names makes a level the authoritative one.
 *
 * The upload dialog shows these as its initial selection so an untouched
 * dialog displays what ingestion will actually use; it does not send them back
 * (see buildTableUploadParserConfig).
 */
export function resolveDatasetTableColumnSettings(
  parserConfig?: Record<string, any> | null,
): DatasetTableColumnSettings {
  const config = parserConfig ?? {};
  const rootMode = storedMode(config.table_column_mode);
  const rootRoles = collectRoles(config.table_column_roles);
  if (
    rootMode ||
    Object.keys(rootRoles).length > 0 ||
    hasStoredColumnNames(config.table_column_names)
  ) {
    return {
      mode: rootMode === 'manual' ? 'manual' : 'auto',
      roles: rootRoles,
    };
  }

  const parserIDs = Object.keys(config)
    .filter((key) => key.startsWith('Parser:'))
    .sort();
  for (const parserID of parserIDs) {
    const spreadsheet = config[parserID]?.spreadsheet;
    if (!spreadsheet || typeof spreadsheet !== 'object') {
      continue;
    }
    const nestedMode = storedMode(spreadsheet.column_mode);
    const nestedRoles = collectRoles(spreadsheet.column_roles);
    if (
      nestedMode ||
      Object.keys(nestedRoles).length > 0 ||
      hasStoredColumnNames(spreadsheet.column_names)
    ) {
      return {
        mode: nestedMode === 'manual' ? 'manual' : 'auto',
        roles: nestedRoles,
      };
    }
  }

  return { mode: 'auto', roles: {} };
}
