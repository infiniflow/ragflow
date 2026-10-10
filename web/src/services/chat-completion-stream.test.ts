import { ReadableStream, TextDecoderStream } from 'node:stream/web';
import { parseCompletionEventStream } from './chat-completion-stream';
import { runChatCompletionStream } from '@/pages/next-chats/chat-stream/run-stream';
import { useChatStreamStore } from '@/pages/next-chats/chat-stream/store';
import { MessageType } from '@/constants/chat';
jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => 'fixture-only',
}));
jest.mock('@/utils/api', () => ({
  __esModule: true,
  default: { completionUrl: '/fixture' },
}));
Object.assign(globalThis, { TextDecoderStream });
function responseFor(body: unknown): Response {
  return { body } as unknown as Response;
}
async function collect(response: Response) {
  const chunks = [];
  for await (const chunk of parseCompletionEventStream(response))
    chunks.push(chunk);
  return chunks;
}
describe('completion transport failures', () => {
  test('propagates a stream read failure', async () => {
    const failure = new Error('fixture connection reset');
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.error(failure);
      },
    });
    await expect(collect(responseFor(body))).rejects.toBe(failure);
  });
  test('preserves user cancellation errors', async () => {
    const failure = new DOMException('fixture cancellation', 'AbortError');
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.error(failure);
      },
    });
    await expect(collect(responseFor(body))).rejects.toBe(failure);
  });
  test('yields successful answers and completes normally', async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode('data: {"data":{"answer":"hello"}}\n\n'),
        );
        controller.close();
      },
    });
    await expect(collect(responseFor(body))).resolves.toEqual([
      { answer: 'hello' },
    ]);
  });
});

describe('completion driver failure handling', () => {
  const originalFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = originalFetch;
    useChatStreamStore.setState({ sessions: {} });
  });
  test.each([
    {
      kind: 'transport failure',
      failure: new Error('fixture connection reset'),
      expected: { ok: false, aborted: false },
    },
    {
      kind: 'user cancellation',
      failure: new DOMException('fixture cancellation', 'AbortError'),
      expected: { ok: true, aborted: true },
    },
  ])(
    'handles $kind without leaving the session streaming',
    async ({ failure, expected }) => {
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          controller.error(failure);
        },
      });
      globalThis.fetch = jest.fn().mockResolvedValue({
        body,
        status: 200,
        clone: () => ({
          json: async () => {
            throw new SyntaxError('fixture SSE is not JSON');
          },
        }),
      });
      const store = useChatStreamStore.getState();
      store.ensureSession('fixture-session', 'fixture-chat');
      store.appendQuestion('fixture-session', {
        id: 'fixture-question',
        role: MessageType.User,
        content: 'fixture question',
      });
      await expect(
        runChatCompletionStream({
          conversationId: 'fixture-session',
          chatId: 'fixture-chat',
          question: 'fixture question',
        }),
      ).resolves.toEqual(expected);
      expect(
        useChatStreamStore.getState().sessions['fixture-session'].isStreaming,
      ).toBe(false);
      expect(
        useChatStreamStore.getState().sessions['fixture-session']
          .abortController,
      ).toBeUndefined();
      if (!expected.ok) {
        store.failStream('fixture-session', 'fixture question');
        expect(
          useChatStreamStore.getState().sessions['fixture-session']
            .pendingInput,
        ).toBe('fixture question');
        expect(
          useChatStreamStore.getState().sessions['fixture-session'].messages,
        ).toEqual([]);
      }
    },
  );
});
