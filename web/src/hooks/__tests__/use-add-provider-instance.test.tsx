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
import { act, renderHook, waitFor } from '@testing-library/react';

import {
  LlmKeys,
  useAddProviderInstance,
  useFetchInstanceModels,
} from '../use-llm-request';

const mockListProviders = jest.fn();
const mockListProviderInstances = jest.fn();
const mockAddProviderInstance = jest.fn();
const mockListInstanceModels = jest.fn();

jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: {
    listProviders: (...args: unknown[]) => mockListProviders(...args),
    listProviderInstances: (...args: unknown[]) =>
      mockListProviderInstances(...args),
    addProviderInstance: (...args: unknown[]) =>
      mockAddProviderInstance(...args),
    listInstanceModels: (...args: unknown[]) => mockListInstanceModels(...args),
  },
}));

describe('useFetchInstanceModels', () => {
  it('rejects a non-zero response code as a failed snapshot', async () => {
    mockListInstanceModels.mockResolvedValue({
      data: { code: 102, data: null, message: 'failed to list models' },
    });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(
      () => useFetchInstanceModels('Bedrock', 'saved-instance'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.isSuccess).toBe(false);
  });
});

jest.mock('../use-warn-empty-model', () => ({
  useWarnEmptyModel: jest.fn(),
}));

describe('useAddProviderInstance', () => {
  it('invalidates the added-model catalog after creating an instance', async () => {
    mockListProviders.mockResolvedValue({
      data: { code: 0, data: [{ name: 'Bedrock' }] },
    });
    mockListProviderInstances.mockResolvedValue({
      data: { code: 0, data: [] },
    });
    mockAddProviderInstance.mockResolvedValue({
      data: { code: 0, data: null },
    });

    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    });
    const invalidateQueries = jest.spyOn(queryClient, 'invalidateQueries');
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useAddProviderInstance(), { wrapper });

    await act(async () => {
      await result.current.addProviderInstance({
        llm_factory: 'Bedrock',
        instance_name: 'bedrock-api-key',
        api_key: {
          auth_mode: 'bedrock_api_key',
          bedrock_api_key: 'test-key',
          bedrock_region: 'us-east-1',
        },
        max_tokens: 8192,
        model_info: [
          {
            model_name: 'amazon.nova-lite-v1:0',
            model_type: ['chat'],
            max_tokens: 8192,
          },
        ],
      });
    });

    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: LlmKeys.allModels(),
    });
  });
});
