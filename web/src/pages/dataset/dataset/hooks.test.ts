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
  run: string;
  progress_msg: string;
  ingestion_status?: string;
  [key: string]: unknown;
};

// The hook branches on the backend variant; drive it from a mutable flag so a
// single suite can exercise both the Python and the Go path.
let mockIsGo = false;

jest.mock('react-router', () => ({
  useParams: jest.fn(() => ({ id: 'kb-1' })),
}));

jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: jest.fn(() => ({ knowledgeId: 'kb-1' })),
}));

jest.mock('@/utils/backend-variant', () => ({
  useIsGoBackend: () => mockIsGo,
  pickByBackend: (variants: { go: unknown; python: unknown }) =>
    mockIsGo ? variants.go : variants.python,
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
  return {
    id: 'doc-1',
    name: 'a.pdf',
    run: RunningStatus.UNSTART,
    progress_msg: '',
    ...overrides,
  };
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
  mockIsGo = false;
});

describe('useShowLog — Python backend is unaffected by the early-log fallback', () => {
  it('never queries pipeline logs and passes document.progress_msg through', () => {
    mockIsGo = false;
    const doc = makeDoc({
      run: RunningStatus.RUNNING,
      progress_msg: 'Parsing chunks',
    });

    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    expect(result.current.logInfo.details).toBe('Parsing chunks');
    expect(result.current.logInfo.status).toBe(RunningStatus.RUNNING);
    expect(mockList).not.toHaveBeenCalled();
  });

  it('leaves an empty progress_msg empty, exactly as before', () => {
    mockIsGo = false;
    const doc = makeDoc({ run: RunningStatus.UNSTART, progress_msg: '' });

    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    // details is the document's own progress_msg, verbatim (the legacy "-"
    // placeholder only applies when there is no source document at all).
    expect(result.current.logInfo.details).toBe(doc.progress_msg);
    expect(mockList).not.toHaveBeenCalled();
  });

  it('keeps the legacy "-" placeholder when there is no source document', () => {
    mockIsGo = false;

    const { result } = renderLogs([]);

    expect(result.current.logInfo.details).toBe('-');
    expect(mockList).not.toHaveBeenCalled();
  });
});

describe('useShowLog — Go backend early-log fallback', () => {
  it('shows the queued message from the early pipeline log row', async () => {
    mockIsGo = true;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: {
        data: {
          logs: [{ document_id: 'doc-1', progress_msg: 'Task is queued...' }],
          total: 1,
        },
      },
    } as any);

    const doc = makeDoc({ ingestion_status: IngestionTaskStatus.CREATED });
    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    await waitFor(() =>
      expect(result.current.logInfo.details).toBe('Task is queued...'),
    );
    expect(result.current.logInfo.status).toBe(RunningStatus.QUEUED);
    expect(mockList).toHaveBeenCalledWith(
      'kb-1',
      expect.objectContaining({ log_type: 'file' }),
    );
  });

  it('ignores a fuzzy name match belonging to another document', async () => {
    mockIsGo = true;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mockList.mockResolvedValue({
      data: {
        data: {
          logs: [{ document_id: 'other-doc', progress_msg: 'someone else' }],
          total: 1,
        },
      },
    } as any);

    const doc = makeDoc({ ingestion_status: IngestionTaskStatus.SCHEDULED });
    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    await waitFor(() => expect(mockList).toHaveBeenCalled());
    expect(result.current.logInfo.details).not.toBe('someone else');
  });

  it('does not query once the document is running', () => {
    mockIsGo = true;
    const doc = makeDoc({
      ingestion_status: IngestionTaskStatus.RUNNING,
      run: RunningStatus.RUNNING,
      progress_msg: 'Indexing done',
    });

    const { result } = renderLogs([doc]);
    act(() => result.current.showLog(doc));

    expect(result.current.logInfo.details).toBe('Indexing done');
    expect(mockList).not.toHaveBeenCalled();
  });
});
