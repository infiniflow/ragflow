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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import {
  useFetchDatasetsByIds,
  useFetchKnowledgeList,
  useFetchKnowledgeMetadataKeys,
} from './use-knowledge-request';

const mockListDataset = jest.fn();
const mockListDatasetByIds = jest.fn();
const mockGetMeta = jest.fn();

jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  listDataset: (...args: unknown[]) => mockListDataset(...args),
  listDatasetByIds: (...args: unknown[]) => mockListDatasetByIds(...args),
  default: {
    getMeta: (...args: unknown[]) => mockGetMeta(...args),
  },
}));

// route-hook (imported by the hook chain) pulls in the app shell — routes
// constructs a browser router at module scope, which jsdom cannot survive.
jest.mock('@/routes', () => ({ Routes: {} }));

const DatasetId = '0cb944c66320448aaae827ba8009ef68';

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // `any` on purpose: files with jest.mock go through esbuild-jest's babel
  // pass, which cannot strip imported bindings used as type annotations.
  return function Wrapper({ children }: { children?: any }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe('useFetchDatasetsByIds', () => {
  beforeEach(() => {
    mockListDatasetByIds.mockClear();
    mockListDatasetByIds.mockResolvedValue({ data: { data: [] } });
  });

  it('passes the canvas id through to the by-ids lookup', async () => {
    renderHook(() => useFetchDatasetsByIds([DatasetId], 'canvas-1'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDatasetByIds).toHaveBeenCalled());
    expect(mockListDatasetByIds).toHaveBeenCalledWith(
      [DatasetId],
      'canvas-1',
      undefined,
    );
  });

  it('leaves the canvas id undefined for global lookups', async () => {
    renderHook(() => useFetchDatasetsByIds([DatasetId]), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDatasetByIds).toHaveBeenCalled());
    expect(mockListDatasetByIds).toHaveBeenCalledWith(
      [DatasetId],
      undefined,
      undefined,
    );
  });
});

describe('useFetchKnowledgeList', () => {
  beforeEach(() => {
    mockListDataset.mockClear();
    mockListDataset.mockResolvedValue({
      data: { data: [], total_datasets: 0 },
    });
  });

  it('adds canvas_id to the list params when a canvas is given', async () => {
    renderHook(() => useFetchKnowledgeList(false, '', 30, 'canvas-1'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDataset).toHaveBeenCalled());
    expect(mockListDataset).toHaveBeenCalledWith(
      expect.objectContaining({
        page: 1,
        page_size: 30,
        canvas_id: 'canvas-1',
      }),
    );
  });

  it('omits canvas_id without a canvas', async () => {
    renderHook(() => useFetchKnowledgeList(false, 'kw'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDataset).toHaveBeenCalled());
    const params = mockListDataset.mock.calls[0][0];
    expect(params).not.toHaveProperty('canvas_id');
    expect(params).toMatchObject({ page: 1, keywords: 'kw' });
  });
});

describe('useFetchKnowledgeMetadataKeys', () => {
  beforeEach(() => {
    mockGetMeta.mockClear();
    mockGetMeta.mockResolvedValue({
      data: { data: { author: { alice: ['doc-1'] }, year: { '2024': [] } } },
    });
  });

  it('requests dataset_ids and returns only the field names', async () => {
    const { result } = renderHook(
      () => useFetchKnowledgeMetadataKeys(['b', 'a']),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(mockGetMeta).toHaveBeenCalled());
    // The Go handler requires `dataset_ids` (not `kb_ids`); ids are sorted for
    // a stable query key.
    expect(mockGetMeta).toHaveBeenCalledWith({ dataset_ids: 'a,b' });
    await waitFor(() =>
      expect(result.current.data).toEqual(['author', 'year']),
    );
  });
});
