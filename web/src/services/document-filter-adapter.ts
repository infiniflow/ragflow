import {
  IngestionTaskStatus,
  RunningStatus,
  RunningStatusOld,
} from '@/constants/knowledge';
import type { IDocumentInfoFilter } from '@/interfaces/database/document';
import { pickByBackend } from '@/utils/backend-variant';

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
  const goRunStatus: IDocumentInfoFilter['run_status'] = {};
  for (const [status, count] of Object.entries(filter.ingestion_status ?? {})) {
    const option = goStatusOptions[status];
    if (option) {
      goRunStatus[option] = (goRunStatus[option] ?? 0) + count;
    }
  }

  return pickByBackend({
    go: { ...filter, run_status: goRunStatus },
    python: { ...filter, run_status: filter.run_status ?? {} },
  });
};

export const adaptDocumentRunStatusFilter = (
  statuses?: string[],
): string[] | undefined =>
  pickByBackend({
    go: statuses?.flatMap((status) => goQueryStatuses[status] ?? [status]),
    python: statuses,
  });
