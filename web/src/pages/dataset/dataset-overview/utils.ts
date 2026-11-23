import { RunningStatus, RunningStatusOld } from '@/constants/knowledge';
import { LogTabs } from './dataset-common';
import { IFileLogList, IOverviewTotal } from './interface';

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

/**
 * Pipeline logs carry the legacy task status ("0".."5", or names such as
 * "SCHEDULE"). A scheduled run is enqueued but has not started, which the
 * document list renders as QUEUED; map it the same way here so the log modal
 * reads "Queued" with the file list's color instead of the legacy "Scheduled".
 */
export const getLogRunningStatus = (
  status?: string,
): RunningStatus | undefined => {
  if (
    status === RunningStatusOld.SCHEDULE ||
    status === RunningStatus.SCHEDULE
  ) {
    return RunningStatus.QUEUED;
  }
  return status as RunningStatus | undefined;
};
