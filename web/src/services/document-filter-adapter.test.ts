import {
  adaptDocumentFilter,
  adaptDocumentRunStatusFilter,
} from './document-filter-adapter';

describe('document status filter adapter', () => {
  it('groups task statuses into the visible document status options', () => {
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

  it('only shows status options present in the dataset', () => {
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

  it('expands each selected option into its task statuses', () => {
    expect(adaptDocumentRunStatusFilter(['1', '3'])).toEqual([
      'RUNNING',
      'STOPPING',
      'COMPLETED',
    ]);
  });
});
