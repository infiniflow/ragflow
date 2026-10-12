import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { IngestionTaskStatus, RunningStatus } from '@/constants/knowledge';

// NOTE: this suite uses jest.mock, so it is transformed by esbuild-jest's
// babel pipeline, which does not strip TypeScript. Imported bindings must not
// appear in type annotations, so the document shape is declared locally and
// the hook input is cast at the call site.
type Doc = {
  id: string;
  name: string;
  ingestion_status?: string;
  [key: string]: unknown;
};

jest.mock('react-router', () => ({
  useParams: jest.fn(() => ({ id: 'kb-1' })),
}));

jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: jest.fn(() => ({ knowledgeId: 'kb-1' })),
}));

jest.mock('@/hooks/use-document-request', () => ({
  useFetchDocumentsByIds: () => ({ documents: [] }),
}));

jest.mock('@/services/knowledge-service', () => ({
  listDataPipelineLogDocument: jest.fn(),
  listIngestionMessages: jest.fn(),
}));

import {
  listDataPipelineLogDocument,
  listIngestionMessages,
} from '@/services/knowledge-service';
import { useShowLog } from './hooks';

const mockList = jest.mocked(listDataPipelineLogDocument);
const mockMessages = jest.mocked(listIngestionMessages);

function makeDoc(overrides: Partial<Doc> = {}): Doc {
  return { id: 'doc-1', name: 'a.pdf', ...overrides };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function Wrapper(props: { children: any }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, props.children);
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function renderLogs(documents: Doc[]): any {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return renderHook(() => useShowLog(documents as any), { wrapper: Wrapper });
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe('useShowLog early-log fallback', () => {
  it('performs one finishing poll after terminal before stopping', async () => {
    jest.useFakeTimers();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: {
        data: { logs: [{ id: 'run-1', document_id: 'doc-1' }], total: 1 },
      },
    } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockMessages.mockResolvedValue({
      data: {
        data: {
          run_count: 1,
          items: [],
          has_more_before: false,
          has_more_after: false,
          terminal: true,
        },
      },
    } as any);

    try {
      const { result } = renderLogs([
        makeDoc({ ingestion_status: IngestionTaskStatus.COMPLETED }),
      ]);
      act(() =>
        result.current.showLog(
          makeDoc({ ingestion_status: IngestionTaskStatus.COMPLETED }),
        ),
      );
      await waitFor(() => expect(mockMessages).toHaveBeenCalledTimes(1));
      await act(async () => {
        jest.advanceTimersByTime(5000);
        await Promise.resolve();
      });
      await waitFor(() => expect(mockMessages).toHaveBeenCalledTimes(2));
    } finally {
      jest.useRealTimers();
    }
  });

  it('exposes a previous-page loader for historical event scrolling', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: {
        data: { logs: [{ id: 'run-1', document_id: 'doc-1' }], total: 1 },
      },
    } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockMessages.mockResolvedValue({
      data: {
        data: {
          run_count: 1,
          items: [
            {
              id: 20,
              ts: '2026-01-01T00:00:00Z',
              event_type: 1,
              component: '',
              phase: 0,
              message: 'latest',
            },
          ],
          oldest_id: 20,
          newest_id: 20,
          has_more_before: true,
          has_more_after: false,
          terminal: false,
        },
      },
    } as any);

    const { result } = renderLogs([
      makeDoc({ ingestion_status: IngestionTaskStatus.RUNNING }),
    ]);
    act(() =>
      result.current.showLog(
        makeDoc({ ingestion_status: IngestionTaskStatus.RUNNING }),
      ),
    );
    await waitFor(() => expect(mockMessages).toHaveBeenCalledTimes(1));
    expect(result.current.logInfo.loadPreviousEvents).toEqual(
      expect.any(Function),
    );
    expect(result.current.logInfo.hasPreviousEvents).toBe(true);
  });

  it('loads messages by the exact pipeline log identity', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: {
        data: { logs: [{ id: 'run-2', document_id: 'doc-1' }], total: 1 },
      },
    } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockMessages.mockResolvedValue({
      data: {
        data: {
          run_count: 2,
          items: [
            {
              id: 12,
              ts: '2026-01-01T00:00:00Z',
              event_type: 1,
              component: '',
              phase: 0,
              message: 'Task is queued...',
            },
          ],
          has_more_before: false,
          has_more_after: false,
          terminal: false,
        },
      },
    } as any);

    const doc = makeDoc({ ingestion_status: IngestionTaskStatus.CREATED });
    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    await waitFor(() =>
      expect(
        result.current.logInfo.events?.map(
          (event: { message: string }) => event.message,
        ),
      ).toEqual(['Task is queued...']),
    );
    expect(mockList).toHaveBeenCalledWith(
      'kb-1',
      expect.objectContaining({
        document_id: 'doc-1',
        log_type: 'file',
        orderby: 'create_time',
        desc: true,
        page_size: 1,
      }),
    );
    expect(mockMessages).toHaveBeenCalledWith('kb-1', 'run-2', {
      limit: 200,
    });
  });

  it('uses the current run progress field', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: {
        data: {
          logs: [
            {
              id: 'run-1',
              document_id: 'doc-1',
              progress_msg: 'Parse completed',
            },
          ],
          total: 1,
        },
      },
    } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockMessages.mockResolvedValue({
      data: {
        data: {
          run_count: 0,
          items: [],
          has_more_before: false,
          has_more_after: false,
          terminal: true,
        },
      },
    } as any);

    const doc = makeDoc({ ingestion_status: IngestionTaskStatus.COMPLETED });
    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    await waitFor(() =>
      expect(mockMessages).toHaveBeenCalledWith('kb-1', 'run-1', {
        limit: 200,
      }),
    );
    expect(result.current.logInfo.details).toBe('Parse completed');
    expect(result.current.logInfo.events).toBeUndefined();
    expect(result.current.logInfo.status).toBe(RunningStatus.DONE);
    expect(mockList).toHaveBeenCalledWith(
      'kb-1',
      expect.objectContaining({ log_type: 'file' }),
    );
  });

  it('prefers the selected log event over its stored progress message', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: {
        data: {
          logs: [
            {
              id: 'run-1',
              document_id: 'doc-1',
              progress_msg: 'old snapshot',
              latest_ingestion_event: {
                id: 7,
                ts: '2026-01-01T00:00:00Z',
                event_type: 3,
                component: '',
                phase: 0,
                message: 'latest run event',
              },
            },
          ],
          total: 1,
        },
      },
    } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockMessages.mockResolvedValue({
      data: {
        data: {
          run_count: 0,
          items: [],
          has_more_before: false,
          has_more_after: false,
          terminal: true,
        },
      },
    } as any);

    const doc = makeDoc({ ingestion_status: IngestionTaskStatus.COMPLETED });
    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    await waitFor(() =>
      expect(result.current.logInfo.details).toBe('latest run event'),
    );
  });

  it('scopes the query to the document instead of a fuzzy name search', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: { data: { logs: [], total: 0 } },
    } as any);

    const doc = makeDoc({
      name: 'a.pdf',
      ingestion_status: IngestionTaskStatus.SCHEDULED,
    });
    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    await waitFor(() => expect(mockList).toHaveBeenCalled());
    expect(mockList).toHaveBeenCalledWith(
      'kb-1',
      expect.objectContaining({ document_id: 'doc-1', log_type: 'file' }),
    );
    // The name search would return other documents' rows and could push this
    // document's queued row past the first page.
    expect(mockList.mock.calls[0][1]).not.toHaveProperty('keywords');
    // No run row means there is no current event to display.
    expect(result.current.logInfo.details).toBe('-');
  });

  it('uses the current log identity even when the document is running', async () => {
    const doc = makeDoc({
      ingestion_status: IngestionTaskStatus.RUNNING,
    });

    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    await waitFor(() => expect(mockList).toHaveBeenCalled());
    expect(result.current.logInfo.details).toBe('-');
  });
});
