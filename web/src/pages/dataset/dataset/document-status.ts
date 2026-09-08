import { pickByBackend } from '@/utils/backend-variant';
import type { IDocumentInfo } from '@/interfaces/database/document';
import * as go from './document-status.go';
import * as python from './document-status.python';

type DocumentStatus = Pick<IDocumentInfo, 'run' | 'ingestion_status'>;

export const isDocumentProcessing = (document: DocumentStatus): boolean =>
  pickByBackend({
    go: go.isDocumentProcessing,
    python: python.isDocumentProcessing,
  })(document);

export const isDocumentQueued = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
): boolean =>
  pickByBackend({
    go: go.isDocumentQueued,
    python: python.isDocumentQueued,
  })(document);

export const isDocumentStopping = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
): boolean =>
  pickByBackend({
    go: go.isDocumentStopping,
    python: python.isDocumentStopping,
  })(document);

export const toLogStatus = (document: DocumentStatus) =>
  pickByBackend({ go: go.toLogStatus, python: python.toLogStatus })(document);
