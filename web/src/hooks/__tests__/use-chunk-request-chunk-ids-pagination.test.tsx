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
import { MemoryRouter } from 'react-router';
import { useFetchNextChunkList } from '../use-chunk-request';

const mockChunkList = jest.fn();

jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {
    chunkList: (...args: unknown[]) => mockChunkList(...args),
  },
}));

// Keep the real pagination hooks, which read page/size from the URL; only the
// dataset/document ids come from a fixed value.
jest.mock('@/hooks/route-hook', () => ({
  ...jest.requireActual('@/hooks/route-hook'),
  useGetKnowledgeSearchParams: () => ({
    type: '',
    documentId: 'doc-1',
    knowledgeId: 'kb-1',
  }),
}));

// route-hook pulls in the app shell; routes constructs a browser router at
// module scope, which jsdom cannot survive.
jest.mock('@/routes', () => ({ Routes: {} }));

function createWrapper(url: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children?: React.ReactNode }) {
    return (
      <MemoryRouter initialEntries={[url]}>
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      </MemoryRouter>
    );
  };
}

describe('useFetchNextChunkList with a chunk id filter', () => {
  beforeEach(() => {
    mockChunkList.mockReset();
    mockChunkList.mockResolvedValue({
      data: { code: 0, data: { chunks: [], total: 30, doc: {} } },
    });
  });

  it('requests the page the pagination control is on', async () => {
    const chunkIds = Array.from({ length: 30 }, (_, i) => `chunk-${i}`);

    renderHook(() => useFetchNextChunkList(true, { chunkIds }), {
      wrapper: createWrapper('/chunk?page=2&size=10'),
    });

    await waitFor(() => expect(mockChunkList).toHaveBeenCalled());
    expect(mockChunkList).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, size: 10, chunk_ids: chunkIds }),
    );
  });
});
