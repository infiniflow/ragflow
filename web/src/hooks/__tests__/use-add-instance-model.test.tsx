/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

const mockAddInstanceModel = jest.fn();
const mockDeleteInstanceModels = jest.fn();

jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: {
    addInstanceModel: (...args: unknown[]) => mockAddInstanceModel(...args),
    deleteInstanceModels: (...args: unknown[]) =>
      mockDeleteInstanceModels(...args),
  },
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn() },
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/hooks/use-warn-empty-model', () => ({
  useWarnEmptyModel: () => jest.fn(),
}));

import {
  LlmKeys,
  useAddInstanceModel,
  useDeleteInstanceModels,
} from '@/hooks/use-llm-request';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { createElement } from 'react';

describe('useAddInstanceModel', () => {
  it('acknowledges the added model in cache before invalidation refetches', async () => {
    mockAddInstanceModel.mockResolvedValue({ data: { code: 0 } });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { result } = renderHook(() => useAddInstanceModel(), { wrapper });

    await act(async () => {
      await result.current.addInstanceModel({
        provider_name: 'Bedrock',
        instance_name: 'saved',
        model_name: 'amazon.nova-lite-v1:0',
        model_type: ['chat'],
        max_tokens: 8192,
        extra: { is_tools: true },
      });
    });

    expect(
      queryClient.getQueryData(LlmKeys.instanceModels('Bedrock', 'saved')),
    ).toEqual([
      expect.objectContaining({
        name: 'amazon.nova-lite-v1:0',
        model_type: ['chat'],
        max_tokens: 8192,
        is_tools: true,
      }),
    ]);
  });

  it('acknowledges deleted models in cache before invalidation refetches', async () => {
    mockDeleteInstanceModels.mockResolvedValue({ data: { code: 0 } });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    queryClient.setQueryData(LlmKeys.instanceModels('Bedrock', 'saved'), [
      { name: 'model-a', model_type: ['chat'], max_tokens: 8192 },
      { name: 'model-b', model_type: ['chat'], max_tokens: 8192 },
    ]);
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { result } = renderHook(() => useDeleteInstanceModels(), { wrapper });

    await act(async () => {
      await result.current.deleteInstanceModels({
        provider_name: 'Bedrock',
        instance_name: 'saved',
        model_name: ['model-a'],
      });
    });

    expect(
      queryClient.getQueryData(LlmKeys.instanceModels('Bedrock', 'saved')),
    ).toEqual([expect.objectContaining({ name: 'model-b' })]);
  });
});
