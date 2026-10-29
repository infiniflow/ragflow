import { act, renderHook } from '@testing-library/react';
import message from '@/components/ui/message';
import { MessageEventType } from '@/hooks/use-send-message';
import { MessageType } from '@/constants/chat';
import {
  findMessageFromList,
  useSendAgentMessage,
} from './use-send-agent-message';

let mockAnswerList: any[] = [];
let mockDone = true;
const mockSend = jest.fn();
const mockAddNewestOneAnswer = jest.fn();
const mockSetValue = jest.fn();
const mockRemoveLatestMessage = jest.fn();
const mockReceiveMessageError = jest.fn(
  (res: any) =>
    res &&
    (res.response.status !== 200 ||
      (typeof res.data?.code === 'number' && res.data.code !== 0)),
);

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn(), warning: jest.fn() },
}));

jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));

jest.mock('../constant', () => ({ BeginId: 'begin' }));

jest.mock('../store', () => ({
  __esModule: true,
  default: (select: (state: unknown) => unknown) =>
    select({ getNode: () => undefined }),
}));

jest.mock('../form/begin-form/use-watch-change', () => ({
  transferInputsArrayToObject: () => ({}),
}));

jest.mock('../utils', () => ({
  receiveMessageError: (res: unknown) => mockReceiveMessageError(res),
}));

jest.mock('@/hooks/use-send-message', () => ({
  // Mirrors the real MessageEventType enum; the real module cannot be
  // requireActual'd here because it pulls in eventsource-parser's
  // TransformStream, which jsdom does not provide.
  MessageEventType: {
    WorkflowStarted: 'workflow_started',
    NodeStarted: 'node_started',
    NodeFinished: 'node_finished',
    Message: 'message',
    MessageEnd: 'message_end',
    WorkflowFinished: 'workflow_finished',
    UserInputs: 'user_inputs',
    WaitingForUser: 'waiting_for_user',
    NodeLogs: 'node_logs',
  },
  useSendMessageBySSE: () => ({
    send: mockSend,
    answerList: mockAnswerList,
    done: mockDone,
    setDone: jest.fn(),
    resetAnswerList: jest.fn(),
    stopOutputMessage: jest.fn(),
  }),
}));

jest.mock('@/hooks/logic-hooks', () => ({
  // Fully mocked: the real module imports eventsource-parser, which needs
  // TransformStream (unavailable in jsdom).
  useHandleMessageInputChange: () => ({
    handleInputChange: jest.fn(),
    value: '',
    setValue: mockSetValue,
  }),
  useSelectDerivedMessages: () => ({
    derivedMessages: [],
    setDerivedMessages: jest.fn(),
    scrollRef: { current: null },
    messageContainerRef: { current: null },
    removeLatestMessage: mockRemoveLatestMessage,
    removeMessageById: jest.fn(),
    addNewestOneQuestion: jest.fn(),
    addNewestOneAnswer: mockAddNewestOneAnswer,
    removeAllMessages: jest.fn(),
    removeAllMessagesExceptFirst: jest.fn(),
    scrollToBottom: jest.fn(),
    addPrologue: jest.fn(),
  }),
}));

jest.mock('react-router', () => ({
  useParams: () => ({}),
  useSearchParams: () => [new URLSearchParams(), jest.fn()],
}));

jest.mock('../hooks/use-get-begin-query', () => ({
  useIsTaskMode: () => false,
  useSelectBeginNodeDataInputs: () => [],
}));

const messageFrame = (sessionId: string, content: string) => ({
  event: MessageEventType.Message,
  data: { content, session_id: sessionId },
  message_id: 'message-1',
  session_id: sessionId,
});

const renderSendAgentMessage = (activeSessionId: string | undefined) =>
  renderHook(
    (props: { activeSessionId: string | undefined }) =>
      useSendAgentMessage({ activeSessionId: props.activeSessionId }),
    { initialProps: { activeSessionId } },
  );

const SuccessResponse = {
  response: { ok: true, status: 200 },
  data: { code: 0 },
};

const complete = (result: unknown, isCurrent = () => true) => ({
  result,
  isCurrent,
});

const deferred = () => {
  let resolve!: (value: ReturnType<typeof complete>) => void;
  const promise = new Promise<ReturnType<typeof complete>>((finish) => {
    resolve = finish;
  });
  return { promise, resolve };
};

