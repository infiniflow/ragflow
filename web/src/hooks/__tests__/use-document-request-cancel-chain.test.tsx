/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { IngestionTaskStatus } from '@/constants/knowledge';
import kbService, { listDocument } from '@/services/knowledge-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import React from 'react';

jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: { documentIngest: jest.fn() },
  listDocument: jest.fn(),
}));

jest.mock('@/components/list-filter-bar/use-handle-filter-submit', () => ({
  useHandleFilterSubmit: () => ({
    filterValue: {},
    handleFilterSubmit: jest.fn(),
    checkValue: jest.fn(),
  }),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));

// `pickByBackend` keeps the real dataset predicates on the Go branch, which is
// what the list's polling and the stop-loss read.
jest.mock('@/utils/backend-variant', () => ({
  useIsGoBackend: () => true,
  pickByBackend: ({ go }: { go: unknown }) => go,
}));

jest.mock('@/hooks/logic-hooks', () => ({
  useGetPaginationWithRouter: () => ({
    pagination: { current: 1, pageSize: 30, total: 0 },
    setPagination: jest.fn(),
  }),
  useHandleSearchChange: () => ({
    searchString: '',
    handleInputChange: jest.fn(),
  }),
}));

jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: () => ({ knowledgeId: 'kb-chain' }),
  useSetPaginationParams: () => ({
    setPaginationParams: jest.fn(),
    page: 1,
    size: 30,
  }),
}));

jest.mock('react-router', () => ({
  useParams: () => ({}),
}));

jest.mock('ahooks', () => ({
  useDebounce: (value: unknown) => value,
}));

// Avoids the module cycle back into use-document-request.
jest.mock('@/pages/dataset/dataset/use-select-filters', () => ({
  EMPTY_METADATA_FIELD: 'empty_metadata',
}));

import { useFetchDocumentList, useRunDocument } from '../use-document-request';

const mockList = jest.mocked(listDocument);
const mockIngest = jest.mocked(kbService.documentIngest);

// Declared locally: an imported binding used in a type position breaks the
// babel pass that hoists jest.mock, and the list only reads these fields.
interface ChainDocument {
  id: string;
  name: string;
  ingestion_status: string;
}

const makeDoc = (id: string, status: string) =>
  ({
    id,
    name: `${id}.pdf`,
    ingestion_status: status,
  }) as unknown as ChainDocument;

const listResult = (doc: ChainDocument) => ({
  data: { code: 0, data: { docs: [doc], total: 1 } },
});

// Drives the same pair of hooks the dataset list page uses, so the cancel
// click, the optimistic update, the poll cadence and the stop-loss all run
// through the real React Query runtime.
const useCancelChain = () => ({
  list: useFetchDocumentList(true),
  run: useRunDocument(),
});

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  // esbuild-jest loads .tsx with the plain "ts" loader here, so JSX in this
  // test file would not transform. Build the provider element with
  // createElement instead.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const Wrapper = (props: { children: any }) =>
    React.createElement(
      QueryClientProvider,
      { client: queryClient },
      props.children,
    );
  return Wrapper;
}

const advanceSeconds = async (seconds: number) => {
  for (let i = 0; i < seconds; i++) {
    await act(async () => {
      jest.advanceTimersByTime(1000);
    });
  }
};

describe('document cancel chain (list polling + stop-loss retry)', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('keeps polling fast after the click and re-sends the cancel once it is overdue', async () => {
    const docId = 'doc-chain-overdue';
    mockList.mockResolvedValueOnce(
      listResult(makeDoc(docId, IngestionTaskStatus.RUNNING)) as never,
    );
    mockList.mockResolvedValue(
      listResult(makeDoc(docId, IngestionTaskStatus.STOPPING)) as never,
    );
    // The cancel request hangs (a 100+ page parse can hold it for minutes),
    // which is exactly the state the stop-loss has to survive.
    mockIngest.mockReturnValue(new Promise(() => {}) as never);

    const { result } = renderHook(useCancelChain, { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.list.documents).toHaveLength(1));
    expect(result.current.list.documents[0].ingestion_status).toBe(
      IngestionTaskStatus.RUNNING,
    );

    await act(async () => {
      void result.current.run.runDocumentByIds({
        documentIds: [docId],
        run: 2,
      });
    });

    // Optimistic STOPPING lands immediately, so the row leaves its idle state
    // before the server has answered.
    await waitFor(() =>
      expect(result.current.list.documents[0].ingestion_status).toBe(
        IngestionTaskStatus.STOPPING,
      ),
    );
    await waitFor(() => expect(mockIngest).toHaveBeenCalledTimes(1));
    expect(mockIngest).toHaveBeenCalledWith({ doc_ids: [docId], run: 2 });

    await advanceSeconds(35);

    // Fast polling for the whole window, then exactly one forced retry that
    // reaches the server while the first request is still pending. Every poll
    // returns a byte-identical stopping row (the stuck case), so the retry must
    // not depend on the list data changing between polls.
    expect(mockList.mock.calls.length).toBeGreaterThanOrEqual(25);
    expect(mockIngest).toHaveBeenCalledTimes(2);
    expect(mockIngest).toHaveBeenLastCalledWith({ doc_ids: [docId], run: 2 });

    // The retry is latched: the overdue window must not produce a request
    // storm, and the poll falls back to the slow cadence.
    const callsAtRetry = mockList.mock.calls.length;
    await advanceSeconds(15);
    expect(mockIngest).toHaveBeenCalledTimes(2);
    expect(mockList.mock.calls.length).toBeLessThan(callsAtRetry + 6);
  });

  it('polls an adopted STOPPING document fast, then slows down without ever re-sending', async () => {
    const docId = 'doc-chain-adopted';
    mockList.mockResolvedValue(
      listResult(makeDoc(docId, IngestionTaskStatus.STOPPING)) as never,
    );

    const { result } = renderHook(useCancelChain, { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.list.documents).toHaveLength(1));

    // A page reload adopts a cancel nobody clicked here: polling stays fast
    // for the window ...
    await advanceSeconds(5);
    expect(mockList.mock.calls.length).toBeGreaterThanOrEqual(5);

    // ... and after the window the interval downgrades instead of hammering
    // the list endpoint forever.
    await advanceSeconds(30);
    const callsAtWindowEnd = mockList.mock.calls.length;
    await advanceSeconds(15);
    expect(mockList.mock.calls.length).toBeLessThan(callsAtWindowEnd + 6);

    expect(mockIngest).not.toHaveBeenCalled();
  });
});
