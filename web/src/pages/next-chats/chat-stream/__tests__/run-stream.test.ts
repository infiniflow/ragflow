import { runChatCompletionStream } from '../run-stream';
import { useChatStreamStore } from '../store';

const mockRequest = jest.fn();
let mockChunks: { answer: string }[] = [];

jest.mock('@/services/chat-completion-stream', () => ({
  ...jest.requireActual('@/services/chat-completion-stream'),
  requestChatCompletionStream: (...args: unknown[]) => mockRequest(...args),
  parseCompletionEventStream: async function* () {
    yield* mockChunks;
  },
}));

const Params = { conversationId: 'c1', chatId: 'chat-1', question: 'hi' };

// jsdom has no Response; the driver only needs a status and a cloneable JSON body.
function fakeResponse(status: number, body?: unknown) {
  const clone = () => ({
    json: async () => {
      if (body === undefined) throw new SyntaxError('not JSON');
      return body;
    },
  });
  return { status, clone };
}

beforeEach(() => {
  mockRequest.mockReset();
  mockChunks = [];
  useChatStreamStore.getState().ensureSession('c1', 'chat-1');
});

it('reports the server message when the endpoint answers with an error envelope', async () => {
  mockRequest.mockResolvedValue(
    fakeResponse(200, { code: 102, message: 'Model provider unavailable' }),
  );

  await expect(runChatCompletionStream(Params)).resolves.toEqual({
    ok: false,
    aborted: false,
    errorMessage: 'Model provider unavailable',
  });
});

it('fails without a message when the request itself fails', async () => {
  mockRequest.mockRejectedValue(new Error('network down'));

  await expect(runChatCompletionStream(Params)).resolves.toEqual({
    ok: false,
    aborted: false,
    errorMessage: undefined,
  });
});

it('succeeds on a stream body', async () => {
  mockRequest.mockResolvedValue(fakeResponse(200));

  await expect(runChatCompletionStream(Params)).resolves.toEqual({
    ok: true,
    aborted: false,
    errorMessage: undefined,
  });
});

it('treats an answer that is only the error marker as a failed request', async () => {
  mockRequest.mockResolvedValue(fakeResponse(200));
  mockChunks = [
    {
      answer:
        '**ERROR**: API request failed with status 503 (gave up after 4 attempts)',
    },
  ];

  await expect(runChatCompletionStream(Params)).resolves.toEqual({
    ok: false,
    aborted: false,
    errorMessage:
      'API request failed with status 503 (gave up after 4 attempts)',
  });
});

it('keeps an error that follows real answer text as part of the answer', async () => {
  mockRequest.mockResolvedValue(fakeResponse(200));
  mockChunks = [
    { answer: 'Partial answer' },
    { answer: 'Partial answer\n**ERROR**: cut off' },
  ];

  const result = await runChatCompletionStream(Params);
  expect(result.ok).toBe(true);
});
