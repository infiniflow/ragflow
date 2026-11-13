import { act, renderHook, waitFor } from '@testing-library/react';
import { useChatStreamStore } from '../../chat-stream/store';
import { useSendMessage } from '../use-send-chat-message';

const mockRun = jest.fn();
const mockNotifyError = jest.fn();

jest.mock('../../chat-stream/run-stream', () => ({
  runChatCompletionStream: (...args: unknown[]) => mockRun(...args),
}));
jest.mock('@/utils/notification', () => ({
  __esModule: true,
  default: { error: (...args: unknown[]) => mockNotifyError(...args) },
}));
jest.mock('react-router', () => ({ useParams: () => ({ id: 'chat-1' }) }));
jest.mock('@/hooks/use-chat-request', () => ({
  useFetchChat: () => ({ data: { llm_setting: {} } }),
  useGetChatSearchParams: () => ({ conversationId: 'c1', isNew: 'false' }),
}));
jest.mock('@/hooks/use-virtual-messages', () => ({
  useVirtualMessages: () => ({
    virtualizer: {},
    scrollToBottom: jest.fn(),
  }),
}));
jest.mock('../use-select-conversation-list', () => ({
  useFindPrologueFromDialogList: () => '',
}));
jest.mock('../use-upload-file', () => ({
  useUploadFile: () => ({
    handleUploadFile: jest.fn(),
    isUploading: false,
    removeFile: jest.fn(),
    files: [],
    clearFiles: jest.fn(),
  }),
}));
jest.mock('../use-chat-url', () => ({
  useCreateConversationBeforeSendMessage: () => ({
    createConversationBeforeSendMessage: async () => ({
      targetConversationId: 'c1',
      currentMessages: [],
    }),
  }),
}));

const asked = () =>
  mockRun.mock.calls.map(
    ([params]) => (params as { question: string }).question,
  );

beforeEach(() => {
  mockRun.mockReset();
  mockNotifyError.mockReset();
  useChatStreamStore.setState({ sessions: {} });
  useChatStreamStore.getState().ensureSession('c1', 'chat-1');
});

it('offers Retry on the failure toast and re-sends the same question', async () => {
  mockRun
    .mockResolvedValueOnce({
      ok: false,
      aborted: false,
      errorMessage: 'Model provider unavailable',
    })
    .mockResolvedValueOnce({ ok: true, aborted: false });
  const { result } = renderHook(() => useSendMessage());

  act(() => result.current.setValue('what is RAGFlow?'));
  await act(async () => {
    await result.current.handlePressEnter({});
  });

  await waitFor(() => expect(mockNotifyError).toHaveBeenCalledTimes(1));
  const toast = mockNotifyError.mock.calls[0][0];
  expect(toast.description).toBe('Model provider unavailable');
  expect(toast.action.label).toBe('Retry');
  // The failure hands the question back to the input box.
  await waitFor(() => expect(result.current.value).toBe('what is RAGFlow?'));

  await act(async () => {
    toast.action.onClick();
  });

  await waitFor(() => expect(mockRun).toHaveBeenCalledTimes(2));
  expect(asked()).toEqual(['what is RAGFlow?', 'what is RAGFlow?']);
  // The retried question is no longer parked in the input box.
  await waitFor(() => expect(result.current.value).toBe(''));
  expect(mockNotifyError).toHaveBeenCalledTimes(1);
});

it('keeps what the user typed meanwhile when retrying', async () => {
  mockRun
    .mockResolvedValueOnce({ ok: false, aborted: false })
    .mockResolvedValueOnce({ ok: true, aborted: false });
  const { result } = renderHook(() => useSendMessage());

  act(() => result.current.setValue('first question'));
  await act(async () => {
    await result.current.handlePressEnter({});
  });
  await waitFor(() => expect(mockNotifyError).toHaveBeenCalled());
  await waitFor(() => expect(result.current.value).toBe('first question'));

  act(() => result.current.setValue('something else'));
  await act(async () => {
    mockNotifyError.mock.calls[0][0].action.onClick();
  });

  await waitFor(() => expect(mockRun).toHaveBeenCalledTimes(2));
  expect(asked()[1]).toBe('first question');
  expect(result.current.value).toBe('something else');
});

it('does not offer Retry for a stopped answer', async () => {
  mockRun.mockResolvedValueOnce({ ok: false, aborted: true });
  const { result } = renderHook(() => useSendMessage());

  act(() => result.current.setValue('hello'));
  await act(async () => {
    await result.current.handlePressEnter({});
  });

  expect(mockRun).toHaveBeenCalledTimes(1);
  expect(mockNotifyError).not.toHaveBeenCalled();
});
