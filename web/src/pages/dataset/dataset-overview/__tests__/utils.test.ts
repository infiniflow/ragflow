import type { IFileLogList } from '../interface';
import {
  buildDatasetOverviewStats,
  getLogRunningStatus,
  hasActiveIngestionLogs,
} from '../utils';
import { RunningStatus, RunningStatusOld } from '@/constants/knowledge';

const Summary = {
  doc_num: 9,
  chunk_num: 20,
  token_num: 200,
  status: {
    unstart_count: 1,
    running_count: 2,
    cancel_count: 1,
    done_count: 4,
    fail_count: 1,
  },
};

describe('dataset overview helpers', () => {
  it('maps the summary and defaults missing download counts to zero', () => {
    expect(
      buildDatasetOverviewStats({
        ...Summary,
        download_status: {
          running_count: 3,
          done_count: 5,
          fail_count: 2,
        },
      }),
    ).toEqual({
      totalFiles: 9,
      downloads: { value: 3, success: 5, failed: 2 },
      processing: { value: 2, success: 4, failed: 1 },
    });
    expect(buildDatasetOverviewStats(Summary).downloads).toEqual({
      value: 0,
      success: 0,
      failed: 0,
    });
  });

  it('recognizes active ingestion logs', () => {
    expect(
      hasActiveIngestionLogs({
        logs: [{ operation_status: 'RUNNING' } as IFileLogList['logs'][number]],
        total: 1,
      }),
    ).toBe(true);
    expect(
      hasActiveIngestionLogs({
        logs: [{ operation_status: 'DONE' } as IFileLogList['logs'][number]],
        total: 1,
      }),
    ).toBe(false);
  });

  it('renders a scheduled pipeline run as queued', () => {
    expect(getLogRunningStatus('5')).toBe(RunningStatus.QUEUED);
    expect(getLogRunningStatus('SCHEDULE')).toBe(RunningStatus.QUEUED);
    expect(getLogRunningStatus('3')).toBe(RunningStatusOld.DONE);
    expect(getLogRunningStatus(undefined)).toBeUndefined();
  });
});
