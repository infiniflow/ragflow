import type { IFileLogList } from '../interface';
import { buildDatasetOverviewStats, hasActiveIngestionLogs } from '../utils';

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
});
