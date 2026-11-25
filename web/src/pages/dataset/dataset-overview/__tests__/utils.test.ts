import { RunningStatusMap, RunningStatusOld } from '@/constants/knowledge';
import type { IFileLogList } from '../interface';
import {
  buildDatasetOverviewStats,
  buildFileLogInfo,
  hasActiveIngestionLogs,
  resolveModalLogInfo,
} from '../utils';

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

  it('labels a scheduled pipeline run as queued', () => {
    expect(RunningStatusMap[RunningStatusOld.SCHEDULE]).toBe('Queued');
  });
});

describe('file-log modal details', () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const makeRow = (overrides: Record<string, unknown> = {}): any => ({
    id: 'run-1',
    dsl: { task_id: 'task-1' },
    document_name: 'a.pdf',
    source_from: 'local',
    task_type: 'parse',
    operation_status: 'RUNNING',
    process_duration: 5,
    progress_msg: 'stale run text',
    ...overrides,
  });

  it('prefers the snapshot message, then progress_msg, then empty', () => {
    expect(
      buildFileLogInfo(
        makeRow({
          latest_ingestion_event: { message: 'Extractor · 标签提取 3/10' },
        }),
      ).details,
    ).toBe('Extractor · 标签提取 3/10');
    expect(buildFileLogInfo(makeRow()).details).toBe('stale run text');
    expect(buildFileLogInfo(makeRow({ progress_msg: undefined })).details).toBe(
      '',
    );
  });

  it('re-derives the open modal from the latest polled row', () => {
    const clicked = buildFileLogInfo(makeRow());
    const fresh = makeRow({ operation_status: 'DONE', progress_msg: 'final' });
    const resolved = resolveModalLogInfo([fresh], 'run-1', clicked);
    expect(resolved?.status).toBe('DONE');
    expect(resolved?.details).toBe('final');
  });

  it('falls back to the click-time snapshot when the row is gone', () => {
    const clicked = buildFileLogInfo(makeRow());
    expect(resolveModalLogInfo([], 'run-1', clicked)).toEqual(clicked);
    expect(resolveModalLogInfo(undefined, 'run-1', clicked)).toEqual(clicked);
  });
});
