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

import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AgentCategory } from '@/constants/agent';
import { useCopyBuiltinPipeline } from './use-copy-builtin-pipeline';

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: {
    getBuiltinPipeline: jest.fn(),
    createAgent: jest.fn(),
  },
}));

const mockNavigate = jest.fn();
const mockNavigateToAgent = jest.fn(() => mockNavigate);
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({ navigateToAgent: mockNavigateToAgent }),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: {
    success: jest.fn(),
    error: jest.fn(),
  },
}));

jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: {
    t: (key: string, opts?: { defaultValue?: string }) =>
      opts?.defaultValue ?? key,
  },
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentService = require('@/services/agent-service').default;
// eslint-disable-next-line @typescript-eslint/no-var-requires
const message = require('@/components/ui/message').default;

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    {children}
  </QueryClientProvider>
);

beforeEach(() => {
  jest.clearAllMocks();
});

describe('useCopyBuiltinPipeline', () => {
  it('fetches the builtin DSL, creates a user-owned pipeline and opens it for editing', async () => {
    agentService.getBuiltinPipeline.mockResolvedValue({
      data: { code: 0, message: 'ok', data: { dsl: { components: {} } } },
    });
    agentService.createAgent.mockResolvedValue({
      data: { code: 0, message: 'ok', data: { id: 'new-canvas' } },
    });

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    let ret;
    await act(async () => {
      ret = await result.current.copy({ id: 'general', title: 'General' });
    });

    expect(agentService.getBuiltinPipeline).toHaveBeenCalledWith('general');
    expect(agentService.createAgent).toHaveBeenCalledWith({
      title: 'General (Copy)',
      dsl: { components: {} },
      canvas_category: 'dataflow_canvas',
    });
    expect(message.success).toHaveBeenCalled();
    expect(message.error).not.toHaveBeenCalled();
    expect(mockNavigateToAgent).toHaveBeenCalledWith(
      'new-canvas',
      AgentCategory.DataflowCanvas,
    );
    expect(mockNavigate).toHaveBeenCalledTimes(1);
    expect(ret).not.toBeNull();
  });

  it('shows an error and does not create or navigate when the builtin DSL is missing', async () => {
    agentService.getBuiltinPipeline.mockResolvedValue({
      data: { code: 0, message: 'ok', data: { dsl: undefined } },
    });

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    let ret;
    await act(async () => {
      ret = await result.current.copy({ id: 'general', title: 'General' });
    });

    expect(agentService.createAgent).not.toHaveBeenCalled();
    expect(message.error).toHaveBeenCalled();
    expect(mockNavigateToAgent).not.toHaveBeenCalled();
    expect(ret).toBeNull();
  });

  it('shows an error and stays on the list when createAgent reports a failure code', async () => {
    agentService.getBuiltinPipeline.mockResolvedValue({
      data: { code: 0, message: 'ok', data: { dsl: { components: {} } } },
    });
    agentService.createAgent.mockResolvedValue({
      data: { code: 1, message: 'boom', data: null },
    });

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    await act(async () => {
      await result.current.copy({ id: 'general', title: 'General' });
    });

    expect(message.error).toHaveBeenCalledWith('boom');
    expect(message.success).not.toHaveBeenCalled();
    expect(mockNavigateToAgent).not.toHaveBeenCalled();
  });

  it('shows a request error when the fetch rejects', async () => {
    agentService.getBuiltinPipeline.mockRejectedValue(new Error('network'));

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    await act(async () => {
      await result.current.copy({ id: 'general', title: 'General' });
    });

    expect(message.error).toHaveBeenCalled();
    expect(agentService.createAgent).not.toHaveBeenCalled();
    expect(mockNavigateToAgent).not.toHaveBeenCalled();
  });
});
