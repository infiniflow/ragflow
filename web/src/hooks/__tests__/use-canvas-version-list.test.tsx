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
import { useFetchCanvasVersionList } from '../use-agent-request';

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: {
    fetchVersionList: jest.fn(),
  },
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentService = require('@/services/agent-service').default;

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <MemoryRouter>
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      {children}
    </QueryClientProvider>
  </MemoryRouter>
);

beforeEach(() => {
  jest.clearAllMocks();
});

describe('useFetchCanvasVersionList', () => {
  it('keeps pending data undefined until versions arrive', async () => {
    let resolveFetch: (value: unknown) => void = () => undefined;
    agentService.fetchVersionList.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const { result } = renderHook(() => useFetchCanvasVersionList('canvas-1'), {
      wrapper,
    });

    // While the request is in flight there is no version data: an empty list
    // here would mean "loaded and empty", i.e. the pipeline has no versions.
    expect(result.current.data).toBeUndefined();

    resolveFetch({ data: { data: [{ id: 'version-1' }] } });

    await waitFor(() => {
      expect(result.current.data).toEqual([{ id: 'version-1' }]);
    });
    expect(result.current.data).toEqual([{ id: 'version-1' }]);
  });

  it('stays disabled without a canvas id', () => {
    const { result } = renderHook(() => useFetchCanvasVersionList(undefined), {
      wrapper,
    });

    expect(result.current.data).toBeUndefined();
    expect(agentService.fetchVersionList).not.toHaveBeenCalled();
  });
});
