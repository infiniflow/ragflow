import { IngestionTaskStatus, RunningStatus } from './constant';
import type { IDocumentInfo } from '@/interfaces/database/document';

export type DocumentStatus = Pick<IDocumentInfo, 'run' | 'ingestion_status'>;

export const isParserRunning = (text: RunningStatus) => {
  const isRunning = text === RunningStatus.RUNNING;
  return isRunning;
};

export const isDocumentQueued = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
) =>
  document.ingestion_status === IngestionTaskStatus.CREATED ||
  document.ingestion_status === IngestionTaskStatus.SCHEDULED;

// Go ingestion status can advance before the legacy document run field. The
// Python endpoint omits ingestion_status, so run remains the compatibility path.
// A terminal legacy run status is authoritative: the Go backend may leave
// ingestion_status at STOPPING after a cancel completes, and the document must
// then be treated as not running so its parsing style and restart action work.
export const isDocumentProcessing = (
  document: Pick<IDocumentInfo, 'run' | 'ingestion_status'>,
) => {
  if (isParserRunning(document.run)) return true;
  if (
    document.run === RunningStatus.CANCEL ||
    document.run === RunningStatus.DONE ||
    document.run === RunningStatus.FAIL
  ) {
    return false;
  }
  return (
    document.ingestion_status === IngestionTaskStatus.CREATED ||
    document.ingestion_status === IngestionTaskStatus.SCHEDULED ||
    document.ingestion_status === IngestionTaskStatus.RUNNING ||
    document.ingestion_status === IngestionTaskStatus.STOPPING
  );
};

export const isDocumentStopping = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
): boolean => document.ingestion_status === IngestionTaskStatus.STOPPING;

// The Go backend reports a queued document via ingestion_status while the
// legacy run field stays UNSTART; surface it as QUEUED.
export const toLogStatus = (document: DocumentStatus): RunningStatus =>
  isDocumentQueued(document) ? RunningStatus.QUEUED : document.run;
