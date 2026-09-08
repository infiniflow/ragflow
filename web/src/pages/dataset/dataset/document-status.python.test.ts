import { RunningStatus } from './constant';
import {
  isDocumentProcessing,
  isDocumentQueued,
  isDocumentStopping,
  toLogStatus,
} from './document-status.python';

describe('document-status (python)', () => {
  it('treats RUNNING as processing', () => {
    expect(isDocumentProcessing({ run: RunningStatus.RUNNING } as any)).toBe(
      true,
    );
  });

  it.each([
    RunningStatus.UNSTART,
    RunningStatus.CANCEL,
    RunningStatus.DONE,
    RunningStatus.FAIL,
    RunningStatus.SCHEDULE,
  ])('treats %s as not processing', (run) => {
    expect(isDocumentProcessing({ run } as any)).toBe(false);
  });

  it('never reports queued or stopping', () => {
    expect(isDocumentQueued({} as any)).toBe(false);
    expect(isDocumentStopping({} as any)).toBe(false);
  });

  it('passes run through as log status', () => {
    expect(toLogStatus({ run: RunningStatus.DONE } as any)).toBe(
      RunningStatus.DONE,
    );
  });
});
