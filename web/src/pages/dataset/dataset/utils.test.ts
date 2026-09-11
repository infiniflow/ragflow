import { IngestionTaskStatus, RunningStatus } from './constant';
import { isDocumentProcessing } from './utils';

describe('isDocumentProcessing', () => {
  it('treats a scheduled ingestion task as processing before parsing starts', () => {
    expect(
      isDocumentProcessing({
        run: RunningStatus.UNSTART,
        ingestion_status: IngestionTaskStatus.SCHEDULED,
      }),
    ).toBe(true);
  });

  it.each([
    IngestionTaskStatus.CREATED,
    IngestionTaskStatus.RUNNING,
    IngestionTaskStatus.STOPPING,
  ])(
    'treats an active %s ingestion task as processing before document state sync',
    (ingestionStatus) => {
      expect(
        isDocumentProcessing({
          run: RunningStatus.UNSTART,
          ingestion_status: ingestionStatus,
        }),
      ).toBe(true);
    },
  );
});
