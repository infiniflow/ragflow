import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import React from 'react';

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
}));

import { listDataPipelineLogDocument } from '@/services/knowledge-service';
import { useShowLog } from './hooks';

const mockList = jest.mocked(listDataPipelineLogDocument);

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

describe('useShowLog with event stream removed', () => {
  it('uses the KV-derived latest event message when the modal opens', async () => {
    mockList.mockResolvedValue({
      data: {
        data: {
          logs: [
            {
              id: 'run-1',
              document_id: 'doc-1',
              progress_msg: 'stale run text',
              latest_ingestion_event: {
                message: 'Extractor · 标签提取 3123/5047',
              },
            },
          ],
          total: 1,
        },
      },
    } as any);

    const { result } = renderLogs([makeDoc()]);
    act(() => result.current.showLog(makeDoc()));
    await waitFor(() => expect(mockList).toHaveBeenCalled());
    await waitFor(() =>
      expect(result.current.logInfo.details).toBe(
        'Extractor · 标签提取 3123/5047',
      ),
    );
  });

  it('scopes the file-log query to the document id', async () => {
    mockList.mockResolvedValue({
      data: { data: { logs: [], total: 0 } },
    } as any);

    const { result } = renderLogs([makeDoc()]);
    act(() => result.current.showLog(makeDoc()));
    await waitFor(() => expect(mockList).toHaveBeenCalled());
    expect(mockList.mock.calls[0][1]).toMatchObject({
      document_id: 'doc-1',
    });
    expect(result.current.logId).toBeUndefined();
  });

  it('fetches the final file-log row once when the document turns terminal', async () => {
    mockList.mockResolvedValue({
      data: {
        data: {
          logs: [
            {
              id: 'run-1',
              document_id: 'doc-1',
              progress_msg: 'Indexing 5/10',
            },
          ],
          total: 1,
        },
      },
    } as any);

    const running = makeDoc({ ingestion_status: 'RUNNING' });
    const done = makeDoc({ ingestion_status: 'COMPLETED' });
    const { result, rerender } = renderHook(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ({ docs }: { docs: Doc[] }) => useShowLog(docs as any),
      { initialProps: { docs: [running] }, wrapper: Wrapper },
    );
    act(() => result.current.showLog(running as any));
    await waitFor(() => expect(mockList).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.logId).toBe('run-1'));

    rerender({ docs: [done] });
    await waitFor(() => expect(mockList).toHaveBeenCalledTimes(2));
  });
});
