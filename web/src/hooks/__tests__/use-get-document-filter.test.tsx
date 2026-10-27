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

import kbService, {
  createDocument,
  deleteDocument,
  documentFilter,
} from '@/services/knowledge-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router';
import {
  useCreateDocument,
  useGetDocumentFilter,
  useRemoveDocument,
  useRunDocument,
} from '../use-document-request';

jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: { documentIngest: jest.fn() },
  createDocument: jest.fn(),
  deleteDocument: jest.fn(),
  documentFilter: jest.fn(),
}));

// src/routes.tsx builds a browser router at module scope, which jsdom cannot
// evaluate; the hooks under test only need the route enum.
jest.mock('@/routes', () => ({
  Routes: { DataflowResult: '/dataflow-result' },
}));

const mockDocumentFilter = jest.mocked(documentFilter);
const mockDeleteDocument = jest.mocked(deleteDocument);
const mockCreateDocument = jest.mocked(createDocument);
const mockDocumentIngest = jest.mocked(kbService.documentIngest);

const createTestQueryClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } });

let queryClient = createTestQueryClient();

let initialEntry = '/dataset/files/dataset-id';

let navigate: ((to: string) => void) | undefined;

// Switching datasets keeps this route -- and so the hook -- mounted, which is
// the state the popover's open flag has to survive.
function NavigationProbe() {
  navigate = useNavigate();
  return null;
}

const wrapper = ({ children }: { children: React.ReactNode }) => {
  return (
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route
          path="/dataset/files/:id"
          element={
            <QueryClientProvider client={queryClient}>
              <NavigationProbe />
              {children}
            </QueryClientProvider>
          }
        />
      </Routes>
    </MemoryRouter>
  );
};

describe('useGetDocumentFilter', () => {
  beforeEach(() => {
    queryClient = createTestQueryClient();
    initialEntry = '/dataset/files/dataset-id';
    mockDocumentFilter.mockReset();
    mockDocumentFilter.mockResolvedValue({
      data: { code: 0, data: { filter: { suffix: { pdf: 1 } } } },
    } as never);
  });

  it('leaves the facet counts unfetched until the filter popover opens', async () => {
    const { result } = renderHook(() => useGetDocumentFilter(), { wrapper });

    await waitFor(() => expect(result.current.filter).toBeDefined());
    expect(mockDocumentFilter).not.toHaveBeenCalled();

    act(() => {
      result.current.onOpenChange(true);
    });

    await waitFor(() => expect(mockDocumentFilter).toHaveBeenCalledTimes(1));
    expect(mockDocumentFilter).toHaveBeenCalledWith('dataset-id');
  });

  it('reports loading only while the first fetch is in flight', async () => {
    const { result } = renderHook(() => useGetDocumentFilter(), { wrapper });

    expect(result.current.loading).toBe(false);

    act(() => {
      result.current.onOpenChange(true);
    });

    await waitFor(() => expect(result.current.loading).toBe(true));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.filter).toEqual({
      suffix: { pdf: 1 },
      run_status: {},
    });
  });

  it('reuses the cached counts while they are still fresh', async () => {
    const { result } = renderHook(() => useGetDocumentFilter(), { wrapper });

    act(() => {
      result.current.onOpenChange(true);
    });
    await waitFor(() =>
      expect(result.current.filter).toEqual({
        suffix: { pdf: 1 },
        run_status: {},
      }),
    );
    expect(mockDocumentFilter).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.onOpenChange(false);
      result.current.onOpenChange(true);
    });

    expect(mockDocumentFilter).toHaveBeenCalledTimes(1);
  });

  it('leaves the next dataset unfetched until its own popover opens', async () => {
    const { result } = renderHook(() => useGetDocumentFilter(), { wrapper });

    act(() => {
      result.current.onOpenChange(true);
    });
    await waitFor(() => expect(mockDocumentFilter).toHaveBeenCalledTimes(1));

    // The file list rerenders for the new dataset without unmounting the hook.
    await act(async () => {
      navigate?.('/dataset/files/other-dataset-id');
    });

    expect(mockDocumentFilter).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.onOpenChange(true);
    });

    await waitFor(() => expect(mockDocumentFilter).toHaveBeenCalledTimes(2));
    expect(mockDocumentFilter).toHaveBeenLastCalledWith('other-dataset-id');
  });

  it('refreshes the counts when the popover is reopened after they went stale', async () => {
    const { result } = renderHook(() => useGetDocumentFilter(), { wrapper });

    act(() => {
      result.current.onOpenChange(true);
    });
    await waitFor(() =>
      expect(result.current.filter).toEqual({
        suffix: { pdf: 1 },
        run_status: {},
      }),
    );

    const nowSpy = jest
      .spyOn(Date, 'now')
      .mockReturnValue(Date.now() + 60 * 1000);
    try {
      act(() => {
        result.current.onOpenChange(true);
      });
      await waitFor(() => expect(mockDocumentFilter).toHaveBeenCalledTimes(2));
    } finally {
      nowSpy.mockRestore();
    }
  });

  // Opens the popover, closes it, applies a change to the documents, then
  // checks the counts stay unfetched until the popover is reopened.
  async function expectRefreshOnReopenAfter(
    useChange: () => () => Promise<unknown>,
  ) {
    const { result } = renderHook(
      () => ({ filter: useGetDocumentFilter(), change: useChange() }),
      { wrapper },
    );

    act(() => {
      result.current.filter.onOpenChange(true);
    });
    await waitFor(() => expect(mockDocumentFilter).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.filter.onOpenChange(false);
    });
    await act(async () => {
      await result.current.change();
    });

    // The counts are only marked stale: nothing is fetched while the popover
    // stays closed.
    expect(mockDocumentFilter).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.filter.onOpenChange(true);
    });
    await waitFor(() => expect(mockDocumentFilter).toHaveBeenCalledTimes(2));
  }

  it('refreshes the counts on the next open after the documents changed', async () => {
    mockDeleteDocument.mockResolvedValue({ data: { code: 0 } } as never);
    await expectRefreshOnReopenAfter(() => {
      const { removeDocument } = useRemoveDocument();
      return () => removeDocument('doc-1');
    });
  });

  it('refreshes the counts on the next open after a parse was started', async () => {
    mockDocumentIngest.mockResolvedValue({ data: { code: 0 } } as never);
    await expectRefreshOnReopenAfter(() => {
      const { runDocumentByIds } = useRunDocument();
      return () => runDocumentByIds({ documentIds: ['doc-1'], run: 1 });
    });
  });

  it('refreshes the counts on the next open after a document was created on a later page', async () => {
    initialEntry = '/dataset/files/dataset-id?page=2';
    mockCreateDocument.mockResolvedValue({ code: 0 } as never);
    await expectRefreshOnReopenAfter(() => {
      const { createDocument } = useCreateDocument();
      return () => createDocument('new.txt');
    });
  });
});
