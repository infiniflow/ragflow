import { MessageType } from '@/constants/chat';
import { act, renderHook } from '@testing-library/react';
import { useChatStreamStore } from '../chat-stream/store';
import { useSendMessage } from './use-send-chat-message';

const mockScrollTo = jest.fn();
const mockRunChatCompletionStream = jest.fn();
const mockScrollElement = document.createElement('div');
mockScrollElement.scrollTo = mockScrollTo;

function setMockScrollHeight(value: number) {
  Object.defineProperty(mockScrollElement, 'scrollHeight', {
    configurable: true,
    value,
  });
}

jest.mock('@/hooks/logic-hooks', () => ({
  useHandleMessageInputChange: () => ({
    handleInputChange: jest.fn(),
    value: '',
    setValue: jest.fn(),
  }),
}));
jest.mock('@/hooks/use-chat-request', () => ({
  useFetchChat: () => ({ data: {} }),
  useGetChatSearchParams: () => ({
    conversationId: 'conversation-id',
    isNew: 'false',
  }),
}));
jest.mock('@/hooks/use-virtual-messages', () => ({
  useVirtualMessages: (
    _messages: unknown[],
    containerRef: { current: HTMLDivElement | null },
  ) => {
    containerRef.current = mockScrollElement;
    return {
      virtualizer: {},
      scrollToBottom: jest.fn(),
    };
  },
}));
jest.mock('@/utils/notification', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('react-router', () => ({
  useParams: () => ({ id: 'chat-id' }),
}));
jest.mock('../chat-stream/run-stream', () => ({
  runChatCompletionStream: (...args: unknown[]) =>
    mockRunChatCompletionStream(...args),
}));
jest.mock('../utils', () => ({
  resolveResendOptions: () => ({}),
}));
jest.mock('./use-chat-url', () => ({
  useCreateConversationBeforeSendMessage: () => ({
    createConversationBeforeSendMessage: jest.fn(),
  }),
}));
jest.mock('./use-select-conversation-list', () => ({
  useFindPrologueFromDialogList: () => '',
}));
jest.mock('./use-upload-file', () => ({
  useUploadFile: () => ({
    handleUploadFile: jest.fn(),
    isUploading: false,
    removeFile: jest.fn(),
    files: [],
    clearFiles: jest.fn(),
  }),
}));

function mockControlledCompletion() {
  let finish = () => {};
  mockRunChatCompletionStream.mockImplementation(
    ({ conversationId }: { conversationId: string }) => {
      useChatStreamStore
        .getState()
        .beginStream(conversationId, new AbortController());
      return new Promise<{ ok: boolean; aborted: boolean }>((resolve) => {
        finish = () => {
          useChatStreamStore.getState().endStream(conversationId);
          resolve({ ok: true, aborted: false });
        };
      });
    },
  );

  return { finish: () => finish() };
}

describe('useSendMessage regenerateMessage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setMockScrollHeight(1000);
    mockRunChatCompletionStream.mockResolvedValue({ ok: true, aborted: false });
    useChatStreamStore.setState({
      sessions: {
        'conversation-id': {
          chatId: 'chat-id',
          messages: [
            {
              id: 'question-id',
              role: MessageType.User,
              content: 'Original question',
              conversationId: 'conversation-id',
            },
            {
              id: 'question-id',
              role: MessageType.Assistant,
              content: 'Original answer',
              conversationId: 'conversation-id',
            },
          ],
          isStreaming: false,
          lastActiveAt: 0,
        },
      },
    });
  });

  afterEach(() => {
    act(() => {
      useChatStreamStore.setState({ sessions: {} });
    });
    jest.restoreAllMocks();
  });

  it('follows the real scroll height throughout the regenerated stream', async () => {
    const stream = mockControlledCompletion();
    const { result } = renderHook(() => useSendMessage());

    await act(async () => {
      result.current.regenerateMessage(result.current.messages[0]);
      await Promise.resolve();
    });
    expect(mockScrollTo).toHaveBeenNthCalledWith(1, { top: 1000 });

    await act(async () => {
      setMockScrollHeight(1600);
      useChatStreamStore.getState().applyAnswer('conversation-id', {
        id: 'regenerated-id',
        answer: 'Regenerated answer',
      });
      stream.finish();
      await Promise.resolve();
    });

    expect(result.current.messages).toHaveLength(4);
    expect(mockScrollTo).toHaveBeenCalledTimes(2);
    expect(mockScrollTo).toHaveBeenNthCalledWith(2, { top: 1600 });
  });

  it('stops following when the user scrolls during regeneration', async () => {
    const stream = mockControlledCompletion();
    const { result } = renderHook(() => useSendMessage());

    await act(async () => {
      result.current.regenerateMessage(result.current.messages[0]);
      await Promise.resolve();
    });
    expect(mockScrollTo).toHaveBeenCalledTimes(1);

    act(() => {
      mockScrollElement.dispatchEvent(new WheelEvent('wheel'));
      setMockScrollHeight(1600);
      useChatStreamStore.getState().applyAnswer('conversation-id', {
        id: 'regenerated-id',
        answer: 'Regenerated answer',
      });
    });

    expect(mockScrollTo).toHaveBeenCalledTimes(1);

    await act(async () => {
      stream.finish();
      await Promise.resolve();
    });
  });
});
