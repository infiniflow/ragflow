import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useCopyBuiltinPipeline } from './use-copy-builtin-pipeline';

jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: {
    getBuiltinPipeline: jest.fn(),
    createAgent: jest.fn(),
  },
}));

// Mock the heavy agent-request module so its transitive react-router/routes
// import (which needs browser globals not present under jest) is not loaded.
// Only the query-key builders used for cache invalidation are needed here.
jest.mock('@/hooks/use-agent-request', () => ({
  AgentKeys: {
    list: () => ['fetchAgentListByPage'],
    filters: () => ['fetchAgentFilters'],
  },
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
  it('fetches the builtin DSL and creates a user-owned dataflow pipeline', async () => {
    agentService.getBuiltinPipeline.mockResolvedValue({
      data: { code: 0, message: 'ok', data: { dsl: { components: {} } } },
    });
    agentService.createAgent.mockResolvedValue({
      data: { code: 0, message: 'ok', data: null },
    });

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    const ret = await result.current.copy({ id: 'general', title: 'General' });

    expect(agentService.getBuiltinPipeline).toHaveBeenCalledWith('general');
    expect(agentService.createAgent).toHaveBeenCalledWith({
      title: 'General (Copy)',
      dsl: { components: {} },
      canvas_category: 'dataflow_canvas',
    });
    expect(message.success).toHaveBeenCalled();
    expect(message.error).not.toHaveBeenCalled();
    expect(ret).not.toBeNull();
  });

  it('shows an error and does not create when the builtin DSL is missing', async () => {
    agentService.getBuiltinPipeline.mockResolvedValue({
      data: { code: 0, message: 'ok', data: { dsl: undefined } },
    });

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    const ret = await result.current.copy({ id: 'general', title: 'General' });

    expect(agentService.createAgent).not.toHaveBeenCalled();
    expect(message.error).toHaveBeenCalled();
    expect(ret).toBeNull();
  });

  it('shows an error when createAgent reports a failure code', async () => {
    agentService.getBuiltinPipeline.mockResolvedValue({
      data: { code: 0, message: 'ok', data: { dsl: { components: {} } } },
    });
    agentService.createAgent.mockResolvedValue({
      data: { code: 1, message: 'boom', data: null },
    });

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    await result.current.copy({ id: 'general', title: 'General' });

    expect(message.error).toHaveBeenCalledWith('boom');
    expect(message.success).not.toHaveBeenCalled();
  });

  it('shows a request error when the fetch rejects', async () => {
    agentService.getBuiltinPipeline.mockRejectedValue(new Error('network'));

    const { result } = renderHook(() => useCopyBuiltinPipeline(), {
      wrapper,
    });
    await result.current.copy({ id: 'general', title: 'General' });

    expect(message.error).toHaveBeenCalled();
    expect(agentService.createAgent).not.toHaveBeenCalled();
  });
});
