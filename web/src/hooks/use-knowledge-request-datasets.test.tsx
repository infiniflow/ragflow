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
} from './use-knowledge-request';

const mockListDataset = jest.fn();
const mockListDatasetByIds = jest.fn();

jest.mock('@/services/knowledge-service', () => ({
  listDataset: (...args: unknown[]) => mockListDataset(...args),
  listDatasetByIds: (...args: unknown[]) => mockListDatasetByIds(...args),
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

  it('passes the owner tenant through to the by-ids lookup', async () => {
    renderHook(() => useFetchDatasetsByIds([DatasetId], 'owner-1'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDatasetByIds).toHaveBeenCalled());
    expect(mockListDatasetByIds).toHaveBeenCalledWith([DatasetId], 'owner-1');
  });

  it('leaves the owner tenant undefined for own-canvas lookups', async () => {
    renderHook(() => useFetchDatasetsByIds([DatasetId]), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDatasetByIds).toHaveBeenCalled());
    expect(mockListDatasetByIds).toHaveBeenCalledWith([DatasetId], undefined);
  });
});

describe('useFetchKnowledgeList', () => {
  beforeEach(() => {
    mockListDataset.mockClear();
    mockListDataset.mockResolvedValue({
      data: { data: [], total_datasets: 0 },
    });
  });

  it('adds tenant_id to the list params when an owner tenant is given', async () => {
    renderHook(() => useFetchKnowledgeList(false, '', 30, 'owner-1'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDataset).toHaveBeenCalled());
    expect(mockListDataset).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, page_size: 30, tenant_id: 'owner-1' }),
    );
  });

  it('omits tenant_id without an owner tenant', async () => {
    renderHook(() => useFetchKnowledgeList(false, 'kw'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockListDataset).toHaveBeenCalled());
    const params = mockListDataset.mock.calls[0][0];
    expect(params).not.toHaveProperty('tenant_id');
    expect(params).toMatchObject({ page: 1, keywords: 'kw' });
  });
});
