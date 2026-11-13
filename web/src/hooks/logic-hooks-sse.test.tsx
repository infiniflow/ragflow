import { act, renderHook } from '@testing-library/react';
import { ReadableStream, TextDecoderStream } from 'node:stream/web';
import { useSendMessageWithSse } from './logic-hooks';

const mockFetch = jest.fn();

beforeAll(() => {
  Object.assign(globalThis, {
    fetch: (...args: unknown[]) => mockFetch(...args),
    ReadableStream,
    TextDecoderStream,
  });
});

beforeEach(() => {
  mockFetch.mockReset();
});

/** A response whose body streams `text` and whose JSON view is `json` (or not JSON). */
function fakeResponse({
  status = 200,
  text,
  json,
}: {
  status?: number;
  text: string;
  json?: unknown;
}) {
  return {
    status,
    statusText: 'status text',
    clone: () => ({
      json: async () => {
        if (json === undefined) throw new SyntaxError('not JSON');
        return json;
      },
    }),
    body: new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(text));
        controller.close();
      },
    }),
  };
}

const sendOnce = async (result: {
  current: ReturnType<typeof useSendMessageWithSse>;
}) => {
  await act(async () => {
    await result.current.send('/ask', { question: 'hi' });
  });
};

it('reports the message of an error envelope and clears it on the next request', async () => {
  const envelope = { code: 102, message: 'Model provider unavailable' };
  mockFetch.mockResolvedValueOnce(
    fakeResponse({ text: JSON.stringify(envelope), json: envelope }),
  );
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBe('Model provider unavailable');

  mockFetch.mockResolvedValueOnce(
    fakeResponse({ text: 'data: {"data":{"answer":"hi"}}\n\n' }),
  );
  await sendOnce(result);
  expect(result.current.error).toBeUndefined();
});

it('falls back to the status text when the envelope has no message', async () => {
  const envelope = { code: 500 };
  mockFetch.mockResolvedValueOnce(
    fakeResponse({
      status: 500,
      text: JSON.stringify(envelope),
      json: envelope,
    }),
  );
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBe('status text');
});

it('does not report an error for a healthy stream', async () => {
  mockFetch.mockResolvedValueOnce(
    fakeResponse({ text: 'data: {"data":{"answer":"hi"}}\n\n' }),
  );
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBeUndefined();
  expect(result.current.done).toBe(true);
});

it('reports a request that never got a response', async () => {
  mockFetch.mockRejectedValueOnce(new TypeError('Failed to fetch'));
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBe('Failed to fetch');
  expect(result.current.done).toBe(true);
});

it('does not report a request the user stopped', async () => {
  mockFetch.mockRejectedValueOnce(new DOMException('aborted', 'AbortError'));
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBeUndefined();
});

it('reports a failure streamed as an error frame instead of showing it as the answer', async () => {
  const frame = {
    code: 500,
    message: 'API request failed with status 503 (gave up after 4 attempts)',
    data: {
      answer:
        '**ERROR**: API request failed with status 503 (gave up after 4 attempts)',
    },
  };
  mockFetch.mockResolvedValueOnce(
    fakeResponse({ text: `data: ${JSON.stringify(frame)}\n\n` }),
  );
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBe(frame.message);
  expect(result.current.answer.answer).toBeUndefined();
});

it('reports an error answer that arrives with a zero code', async () => {
  const frame = {
    code: 0,
    data: { answer: '**ERROR**: provider unavailable' },
  };
  mockFetch.mockResolvedValueOnce(
    fakeResponse({ text: `data: ${JSON.stringify(frame)}\n\n` }),
  );
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBe('provider unavailable');
});

it.each([
  ['an HTML page', 'Bad Gateway'],
  ['an empty body', ''],
])('reports a non-OK status whose body is %s', async (_name, text) => {
  mockFetch.mockResolvedValueOnce(fakeResponse({ status: 502, text }));
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBe('status text');
  expect(result.current.done).toBe(true);
});

it('falls back to the HTTP status when a non-OK response has no status text', async () => {
  mockFetch.mockResolvedValueOnce({
    ...fakeResponse({ status: 503, text: '' }),
    statusText: '',
  });
  const { result } = renderHook(() => useSendMessageWithSse());

  await sendOnce(result);
  expect(result.current.error).toBe('HTTP 503');
});
