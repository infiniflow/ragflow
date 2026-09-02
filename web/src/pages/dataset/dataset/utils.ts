import { IngestionTaskStatus, RunningStatus } from './constant';
import type { IDocumentInfo } from '@/interfaces/database/document';

export const isParserRunning = (text: RunningStatus) => {
  const isRunning = text === RunningStatus.RUNNING;
  return isRunning;
};

// Go ingestion status can advance before the legacy document run field. The
// Python endpoint omits ingestion_status, so run remains the compatibility path.
export const isDocumentProcessing = (
  document: Pick<IDocumentInfo, 'run' | 'ingestion_status'>,
) =>
  isParserRunning(document.run) ||
  document.ingestion_status === IngestionTaskStatus.CREATED ||
  document.ingestion_status === IngestionTaskStatus.SCHEDULED ||
  document.ingestion_status === IngestionTaskStatus.RUNNING ||
  document.ingestion_status === IngestionTaskStatus.STOPPING;
