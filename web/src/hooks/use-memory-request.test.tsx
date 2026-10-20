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
import { useFetchAllMemoryList } from './use-memory-request';

const mockGetMemoryList = jest.fn();

jest.mock('@/services/memory-service', () => ({
  __esModule: true,
  default: {
    getMemoryList: (...args: unknown[]) => mockGetMemoryList(...args),
  },
}));

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

describe('useFetchAllMemoryList', () => {
  beforeEach(() => {
    mockGetMemoryList.mockClear();
    mockGetMemoryList.mockResolvedValue({
      data: { data: { memory_list: [] } },
    });
  });

  it('passes tenant_id when an owner tenant is given', async () => {
    renderHook(() => useFetchAllMemoryList('owner-1'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockGetMemoryList).toHaveBeenCalled());
    expect(mockGetMemoryList).toHaveBeenCalledWith(
      {
        params: { page_size: 100, page: 1, tenant_id: 'owner-1' },
        data: {},
      },
      true,
    );
  });

  it('omits tenant_id without an owner tenant', async () => {
    renderHook(() => useFetchAllMemoryList(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockGetMemoryList).toHaveBeenCalled());
    expect(mockGetMemoryList).toHaveBeenCalledWith(
      {
        params: { page_size: 100, page: 1 },
        data: {},
      },
      true,
    );
  });

  it('caches owner-scoped lists separately', async () => {
    const wrapper = createWrapper();
    renderHook(() => useFetchAllMemoryList('owner-1'), { wrapper });
    renderHook(() => useFetchAllMemoryList('owner-2'), { wrapper });

    await waitFor(() => expect(mockGetMemoryList).toHaveBeenCalledTimes(2));
  });
});
