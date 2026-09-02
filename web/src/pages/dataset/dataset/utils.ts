import { IngestionStatus, RunningStatus } from './constant';
import type { IDocumentInfo } from '@/interfaces/database/document';

export const isParserRunning = (text: RunningStatus) => {
  const isRunning = text === RunningStatus.RUNNING;
  return isRunning;
};

export const isDocumentProcessing = (
  document: Pick<IDocumentInfo, 'run' | 'ingestion_status'>,
) =>
  isParserRunning(document.run) ||
  document.ingestion_status === IngestionStatus.SCHEDULED;
