import { act, renderHook } from '@testing-library/react';
import { MessageEventType } from '@/hooks/use-send-message';
import { MessageType } from '@/constants/chat';
import { useSendAgentMessage } from './use-send-agent-message';

let mockAnswerList: any[] = [];
let mockDone = true;
const mockSend = jest.fn();
const mockAddNewestOneAnswer = jest.fn();

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
    setValue: jest.fn(),
  }),
  useSelectDerivedMessages: () => ({
    derivedMessages: [],
    setDerivedMessages: jest.fn(),
    scrollRef: { current: null },
    messageContainerRef: { current: null },
    removeLatestMessage: jest.fn(),
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
    mockSend.mockResolvedValue({
      response: { ok: true, status: 200 },
      data: { code: 0 },
    });
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
