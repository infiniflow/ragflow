import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { GenerateType } from '@/constants/knowledge';

import { useTraceRunData } from '../use-dataset-generate';

jest.mock('react-router', () => ({
  useParams: jest.fn(() => ({ id: 'kb1' })),
}));

jest.mock('@/utils/api-proxy-scheme', () => ({
  isGoDatasetBackend: jest.fn(() => true),
}));

jest.mock('@/services/knowledge-service', () => ({
  getDatasetCompilationStatus: jest.fn(),
}));

// use-dataset-generate imports agent-service (and transitively register-server /
// next-request / locales config that touch import.meta.env). Mock it so the
// Go status path under test doesn't pull in that module graph.
jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: { cancelDataflow: jest.fn(), deletePipelineTask: jest.fn() },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { getDatasetCompilationStatus } from '@/services/knowledge-service';
import { isGoDatasetBackend } from '@/utils/api-proxy-scheme';

const mockStatus = jest.mocked(getDatasetCompilationStatus);

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  // esbuild-jest config here loads .ts with the "tsx" loader but .tsx with the
  // plain "ts" loader, so JSX in this test file would not transform. Build the
  // provider element with createElement instead.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const Wrapper = (props: { children: any }) =>
    React.createElement(
      QueryClientProvider,
      { client: queryClient },
      props.children,
    );
  return Wrapper;
}

describe('useTraceRunData (Go/hybrid compile-status contract)', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (isGoDatasetBackend as jest.Mock).mockReturnValue(true);
  });

  it('maps a successful status to the scheduler contract fields', async () => {
    mockStatus.mockResolvedValue({
      data: {
        code: 0,
        data: { state: 'running', inflight: 2, backlog: 1, error: '' },
      },
    } as never);

    const { result } = renderHook(
      () => useTraceRunData(GenerateType.Artifact),
      {
        wrapper: makeWrapper(),
      },
    );

    await waitFor(() => expect(result.current.isPending).toBe(false), {
      timeout: 5000,
    });

    const info = result.current.data;
    expect(info?.compilationState).toBe('running');
    expect(info?.inflight).toBe(2);
    expect(info?.backlog).toBe(1);
    expect(info?.compilationError).toBe('');
  });

  it('rejects a non-zero business code instead of mapping to idle', async () => {
    mockStatus.mockResolvedValue({
      data: { code: 1, message: 'no authorization', data: undefined },
    } as never);

    const { result } = renderHook(
      () => useTraceRunData(GenerateType.Artifact),
      {
        wrapper: makeWrapper(),
      },
    );

    // The query configures retry: 3 with a 1s delay, so the terminal error state
    // arrives after the retries drain.
    await waitFor(() => expect(result.current.isError).toBe(true), {
      timeout: 10000,
    });

    expect(result.current.error).toBeTruthy();
    // The error must not be silently treated as an idle/empty status.
    expect(result.current.data).toBeUndefined();
  });
});
