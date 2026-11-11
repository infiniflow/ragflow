import { act, renderHook } from '@testing-library/react';
import { useSendSharedMessage } from './use-send-shared-message';
import { useThinkingLevel } from '@/hooks/use-thinking-level';
import storage from '@/utils/authorization-util';

const mockSend = jest.fn().mockResolvedValue(undefined);
const mockParams = new URLSearchParams(
  'shared_id=shared-chat&from=chat&reasoning=0&data_name=Alice',
);

jest.mock('react-router', () => ({ useSearchParams: () => [mockParams] }));
jest.mock('@/hooks/use-chat-request', () => ({
  useFetchExternalChatInfo: () => ({ data: { llm_id: 'model' } }),
}));
jest.mock('@/hooks/logic-hooks', () => ({
  useHandleMessageInputChange: () => ({ value: 'Hello', setValue: jest.fn() }),
  useSendMessageWithSse: () => ({ send: mockSend, answer: {}, done: true }),
  useSelectDerivedMessages: () => ({
    derivedMessages: [],
    addNewestQuestion: jest.fn(),
    removeLatestMessage: jest.fn(),
  }),
}));

beforeEach(() => {
  mockSend.mockClear();
  storage.setThinkingLevel('5');
});
afterEach(() => storage.setThinkingLevel('0'));

it('initializes thinking from the URL and sends the disabled level', async () => {
  const { result } = renderHook(() => ({
    chat: useSendSharedMessage(),
    level: useThinkingLevel(),
  }));
  expect(result.current.level).toBe('0');
  await act(async () => {
    result.current.chat.handlePressEnter({
      enableThinking: result.current.level,
    });
  });
  expect(mockSend).toHaveBeenLastCalledWith(
    '/api/v1/chatbots/shared-chat/completions',
    expect.objectContaining({ question: 'Hello', reasoning: 0 }),
  );
});

it('does not initialize the session again on an unrelated render', () => {
  const { rerender } = renderHook(() => useSendSharedMessage());
  expect(mockSend).toHaveBeenCalledTimes(1);
  expect(mockSend).toHaveBeenCalledWith(
    '/api/v1/chatbots/shared-chat/completions',
    { question: '', name: 'Alice' },
  );
  rerender();
  expect(mockSend).toHaveBeenCalledTimes(1);
});

it('allows the message input to override the embedded thinking preference', async () => {
  const { result } = renderHook(() => useSendSharedMessage());
  await act(async () => {
    result.current.handlePressEnter({ enableThinking: '5' });
  });
  expect(mockSend).toHaveBeenLastCalledWith(
    '/api/v1/chatbots/shared-chat/completions',
    expect.objectContaining({ reasoning: 5 }),
  );
});
