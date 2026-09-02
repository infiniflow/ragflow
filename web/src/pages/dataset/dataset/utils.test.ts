import { IngestionStatus, RunningStatus } from './constant';
import { isDocumentProcessing } from './utils';

describe('isDocumentProcessing', () => {
  it('treats a scheduled ingestion task as processing before parsing starts', () => {
    expect(
      isDocumentProcessing({
        run: RunningStatus.UNSTART,
        ingestion_status: IngestionStatus.SCHEDULED,
      }),
    ).toBe(true);
  });
});
