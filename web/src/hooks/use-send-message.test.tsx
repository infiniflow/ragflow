/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { act, renderHook } from '@testing-library/react';
import { ReadableStream, TextDecoderStream } from 'node:stream/web';
import { MessageEventType, useSendMessageBySSE } from './use-send-message';

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock('@/utils/authorization-util', () => ({
  getAuthorization: () => 'test-token',
}));

const frame = (messageId: string, content: string) => ({
  event: MessageEventType.Message,
  message_id: messageId,
  session_id: 'session-1',
  data: { content },
});

const createStream = () => {
  let controller!: {
    enqueue(chunk: Uint8Array): void;
    close(): void;
  };
  const body = new ReadableStream<Uint8Array>({
    start(streamController) {
      controller = streamController;
    },
  });

  return {
    // Keep abort separate from the stream so tests can deliver a queued old
    // frame after cancellation and exercise request ownership explicitly.
    response: {
      ok: true,
      status: 200,
      body,
      clone: () => ({
        json: () => Promise.reject(new SyntaxError('SSE is not JSON')),
      }),
    } as unknown as Response,
    emit(value: unknown) {
      controller.enqueue(
        new TextEncoder().encode(`data: ${JSON.stringify(value)}\n\n`),
      );
    },
    close() {
      controller.close();
    },
  };
};

const flushStream = async (update: () => void) => {
  await act(async () => {
    update();
    await jest.advanceTimersByTimeAsync(0);
  });
};

