import type { UploadFormSchemaType } from '@/components/file-upload-dialog';
import { isTableFile } from '@/utils/table-column-extract';

type TableColumnUploadValues = Pick<
  UploadFormSchemaType,
  | 'tableColumnMode'
  | 'tableColumnNames'
  | 'tableColumnNamesByFile'
  | 'tableColumnRoles'
>;

// Builds the parser_config that carries the table column settings into the
// upload request. Only table files carry column settings, so a mixed upload
// with no table file sends nothing.
//
// table_column_names_by_file is positional: the backend pairs entry [i] with
// the i-th file it receives (internal/service/document/document_upload.go,
// tableDocumentConfigForFile), and the multipart body preserves this list's
// order. The array therefore stays aligned with the full file list — non-table
// files keep an empty placeholder rather than being compacted out, which would
// shift every later entry onto the wrong document.
//
// table_column_mode / table_column_roles are sent ONLY when the user chose them
// in the dialog. A root-level key makes the root level authoritative for that
// document: the resolver stops there and never reads the manual profile the same
// document may carry on its canvas entry (internal/ingestion/task/indexdoc,
// ResolveTableProfile). Submitting the untouched "auto" would pin the document
// to auto and silently discard the column roles it was configured with.
export function buildTableUploadParserConfig(
  fileList: UploadFormSchemaType['fileList'],
  {
    tableColumnMode,
    tableColumnNames,
    tableColumnNamesByFile,
    tableColumnRoles,
  }: TableColumnUploadValues,
): Record<string, any> | undefined {
  const hasTableFile = fileList.some((entry) => {
    const file = entry instanceof File ? entry : entry.file;
    return isTableFile(file);
  });
  if (!hasTableFile) {
    return undefined;
  }

  const parserConfig: Record<string, any> = {};
  if (tableColumnNames?.length) {
    parserConfig.table_column_names = tableColumnNames;
  }
  if (Array.isArray(tableColumnNamesByFile)) {
    const byFile = fileList.map((_, index) => {
      const columns = tableColumnNamesByFile[index];
      return Array.isArray(columns) ? columns : [];
    });
    if (byFile.some((columns) => columns.length > 0)) {
      parserConfig.table_column_names_by_file = byFile;
    }
  }
  if (tableColumnMode) {
    parserConfig.table_column_mode = tableColumnMode;
  }
  if (tableColumnMode === 'manual' && tableColumnRoles) {
    parserConfig.table_column_roles = tableColumnRoles;
  }

  return Object.keys(parserConfig).length > 0 ? parserConfig : undefined;
}