describe('useSendAgentMessage session-scoped stream gating', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAnswerList = [];
    mockDone = true;
  });

  it('does not apply frames whose session_id differs from activeSessionId', () => {
    const { rerender } = renderSendAgentMessage('session-b');
    mockAnswerList = [messageFrame('session-a', 'hello')];
    rerender({ activeSessionId: 'session-b' });
    expect(mockAddNewestOneAnswer).not.toHaveBeenCalled();
  });

  it('applies frames whose session_id matches activeSessionId', () => {
    const { rerender } = renderSendAgentMessage('session-a');
    mockAnswerList = [messageFrame('session-a', 'hello')];
    rerender({ activeSessionId: 'session-a' });
    expect(mockAddNewestOneAnswer).toHaveBeenCalledTimes(1);
    expect(mockAddNewestOneAnswer).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'message-1', answer: 'hello' }),
    );
  });

  it('keeps applying frames that carry no session id', () => {
    const { rerender } = renderSendAgentMessage('');
    mockAnswerList = [
      {
        event: MessageEventType.Message,
        data: { content: 'legacy frame' },
        message_id: 'message-2',
      },
    ];
    rerender({ activeSessionId: '' });
    expect(mockAddNewestOneAnswer).toHaveBeenCalledTimes(1);
    expect(mockAddNewestOneAnswer).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'message-2', answer: 'legacy frame' }),
    );
  });

  it('replays the streamed answer when switching back to the stream session', () => {
    const { rerender } = renderSendAgentMessage('session-b');
    mockAnswerList = [messageFrame('session-a', 'hello')];
    rerender({ activeSessionId: 'session-b' });
    expect(mockAddNewestOneAnswer).not.toHaveBeenCalled();

    // A -> B -> A: the accumulated answer is re-applied without a new frame.
    rerender({ activeSessionId: 'session-a' });
    expect(mockAddNewestOneAnswer).toHaveBeenCalledTimes(1);
  });

  it('re-applies the streamed answer after external hydration replaced the list', async () => {
    const { result, rerender } = renderSendAgentMessage('session-a');
    mockAnswerList = [messageFrame('session-a', 'hello')];
    rerender({ activeSessionId: 'session-a' });
    expect(mockAddNewestOneAnswer).toHaveBeenCalledTimes(1);

    // SessionChat hydrates persisted messages and asks for a re-apply.
    await act(async () => {
      result.current.reapplyStreamedAnswer();
    });
    expect(mockAddNewestOneAnswer).toHaveBeenCalledTimes(2);
  });

  it('still applies frames when no activeSessionId is provided', () => {
    const { rerender } = renderSendAgentMessage(undefined);
    mockAnswerList = [messageFrame('session-a', 'hello')];
    rerender({ activeSessionId: undefined });
    expect(mockAddNewestOneAnswer).toHaveBeenCalledTimes(1);
  });

  it('records the session a request is sent to before the first frame', async () => {
    mockSend.mockResolvedValue(complete(SuccessResponse));
    const { result } = renderSendAgentMessage('session-new');
    await act(async () => {
      await result.current.sendMessage({
        message: { id: 'q1', content: 'hi', role: MessageType.User },
        exploreSessionId: 'session-new',
      });
    });
    expect(result.current.requestedSessionId).toBe('session-new');
  });
});

