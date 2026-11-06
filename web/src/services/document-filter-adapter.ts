import {
  IngestionTaskStatus,
  RunningStatus,
  RunningStatusOld,
} from '@/constants/knowledge';
import type { IDocumentInfoFilter } from '@/interfaces/database/document';

type DocumentFilterResponse = Omit<IDocumentInfoFilter, 'run_status'> & {
  run_status?: IDocumentInfoFilter['run_status'];
  ingestion_status?: Record<string, number>;
};

const goQueryStatuses: Record<string, string[]> = {
  [RunningStatusOld.UNSTART]: [IngestionTaskStatus.UNSTART],
  [RunningStatus.QUEUED]: [
    IngestionTaskStatus.CREATED,
    IngestionTaskStatus.SCHEDULED,
  ],
  [RunningStatusOld.RUNNING]: [
    IngestionTaskStatus.RUNNING,
    IngestionTaskStatus.STOPPING,
  ],
  [RunningStatusOld.CANCEL]: [IngestionTaskStatus.STOPPED],
  [RunningStatusOld.DONE]: [IngestionTaskStatus.COMPLETED],
  [RunningStatusOld.FAIL]: [IngestionTaskStatus.FAILED],
};

const goStatusOptions = Object.fromEntries(
  Object.entries(goQueryStatuses).flatMap(([option, statuses]) =>
    statuses.map((status) => [status, option]),
  ),
);

export const adaptDocumentFilter = (
  filter: DocumentFilterResponse,
): IDocumentInfoFilter => {
  const runStatus: IDocumentInfoFilter['run_status'] = {};
  for (const [status, count] of Object.entries(filter.ingestion_status ?? {})) {
    const option = goStatusOptions[status];
    if (option) {
      runStatus[option] = (runStatus[option] ?? 0) + count;
    }
  }

  return { ...filter, run_status: runStatus };
};

export const adaptDocumentRunStatusFilter = (
  statuses?: string[],
): string[] | undefined =>
  statuses?.flatMap((status) => goQueryStatuses[status] ?? [status]);
