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

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { AgentCategory } from '@/constants/agent';
import {
  useFetchAgentFilters,
  useFetchAgentListByPage,
} from '../use-agent-request';

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: {
    listAgents: jest.fn(),
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
  agentService.listAgents.mockResolvedValue({
    data: { data: { canvas: [], total: 0 } },
  });
});

describe('useFetchAgentListByPage', () => {
  it('requests a single canvas_category for the active tab', async () => {
    renderHook(() => useFetchAgentListByPage(AgentCategory.DataflowCanvas), {
      wrapper,
    });

    await waitFor(() => expect(agentService.listAgents).toHaveBeenCalled());
    expect(agentService.listAgents).toHaveBeenCalledWith(
      {
        params: {
          page: 1,
          page_size: 50,
          canvas_category: AgentCategory.DataflowCanvas,
        },
      },
      true,
    );
  });

  it('requests the compilation template groups category', async () => {
    renderHook(() => useFetchAgentListByPage('compilation_template_group'), {
      wrapper,
    });

    await waitFor(() => expect(agentService.listAgents).toHaveBeenCalled());
    expect(
      agentService.listAgents.mock.calls[0][0].params.canvas_category,
    ).toBe('compilation_template_group');
  });

  it('keeps the home view unscoped when no category is given', async () => {
    renderHook(() => useFetchAgentListByPage(), { wrapper });

    await waitFor(() => expect(agentService.listAgents).toHaveBeenCalled());
    expect(agentService.listAgents.mock.calls[0][0].params).not.toHaveProperty(
      'canvas_category',
    );
  });
});

describe('useFetchAgentFilters', () => {
  beforeEach(() => {
    agentService.listAgents.mockResolvedValue({
      data: { data: { filter: { owner: [], canvas_category: [] }, total: 0 } },
    });
  });

  it('passes the active tab category to the filter aggregation', async () => {
    renderHook(() => useFetchAgentFilters(AgentCategory.DataflowCanvas), {
      wrapper,
    });

    await waitFor(() => expect(agentService.listAgents).toHaveBeenCalled());
    expect(agentService.listAgents).toHaveBeenCalledWith(
      {
        params: {
          type: 'filter',
          canvas_category: AgentCategory.DataflowCanvas,
        },
      },
      true,
    );
  });

  it('requests the unscoped aggregation without a category', async () => {
    renderHook(() => useFetchAgentFilters(), { wrapper });

    await waitFor(() => expect(agentService.listAgents).toHaveBeenCalled());
    expect(agentService.listAgents).toHaveBeenCalledWith(
      { params: { type: 'filter' } },
      true,
    );
  });
});
