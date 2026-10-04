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

import { useDocumentImageUrl } from '@/components/image';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useCreateChunk } from '../use-chunk-request';

const mockSetChunk = jest.fn();

jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {
    setChunk: (...args: unknown[]) => mockSetChunk(...args),
  },
}));

// route-hook reads react-router's search params, which jsdom has no router for.
jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: () => ({
    type: '',
    documentId: 'doc-1',
    knowledgeId: 'kb-1',
  }),
}));

// route-hook (imported by the hook chain) pulls in the app shell — routes
// constructs a browser router at module scope, which jsdom cannot survive.
jest.mock('@/routes', () => ({ Routes: {} }));

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

describe('useCreateChunk image eviction', () => {
  let fetchMock: jest.Mock;

  beforeEach(() => {
    mockSetChunk.mockReset();
    mockSetChunk.mockResolvedValue({ data: { code: 0 } });
    fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['image-bytes']),
    });
    global.fetch = fetchMock as unknown as typeof fetch;
    URL.createObjectURL = jest.fn(
      () => 'blob:cached',
    ) as unknown as typeof URL.createObjectURL;
    URL.revokeObjectURL = jest.fn() as unknown as typeof URL.revokeObjectURL;
  });

  it('drops the cached image of the chunk it updated', async () => {
    const wrapper = createWrapper();
    // The card renders the backend's img_id, `<dataset_id>-<chunk_id>`.
    const image = renderHook(
      () => useDocumentImageUrl('kb-1-chunk-1', 'doc-1'),
      { wrapper },
    );
    await waitFor(() => expect(image.result.current).toBeTruthy());
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const { result } = renderHook(() => useCreateChunk(), { wrapper });
    await act(async () => {
      await result.current.createChunk({
        chunk_id: 'chunk-1',
        doc_id: 'doc-1',
        kb_id: 'kb-1',
        image_base64: 'aGVsbG8=',
      });
    });
    expect(mockSetChunk).toHaveBeenCalledTimes(1);

    renderHook(() => useDocumentImageUrl('kb-1-chunk-1', 'doc-1'), { wrapper });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  });
});
