import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { useFetchSessionManually as useAgentSession } from '../use-agent-request';
import { useFetchSessionManually as useChatSession } from '../use-chat-request';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: () => ({ id: 'resource' }),
}));
jest.mock('@/services/next-chat-service', () => ({
  __esModule: true,
  default: { getSession: jest.fn() },
}));
jest.mock('@/services/agent-service', () => ({
  __esModule: true,
  default: {},
  fetchAgentLogsById: jest.fn(),
}));

const chatService = require('@/services/next-chat-service').default;
const agentService = require('@/services/agent-service');

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    {children}
  </QueryClientProvider>
);

it('requests references and prompts for chat history displayed in the UI', async () => {
  chatService.getSession.mockResolvedValue({
    data: { data: { messages: [] } },
  });
  const { result } = renderHook(() => useChatSession(), { wrapper });

  await act(async () => {
    await result.current.fetchSessionManually('session');
  });

  expect(chatService.getSession).toHaveBeenCalledWith(
    expect.objectContaining({
      params: { include_reference: true, include_prompt: true },
    }),
    true,
  );
});

it('requests references and prompts for agent history displayed in the UI', async () => {
  agentService.fetchAgentLogsById.mockResolvedValue({ data: { data: {} } });
  const { result } = renderHook(() => useAgentSession(), { wrapper });

  await act(async () => {
    await result.current.fetchSessionManually('session');
  });

  expect(agentService.fetchAgentLogsById).toHaveBeenCalledWith(
    'resource',
    'session',
    { include_reference: true, include_prompt: true },
  );
});