describe('useSendAgentMessage completion ownership', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSend.mockReset();
    mockAnswerList = [];
    mockDone = true;
  });

  it.each([
    ['success', SuccessResponse],
    [
      'HTTP error',
      { response: { status: 503 }, data: { code: 503, message: 'Old error' } },
    ],
    ['network failure', undefined],
  ])(
    'ignores a superseded %s without clearing uploads or touching the current conversation',
    async (_name, oldResult) => {
      const first = deferred();
      const second = deferred();
      const refetch = jest.fn();
      mockSend
        .mockReturnValueOnce(first.promise)
        .mockReturnValueOnce(second.promise)
        .mockResolvedValue(complete(SuccessResponse));
      const { result } = renderHook(() => useSendAgentMessage({ refetch }));
      const question = {
        id: 'question',
        content: 'hello',
        role: MessageType.User,
      };
      let firstRequest!: Promise<void>;
      act(() => {
        firstRequest = result.current.sendMessage({ message: question });
      });

      const upload = {
        created_at: 0,
        created_by: 'user',
        extension: 'pdf',
        id: 'new-upload',
        mime_type: 'application/pdf',
        name: 'new.pdf',
        preview_url: null,
        size: 1,
      };
      act(() => {
        result.current.appendUploadResponseList(upload, [
          new File([], 'new.pdf'),
        ]);
      });
      let secondRequest!: Promise<void>;
      act(() => {
        secondRequest = result.current.sendMessage({ message: question });
      });
      await act(async () => {
        first.resolve(complete(oldResult, () => false));
        await firstRequest;
      });
      expect(refetch).not.toHaveBeenCalled();
      expect(mockReceiveMessageError).not.toHaveBeenCalled();
      expect(message.error).not.toHaveBeenCalled();
      expect(mockSetValue).not.toHaveBeenCalled();
      expect(mockRemoveLatestMessage).not.toHaveBeenCalled();

      // Read the remaining upload state through the next request payload.
      await act(async () => {
        await result.current.sendMessage({ message: question });
      });
      expect(mockSend).toHaveBeenLastCalledWith(
        expect.objectContaining({ files: [upload] }),
      );
      await act(async () => {
        second.resolve(complete(SuccessResponse, () => false));
        await secondRequest;
      });
    },
  );

  it('checks ownership after a resolved send when another send path starts before continuation', async () => {
    let generation = 0;
    const refetch = jest.fn();
    mockSend.mockImplementation(() => {
      const owner = ++generation;
      return Promise.resolve(
        complete(SuccessResponse, () => generation === owner),
      );
    });
    const { result } = renderHook(() => useSendAgentMessage({ refetch }));
    await act(async () => {
      const first = result.current.sendMessage({
        message: { id: 'first', content: 'first', role: MessageType.User },
      });
      const second = result.current.sendFormMessage({ inputs: {} });
      await Promise.all([first, second]);
    });
    expect(mockReceiveMessageError).not.toHaveBeenCalled();
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it('does not refetch for a superseded form submission', async () => {
    const refetch = jest.fn();
    mockSend.mockResolvedValue(complete(SuccessResponse, () => false));
    const { result } = renderHook(() => useSendAgentMessage({ refetch }));
    await act(async () => {
      await result.current.sendFormMessage({ inputs: {} });
    });
    expect(refetch).not.toHaveBeenCalled();
  });

  it('keeps current HTTP failure handling', async () => {
    const refetch = jest.fn();
    mockSend.mockResolvedValue(
      complete({
        response: { status: 503 },
        data: { code: 503, message: 'Current error' },
      }),
    );
    const { result } = renderHook(() => useSendAgentMessage({ refetch }));
    await act(async () => {
      await result.current.sendMessage({
        message: { id: 'current', content: 'retry me', role: MessageType.User },
      });
    });
    expect(message.error).toHaveBeenCalledWith('Current error');
    expect(mockSetValue).toHaveBeenCalledWith('retry me');
    expect(mockRemoveLatestMessage).toHaveBeenCalledTimes(1);
    expect(refetch).not.toHaveBeenCalled();
  });

  it('keeps current network failure and manual-stop completion behavior', async () => {
    const refetch = jest.fn();
    mockSend.mockResolvedValue(complete(undefined));
    const { result } = renderHook(() => useSendAgentMessage({ refetch }));
    await act(async () => {
      await result.current.sendMessage({
        message: { id: 'current', content: 'hello', role: MessageType.User },
      });
    });
    expect(mockReceiveMessageError).toHaveBeenCalledWith(undefined);
    expect(mockSetValue).not.toHaveBeenCalled();
    expect(mockRemoveLatestMessage).not.toHaveBeenCalled();
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

describe('findMessageFromList memory errors', () => {
  it.each([
    [
      MessageEventType.NodeFinished,
      { outputs: { memory_error: 'failed' } },
      'failed',
    ],
    [
      MessageEventType.Message,
      { content: 'hello', memory_error: 'failed' },
      'failed',
    ],
    [MessageEventType.Message, { content: 'hello' }, undefined],
  ])('extracts memory_error from %s with data %j', (event, data, expected) => {
    mockAnswerList = [{ ...messageFrame('session-a', 'hello'), event, data }];
    expect(findMessageFromList(mockAnswerList).memory_error).toBe(expected);
  });
});
