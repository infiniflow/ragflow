import Papa from 'papaparse';
import * as XLSX from 'xlsx';

/**
 * Extracts column headers from a CSV or Excel file.
 * Returns an empty array if the file type is not supported or headers cannot be read.
 */
export async function extractTableColumns(file: File): Promise<string[]> {
  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';

  if (ext === 'csv') {
    return extractCsvColumns(file);
  }

  if (['xlsx', 'xls'].includes(ext)) {
    return extractExcelColumns(file);
  }

  return [];
}

function extractCsvColumns(file: File): Promise<string[]> {
  return new Promise((resolve) => {
    Papa.parse(file, {
      preview: 1, // Only read the first row (header)
      header: true,
      skipEmptyLines: true,
      complete(results) {
        const fields = results.meta?.fields ?? [];
        resolve(fields.filter((f) => f.trim().length > 0));
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
        const workbook = XLSX.read(data, { type: 'array', sheetRows: 1 });
        const firstSheetName = workbook.SheetNames[0];
        if (!firstSheetName) {
          resolve([]);
          return;
        }
        const sheet = workbook.Sheets[firstSheetName];
        const rows = XLSX.utils.sheet_to_json<string[]>(sheet, { header: 1 });
        if (rows.length > 0) {
          const headers = rows[0]
            .map((h) => String(h ?? '').trim())
            .filter((h) => h.length > 0);
          resolve(headers);
        } else {
          resolve([]);
        }
      } catch {
        resolve([]);
      }
    };
    reader.onerror = () => resolve([]);
    reader.readAsArrayBuffer(file);
  });
}

/**
 * Check if a file is a table file (CSV or Excel).
 */
export function isTableFile(file: File): boolean {
  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';
  return ['csv', 'xlsx', 'xls'].includes(ext);
}
