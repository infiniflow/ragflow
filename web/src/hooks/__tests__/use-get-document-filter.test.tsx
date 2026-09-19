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

import { documentFilter } from '@/services/knowledge-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { useGetDocumentFilter } from '../use-document-request';

jest.mock('@/services/knowledge-service', () => ({
  documentFilter: jest.fn(),
}));

// src/routes.tsx builds a browser router at module scope, which jsdom cannot
// evaluate; the hooks under test only need the route enum.
jest.mock('@/routes', () => ({
  Routes: { DataflowResult: '/dataflow-result' },
}));

const mockDocumentFilter = jest.mocked(documentFilter);

const createTestQueryClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } });

let queryClient = createTestQueryClient();

const wrapper = ({ children }: { children: React.ReactNode }) => {
  return (
    <MemoryRouter initialEntries={['/dataset/files/dataset-id']}>
      <Routes>
        <Route
          path="/dataset/files/:id"
          element={
            <QueryClientProvider client={queryClient}>
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
    expect(result.current.filter).toEqual({ suffix: { pdf: 1 } });
  });

  it('reuses the cached counts while they are still fresh', async () => {
    const { result } = renderHook(() => useGetDocumentFilter(), { wrapper });

    act(() => {
      result.current.onOpenChange(true);
    });
    await waitFor(() =>
      expect(result.current.filter).toEqual({ suffix: { pdf: 1 } }),
    );
    expect(mockDocumentFilter).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.onOpenChange(false);
      result.current.onOpenChange(true);
    });

    expect(mockDocumentFilter).toHaveBeenCalledTimes(1);
  });

  it('refreshes the counts when the popover is reopened after they went stale', async () => {
    const { result } = renderHook(() => useGetDocumentFilter(), { wrapper });

    act(() => {
      result.current.onOpenChange(true);
    });
    await waitFor(() =>
      expect(result.current.filter).toEqual({ suffix: { pdf: 1 } }),
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
});
