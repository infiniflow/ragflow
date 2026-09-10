import { pickByBackend } from '@/utils/backend-variant';
import * as go from './document-status.go';
import * as python from './document-status.python';
import type { DocumentStatus } from './document-status.go';

export const isDocumentProcessing = (document: DocumentStatus): boolean =>
  pickByBackend({
    go: go.isDocumentProcessing,
    python: python.isDocumentProcessing,
  })(document);

export const toLogStatus = (document: DocumentStatus) =>
  pickByBackend({ go: go.toLogStatus, python: python.toLogStatus })(document);
