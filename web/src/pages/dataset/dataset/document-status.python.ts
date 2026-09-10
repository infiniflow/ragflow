import { RunningStatus } from './constant';
import type { DocumentStatus } from './document-status.go';

// The Python endpoint omits ingestion_status, so run is the only signal.
export const isDocumentProcessing = (document: DocumentStatus): boolean =>
  document.run === RunningStatus.RUNNING;

export const toLogStatus = (document: DocumentStatus): RunningStatus =>
  document.run;