describe('useSendMessageBySSE request isolation', () => {
  const originalFetch = globalThis.fetch;
  const originalTextDecoderStream = globalThis.TextDecoderStream;
  const mockFetch = jest.fn();

  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    mockFetch.mockReset();
    globalThis.fetch = mockFetch;
    Object.assign(globalThis, { TextDecoderStream });
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
    globalThis.fetch = originalFetch;
    globalThis.TextDecoderStream = originalTextDecoderStream;
  });

  it('keeps consecutive same-session turns separate across the old cleanup deadline', async () => {
    const first = createStream();
    const second = createStream();
    mockFetch
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));

    let firstRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      firstRequest = result.current.send({ query: 'first' });
      first.emit(frame('message-1', 'First answer.'));
      first.close();
    });
    await act(async () => {
      await firstRequest;
    });
    expect(result.current.done).toBe(true);
    expect(result.current.answerList).toEqual([
      frame('message-1', 'First answer.'),
    ]);

    let secondRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      secondRequest = result.current.send({ query: 'second' });
      second.emit(frame('message-2', 'Second prefix.'));
    });
    expect(result.current.done).toBe(false);
    expect(result.current.answerList).toEqual([
      frame('message-2', 'Second prefix.'),
    ]);

    await act(async () => {
      await jest.advanceTimersByTimeAsync(1000);
    });
    expect(result.current.answerList).toEqual([
      frame('message-2', 'Second prefix.'),
    ]);

    const lastFrame = {
      event: MessageEventType.MessageEnd,
      message_id: 'message-2',
      session_id: 'session-1',
      data: { reference: { chunks: [] }, downloads: [{ id: 'download-1' }] },
    };
    await flushStream(() => {
      second.emit(frame('message-2', 'Second suffix.'));
      second.emit(lastFrame);
      second.close();
    });
    let response: Awaited<ReturnType<typeof result.current.send>>;
    await act(async () => {
      response = await secondRequest;
    });
    expect(result.current.answerList).toEqual([
      frame('message-2', 'Second prefix.'),
      frame('message-2', 'Second suffix.'),
      lastFrame,
    ]);
    expect(response!.result).toEqual({
      response: second.response,
      data: lastFrame,
    });
    expect(response!.isCurrent()).toBe(true);
    expect(result.current.done).toBe(true);

    await act(async () => {
      await jest.advanceTimersByTimeAsync(999);
    });
    expect(result.current.answerList.at(-1)).toEqual(lastFrame);
    await act(async () => {
      await jest.advanceTimersByTimeAsync(1);
    });
    expect(result.current.answerList).toEqual([]);
  });

  it('ignores superseded frames and finalizers while the next request is active', async () => {
    const first = createStream();
    const second = createStream();
    mockFetch
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));

    let firstRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      firstRequest = result.current.send({ query: 'first' });
      first.emit(frame('message-1', 'Old prefix.'));
    });
    let secondRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      secondRequest = result.current.send({ query: 'second' });
      second.emit(frame('message-2', 'New prefix.'));
    });
    expect(mockFetch.mock.calls[0][1].signal.aborted).toBe(true);

    await flushStream(() => {
      first.emit(frame('message-1', 'Stale suffix.'));
      first.close();
    });
    await act(async () => {
      const completion = await firstRequest;
      expect(completion.isCurrent()).toBe(false);
      await jest.advanceTimersByTimeAsync(1000);
    });
    expect(result.current.done).toBe(false);
    expect(result.current.answerList).toEqual([
      frame('message-2', 'New prefix.'),
    ]);

    await flushStream(() => second.close());
    await act(async () => {
      await secondRequest;
    });
  });

  it('rechecks ownership when an already-resolved promise resumes after another send', async () => {
    const first = createStream();
    const second = createStream();
    mockFetch
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));
    let firstRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      firstRequest = result.current.send({ query: 'first' });
      first.close();
    });
    const firstCompletion = await firstRequest;
    expect(firstCompletion.isCurrent()).toBe(true);

    let continuation!: Promise<void>;
    let secondRequest!: ReturnType<typeof result.current.send>;
    let ownedAtContinuation: boolean | undefined;
    await flushStream(() => {
      // The first promise has settled, but its caller has not resumed yet.
      continuation = firstRequest.then((completion) => {
        ownedAtContinuation = completion.isCurrent();
      });
      secondRequest = result.current.send({ query: 'second' });
      second.emit(frame('message-2', 'New answer.'));
    });
    await continuation;
    expect(ownedAtContinuation).toBe(false);
    expect(result.current.done).toBe(false);

    await flushStream(() => second.close());
    await act(async () => {
      expect((await secondRequest).isCurrent()).toBe(true);
    });
  });

  it('keeps manual cancellation of the current request owned by its caller', async () => {
    const stream = createStream();
    mockFetch.mockResolvedValueOnce(stream.response);
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));
    let request!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      request = result.current.send({ query: 'first' });
      stream.emit(frame('message-1', 'Partial answer.'));
    });
    await flushStream(() => {
      result.current.stopOutputMessage();
      stream.close();
    });
    const completion = await request;
    expect(completion.isCurrent()).toBe(true);
    expect(completion.result?.response).toBe(stream.response);
    expect(result.current.done).toBe(true);
  });

  it('keeps a current network failure distinguishable from a superseded completion', async () => {
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    mockFetch.mockRejectedValueOnce(new Error('Connection failed'));
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));
    let completion!: Awaited<ReturnType<typeof result.current.send>>;
    await act(async () => {
      completion = await result.current.send({ query: 'first' });
    });
    expect(completion.result).toBeUndefined();
    expect(completion.isCurrent()).toBe(true);
    expect(result.current.done).toBe(true);
  });

  it('preserves the current HTTP error response inside its owned completion', async () => {
    const response = {
      ok: false,
      status: 503,
      statusText: 'Service unavailable',
      clone: () => ({ json: async () => ({ message: 'Current HTTP error' }) }),
    };
    mockFetch.mockResolvedValueOnce(response);
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));
    let completion!: Awaited<ReturnType<typeof result.current.send>>;
    await act(async () => {
      completion = await result.current.send({ query: 'first' });
    });
    expect(completion.result).toEqual({
      response,
      data: {
        code: 503,
        data: null,
        message: 'Current HTTP error',
        status: 503,
      },
    });
    expect(completion.isCurrent()).toBe(true);
    expect(result.current.done).toBe(true);
  });

  it('marks a superseded fetch failure as stale without finishing the new request', async () => {
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    let rejectFirst!: (error: Error) => void;
    const firstFetch = new Promise((_, reject) => {
      rejectFirst = reject;
    });
    const second = createStream();
    mockFetch
      .mockReturnValueOnce(firstFetch)
      .mockResolvedValueOnce(second.response);
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));
    let firstRequest!: ReturnType<typeof result.current.send>;
    let secondRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      firstRequest = result.current.send({ query: 'first' });
      secondRequest = result.current.send({ query: 'second' });
      rejectFirst(new DOMException('Aborted', 'AbortError'));
    });
    const completion = await firstRequest;
    expect(completion.result).toBeUndefined();
    expect(completion.isCurrent()).toBe(false);
    expect(result.current.done).toBe(false);
    await flushStream(() => second.close());
    await act(async () => {
      await secondRequest;
    });
  });

  it('cancels stop/reset cleanup when another request starts immediately', async () => {
    const first = createStream();
    const second = createStream();
    mockFetch
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const { result } = renderHook(() => useSendMessageBySSE('/agent'));

    let firstRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      firstRequest = result.current.send({ query: 'first' });
      first.emit(frame('message-1', 'Stopped answer.'));
    });
    act(() => {
      result.current.stopOutputMessage();
      result.current.resetAnswerList();
    });

    let secondRequest!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      secondRequest = result.current.send({ query: 'second' });
      second.emit(frame('message-2', 'New answer.'));
      first.close();
    });
    await act(async () => {
      await firstRequest;
      await jest.advanceTimersByTimeAsync(1000);
    });
    expect(result.current.done).toBe(false);
    expect(result.current.answerList).toEqual([
      frame('message-2', 'New answer.'),
    ]);

    await flushStream(() => second.close());
    await act(async () => {
      await secondRequest;
    });
  });

  it('clears pending cleanup on unmount', async () => {
    const stream = createStream();
    mockFetch.mockResolvedValueOnce(stream.response);
    const { result, unmount } = renderHook(() => useSendMessageBySSE('/agent'));
    await act(async () => {
      const request = result.current.send({ query: 'first' });
      stream.close();
      await request;
    });
    expect(jest.getTimerCount()).toBe(1);
    unmount();
    expect(jest.getTimerCount()).toBe(0);
  });

  it('aborts an in-flight request on unmount without scheduling more cleanup', async () => {
    const stream = createStream();
    mockFetch.mockResolvedValueOnce(stream.response);
    const { result, unmount } = renderHook(() => useSendMessageBySSE('/agent'));
    let request!: ReturnType<typeof result.current.send>;
    await flushStream(() => {
      request = result.current.send({ query: 'first' });
    });
    unmount();
    expect(mockFetch.mock.calls[0][1].signal.aborted).toBe(true);
    stream.close();
    expect((await request).isCurrent()).toBe(false);
    expect(jest.getTimerCount()).toBe(0);
  });
});
