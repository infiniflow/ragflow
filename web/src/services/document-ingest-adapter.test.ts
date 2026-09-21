let mockIsGo = false;

jest.mock('@/utils/backend-variant', () => ({
  pickByBackend: ({ go, python }: { go: unknown; python: unknown }) =>
    mockIsGo ? go : python,
}));

import { buildDocumentIngestPayload } from './document-ingest-adapter';

describe('buildDocumentIngestPayload', () => {
  beforeEach(() => {
    mockIsGo = false;
  });

  it('forces delete for a Go parse request', () => {
    mockIsGo = true;

    expect(
      buildDocumentIngestPayload({
        documentIds: ['doc-1'],
        run: 1,
        option: { delete: false, apply_kb: true },
      }),
    ).toEqual({
      doc_ids: ['doc-1'],
      run: 1,
      delete: true,
      apply_kb: true,
    });
  });

  it('adds delete when a Go parse request has no options', () => {
    mockIsGo = true;

    expect(
      buildDocumentIngestPayload({ documentIds: ['doc-1'], run: 1 }),
    ).toEqual({ doc_ids: ['doc-1'], run: 1, delete: true });
  });

  it('does not add delete to a Go cancel request', () => {
    mockIsGo = true;

    expect(
      buildDocumentIngestPayload({ documentIds: ['doc-1'], run: 2 }),
    ).toEqual({ doc_ids: ['doc-1'], run: 2 });
  });

  it('preserves the Python parse payload', () => {
    expect(
      buildDocumentIngestPayload({ documentIds: ['doc-1'], run: 1 }),
    ).toEqual({ doc_ids: ['doc-1'], run: 1 });
  });
});
