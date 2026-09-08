import { RunningStatus } from './constant';
import type { IDocumentInfo } from '@/interfaces/database/document';

type DocumentStatus = Pick<IDocumentInfo, 'run' | 'ingestion_status'>;

// The Python endpoint omits ingestion_status, so run is the only signal.
export const isDocumentProcessing = (document: DocumentStatus): boolean =>
  document.run === RunningStatus.RUNNING;

export const isDocumentQueued = (
  _document: Pick<IDocumentInfo, 'ingestion_status'>,
): boolean => false;

export const isDocumentStopping = (
  _document: Pick<IDocumentInfo, 'ingestion_status'>,
): boolean => false;

export const toLogStatus = (document: DocumentStatus): RunningStatus =>
  document.run;
