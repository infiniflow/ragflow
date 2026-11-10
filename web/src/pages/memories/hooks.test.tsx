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
import { MemoryRouter, Route, Routes, useSearchParams } from 'react-router';
import { useFetchMemoryDetail } from './hooks';

const mockGetMemoryConfig = jest.fn();

jest.mock('@/services/memory-service', () => ({
  __esModule: true,
  default: {
    getMemoryConfig: (...args: unknown[]) => mockGetMemoryConfig(...args),
  },
}));

describe('useFetchMemoryDetail', () => {
  beforeEach(() => {
    mockGetMemoryConfig.mockReset();
  });

  it('loads a shared memory through the configuration endpoint', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const responseData = {
      id: 'memory-shared',
      name: 'Shared memory',
      memory_config: { top_k: 6 },
    };
    mockGetMemoryConfig.mockResolvedValue({
      data: { code: 0, data: responseData },
    });
    const wrapper = ({ children }: { children?: unknown }) => (
      <MemoryRouter initialEntries={['/memories?shared_id=memory-shared']}>
        <QueryClientProvider client={queryClient}>
          <Routes>
            <Route path="/memories" element={children as JSX.Element} />
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>
    );

    const { result: paramsResult } = renderHook(
      () => useSearchParams()[0].get('shared_id'),
      { wrapper },
    );
    expect(paramsResult.current).toBe('memory-shared');

    const { result } = renderHook(() => useFetchMemoryDetail('tenant-1'), {
      wrapper,
    });

    await waitFor(() => {
      expect(mockGetMemoryConfig).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(result.current.data).toEqual(responseData);
    });
    expect(mockGetMemoryConfig).toHaveBeenCalledWith('memory-shared');
    queryClient.clear();
  });

  it('loads a memory by route id through the configuration endpoint', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const responseData = {
      id: 'memory-1',
      name: 'Memory',
      memory_config: { top_k: 6 },
    };
    mockGetMemoryConfig.mockResolvedValue({
      data: { code: 0, data: responseData },
    });
    const wrapper = ({ children }: { children?: unknown }) => (
      <MemoryRouter initialEntries={['/memories/memory-1']}>
        <QueryClientProvider client={queryClient}>
          <Routes>
            <Route path="/memories/:id" element={children as JSX.Element} />
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFetchMemoryDetail(), { wrapper });

    await waitFor(() => {
      expect(result.current.data).toEqual(responseData);
    });
    expect(mockGetMemoryConfig).toHaveBeenCalledWith('memory-1');
    queryClient.clear();
  });
});
