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

import api from '@/utils/api';
import request from '@/utils/request';
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
    const res = await request.post(api.probeTable, { data: formData });
    if (res?.data?.code === 0 && Array.isArray(res?.data?.data?.columns)) {
      return res.data.data.columns;
    }
  } catch {
    // Graceful fallback to client-side parsing below
  }

  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';

  if (ext === 'csv' || ext === 'tsv' || ext === 'txt') {
    return extractCsvColumns(file, ext === 'tsv');
  }

  if (['xlsx', 'xls'].includes(ext)) {
    return extractExcelColumns(file);
  }

  return [];
}

function extractCsvColumns(file: File, isTSV: boolean): Promise<string[]> {
  return new Promise((resolve) => {
    Papa.parse(file, {
      preview: 5, // Read initial rows to skip leading empties
      header: false,
      skipEmptyLines: false,
      delimiter: isTSV ? '\t' : undefined,
      complete(results) {
        const rows = (results.data as string[][]) ?? [];
        for (const row of rows) {
          if (row.some((cell) => String(cell ?? '').trim().length > 0)) {
            const raw = row.map((cell, idx) => {
              const trimmed = String(cell ?? '').trim();
              return trimmed.length > 0 ? trimmed : `Column_${idx + 1}`;
            });
            resolve(deduplicateColumns(raw));
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
            const raw = row.map((cell, idx) => {
              const trimmed = String(cell ?? '').trim();
              return trimmed.length > 0 ? trimmed : `Column_${idx + 1}`;
            });
            resolve(deduplicateColumns(raw));
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

/**
 * Check if a file is a table file (CSV, TSV, or Excel).
 */
export function isTableFile(file: File): boolean {
  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';
  return ['csv', 'xlsx', 'xls', 'tsv'].includes(ext);
}
