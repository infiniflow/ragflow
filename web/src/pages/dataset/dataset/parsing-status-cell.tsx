import { IDocumentInfo } from '@/interfaces/database/document';
import { BackendVariant } from '@/utils/backend-variant';
import { ParseDropdownButton } from './parse-dropdown-button';
import { ParsingStatusCellGo } from './parsing-status-cell.go';
import { ParsingStatusCellPython } from './parsing-status-cell.python';

export { ParseDropdownButton };

// Dispatch point for the document parse-status cell: the Go backend drives
// status from ingestion_status (queued / progress, with cancel disabled while
// stopping), while the Python backend only knows the legacy run field.
export function ParsingStatusCell({
  record,
  showLog,
}: {
  record: IDocumentInfo;
  showLog: (record: IDocumentInfo) => void;
}) {
  return (
    <BackendVariant
      go={<ParsingStatusCellGo record={record} showLog={showLog} />}
      python={<ParsingStatusCellPython record={record} showLog={showLog} />}
    />
  );
}
