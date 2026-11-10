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
import type { PropsWithChildren } from 'react';
import kbService from '@/services/knowledge-service';
import { useFetchKnowledgeMetadataKeys } from '../use-knowledge-request';

describe('useFetchKnowledgeMetadataKeys', () => {
  it('derives keys from the Go metadata endpoint with query parameters', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const getMeta = jest.spyOn(kbService, 'getMeta').mockResolvedValue({
      data: {
        code: 0,
        data: {
          author: { Alice: ['doc-1'] },
          year: { '2026': ['doc-1'] },
        },
      },
    } as never);
    const wrapper = ({ children }: PropsWithChildren) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(
      () => useFetchKnowledgeMetadataKeys(['dataset-2', 'dataset-1']),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.data).toEqual(['author', 'year']);
    });
    expect(getMeta).toHaveBeenCalledWith({
      dataset_ids: 'dataset-1,dataset-2',
    });
    queryClient.clear();
    getMeta.mockRestore();
  });
});
