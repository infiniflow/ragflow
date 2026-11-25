import { formatDate, formatSecondsToHumanReadable } from '@/utils/date';
import { ILogInfo } from '../process-log-modal';
import { LogTabs } from './dataset-common';
import {
  DocumentLog,
  IFileLogItem,
  IFileLogList,
  IOverviewTotal,
} from './interface';

const ActiveLogStatuses = new Set([
  'UNSTART',
  'RUNNING',
  'SCHEDULE',
  '0',
  '1',
  '5',
]);

export const DatasetOverviewKeys = {
  summary: (datasetId?: string) =>
    ['datasetOverview', datasetId, 'summary'] as const,
  logs: (
    datasetId: string | undefined,
    page: number | undefined,
    pageSize: number | undefined,
    searchString: string,
    active: (typeof LogTabs)[keyof typeof LogTabs],
    filterValue: Record<string, unknown>,
  ) =>
    [
      'datasetOverview',
      datasetId,
      'logs',
      page,
      pageSize,
      searchString,
      active,
      filterValue,
    ] as const,
};

export const buildDatasetOverviewStats = (summary?: IOverviewTotal) => ({
  totalFiles: summary?.doc_num ?? 0,
  downloads: {
    value: summary?.download_status?.running_count ?? 0,
    success: summary?.download_status?.done_count ?? 0,
    failed: summary?.download_status?.fail_count ?? 0,
  },
  processing: {
    value: summary?.status.running_count ?? 0,
    success: summary?.status.done_count ?? 0,
    failed: summary?.status.fail_count ?? 0,
  },
});

export const hasActiveIngestionLogs = (logs?: IFileLogList) =>
  logs?.logs.some((log) => ActiveLogStatuses.has(log.operation_status)) ??
  false;

type FileLogRow = IFileLogItem & DocumentLog;

/** Build the modal view from a single run row, preferring the KV-derived
 * snapshot message over the stored progress text. */
export const buildFileLogInfo = (row: FileLogRow): ILogInfo =>
  ({
    taskId: row.dsl?.task_id,
    fileName: row.document_name,
    source: row.source_from,
    task: row.task_type,
    status: row.operation_status,
    startDate: formatDate(row.process_begin_at),
    duration: formatSecondsToHumanReadable(row.process_duration || 0),
    details: row.latest_ingestion_event?.message || row.progress_msg || '',
  }) as ILogInfo;

/** Re-derive an open modal from the latest polled rows so its badge, details
 * and duration track the selected run. Falls back to the click-time snapshot
 * when the row has disappeared from the current page. */
export const resolveModalLogInfo = (
  rows: FileLogRow[] | undefined,
  selectedID: string | undefined,
  fallback: ILogInfo | undefined,
): ILogInfo | undefined => {
  if (!fallback || !rows || !selectedID) {
    return fallback;
  }
  const current = rows.find((row) => row.id === selectedID);
  return current ? buildFileLogInfo(current) : fallback;
};
