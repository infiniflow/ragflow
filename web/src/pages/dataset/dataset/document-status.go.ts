import { IngestionTaskStatus, RunningStatus } from './constant';
import type { IDocumentInfo } from '@/interfaces/database/document';

type DocumentStatus = Pick<IDocumentInfo, 'run' | 'ingestion_status'>;

// Go ingestion status is the source of truth: it advances before the legacy
// document run field, which StartRunning / progressSink mirror with
// best-effort non-atomic writes. A terminal run is authoritative: the backend
// may leave ingestion_status at STOPPING after a cancel completes, and the
// document must then be treated as not running so its parsing style and
// restart action work.
export const isDocumentProcessing = (document: DocumentStatus): boolean => {
  if (document.run === RunningStatus.RUNNING) return true;
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

export const isDocumentQueued = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
): boolean =>
  document.ingestion_status === IngestionTaskStatus.CREATED ||
  document.ingestion_status === IngestionTaskStatus.SCHEDULED;

export const isDocumentStopping = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
): boolean => document.ingestion_status === IngestionTaskStatus.STOPPING;

// The Go backend reports a queued document via ingestion_status while the
// legacy run field stays UNSTART; surface it as QUEUED.
export const toLogStatus = (document: DocumentStatus): RunningStatus =>
  isDocumentQueued(document) ? RunningStatus.QUEUED : document.run;
