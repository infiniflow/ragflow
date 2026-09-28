import { pickByBackend } from '@/utils/backend-variant';

export type DocumentIngestOption = {
  delete: boolean;
  apply_kb: boolean;
};

export type DocumentIngestPayload = {
  doc_ids: string[];
  run: number;
  delete?: boolean;
  apply_kb?: boolean;
};

export const buildDocumentIngestPayload = ({
  documentIds,
  run,
  option,
}: {
  documentIds: string[];
  run: number;
  option?: DocumentIngestOption;
}): DocumentIngestPayload => {
  const payload = {
    doc_ids: documentIds,
    run,
    ...(option || {}),
  };

  return pickByBackend({
    go: run === 1 ? { ...payload, delete: true } : payload,
    python: payload,
  });
};
