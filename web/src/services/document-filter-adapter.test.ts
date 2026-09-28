let mockIsGo = false;

jest.mock('@/utils/backend-variant', () => ({
  pickByBackend: ({ go, python }: { go: unknown; python: unknown }) =>
    mockIsGo ? go : python,
}));

import {
  adaptDocumentFilter,
  adaptDocumentRunStatusFilter,
} from './document-filter-adapter';

describe('document status filter adapter', () => {
  beforeEach(() => {
    mockIsGo = false;
  });

  it('groups Go task statuses into the visible document status options', () => {
    mockIsGo = true;

    expect(
      adaptDocumentFilter({
        suffix: { pdf: 8 },
        metadata: {},
        ingestion_status: {
          UNSTART: 1,
          CREATED: 2,
          SCHEDULED: 1,
          RUNNING: 3,
          STOPPING: 1,
          STOPPED: 1,
          COMPLETED: 4,
          FAILED: 1,
        },
      }).run_status,
    ).toEqual({
      '0': 1,
      QUEUED: 3,
      '1': 4,
      '2': 1,
      '3': 4,
      '4': 1,
    });

    expect(adaptDocumentRunStatusFilter(['QUEUED', '1', '3'])).toEqual([
      'CREATED',
      'SCHEDULED',
      'RUNNING',
      'STOPPING',
      'COMPLETED',
    ]);
  });

  it('only shows Go status options present in the dataset', () => {
    mockIsGo = true;

    expect(
      adaptDocumentFilter({
        suffix: { pdf: 2 },
        metadata: { empty_metadata: { true: 1 } },
        ingestion_status: { RUNNING: 1, COMPLETED: 1 },
      }).run_status,
    ).toEqual({
      '1': 1,
      '3': 1,
    });
  });

  it('keeps the Python filter response and selected status values', () => {
    const filter = {
      suffix: { docx: 2 },
      metadata: {},
      run_status: { '1': 1, '3': 1 },
    };

    expect(adaptDocumentFilter(filter)).toEqual(filter);
    expect(adaptDocumentRunStatusFilter(['1', '3'])).toEqual(['1', '3']);
  });
});
