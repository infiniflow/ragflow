import { IngestionTaskStatus, RunningStatus } from './constant';
import {
  isDocumentProcessing,
  isDocumentQueued,
  isDocumentStopping,
  toLogStatus,
} from './document-status.go';

describe('document-status (go)', () => {
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

  it('treats a terminal run as authoritative even if ingestion_status lingers at STOPPING', () => {
    for (const run of [
      RunningStatus.CANCEL,
      RunningStatus.DONE,
      RunningStatus.FAIL,
    ]) {
      expect(
        isDocumentProcessing({
          run,
          ingestion_status: IngestionTaskStatus.STOPPING,
        }),
      ).toBe(false);
    }
  });

  it('reports queued only for CREATED/SCHEDULED', () => {
    expect(
      isDocumentQueued({ ingestion_status: IngestionTaskStatus.CREATED }),
    ).toBe(true);
    expect(
      isDocumentQueued({ ingestion_status: IngestionTaskStatus.RUNNING }),
    ).toBe(false);
  });

  it('reports stopping only for STOPPING', () => {
    expect(
      isDocumentStopping({ ingestion_status: IngestionTaskStatus.STOPPING }),
    ).toBe(true);
    expect(
      isDocumentStopping({ ingestion_status: IngestionTaskStatus.RUNNING }),
    ).toBe(false);
  });

  it('maps a queued document to QUEUED log status', () => {
    expect(
      toLogStatus({
        run: RunningStatus.UNSTART,
        ingestion_status: IngestionTaskStatus.SCHEDULED,
      }),
    ).toBe(RunningStatus.QUEUED);
  });

  it('passes through run for non-queued documents', () => {
    expect(
      toLogStatus({
        run: RunningStatus.DONE,
        ingestion_status: IngestionTaskStatus.COMPLETED,
      }),
    ).toBe(RunningStatus.DONE);
  });
});
