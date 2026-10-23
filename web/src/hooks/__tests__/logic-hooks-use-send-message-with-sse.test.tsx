import { act, renderHook } from '@testing-library/react';
import { ReadableStream, TextDecoderStream } from 'node:stream/web';
import { useSendMessageWithSse } from '../logic-hooks';

jest.mock('../route-hook', () => ({}));
jest.mock('../use-user-setting-request', () => ({}));

const TextDecoderStreamDescriptor = Object.getOwnPropertyDescriptor(
  globalThis,
  'TextDecoderStream',
);

function mockResponse(
  read: jest.Mock,
  json: () => Promise<object> = async () => ({ code: 0 }),
) {
  const body = {
    pipeThrough: jest.fn().mockReturnThis(),
    getReader: () => ({ read }),
  };
  const response = {
    body,
    clone: () => ({ json }),
  } as unknown as Response;
  jest.spyOn(globalThis, 'fetch').mockResolvedValue(response);
  return response;
}

describe('useSendMessageWithSse', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    Object.defineProperty(globalThis, 'TextDecoderStream', {
      configurable: true,
      value: TextDecoderStream,
    });
  });

  afterEach(() => {
    act(() => jest.runOnlyPendingTimers());
    jest.useRealTimers();
    jest.restoreAllMocks();
    if (TextDecoderStreamDescriptor) {
      Object.defineProperty(
        globalThis,
        'TextDecoderStream',
        TextDecoderStreamDescriptor,
      );
    } else {
      Reflect.deleteProperty(globalThis, 'TextDecoderStream');
    }
  });

  it.each([
    [{}, false],
    [{ chatBoxId: 'model-1' }, false],
    [{}, true],
    [{ chatBoxId: 'model-1' }, true],
  ] as const)(
    'finishes after a stream read fails for %j (partial answer: %s)',
    async (body, partialAnswer) => {
      const initialReadCount = partialAnswer ? 2 : 1;
      let failStream: (error: Error) => void;
      const reader = new ReadableStream({
        start(controller) {
          if (partialAnswer) {
            controller.enqueue({
              data: JSON.stringify({ data: { answer: 'Hello' } }),
            });
          }
          failStream = (error) => controller.error(error);
        },
      }).getReader();
      const read = jest.fn(() => {
        // Bound retries so a regression cannot hang the test runner.
        if (read.mock.calls.length > initialReadCount + 1) {
          return Promise.resolve({ done: true, value: undefined });
        }
        return reader.read();
      });
      let rejectJson: (error: Error) => void;
      const json = new Promise<never>((_, reject) => {
        rejectJson = reject;
      });
      mockResponse(read, () => json);
      const { result } = renderHook(() => useSendMessageWithSse());
      let pending: ReturnType<typeof result.current.send>;

      await act(async () => {
        pending = result.current.send('/completion', body);
      });
      expect(read).toHaveBeenCalledTimes(initialReadCount);
      expect(
        'chatBoxId' in body ? result.current.allDone : result.current.done,
      ).toBe(false);

      let response;
      await act(async () => {
        const error = new TypeError('connection reset');
        failStream(error);
        rejectJson(error);
        response = await pending;
      });

      expect(read).toHaveBeenCalledTimes(initialReadCount);
      expect(result.current.done).toBe(true);
      expect(result.current.allDone).toBe(true);
      expect(response).toBeUndefined();
      if (partialAnswer) {
        expect(result.current.answer.answer).toBe('Hello');
      }
    },
  );

  it('keeps another chat box loading when one stream fails', async () => {
    const error = new TypeError('connection reset');
    const failedRead = jest
      .fn()
      .mockRejectedValueOnce(error)
      .mockRejectedValueOnce(new DOMException('cancelled', 'AbortError'));
    const failedResponse = mockResponse(failedRead, async () => {
      throw error;
    });
    let finishStream: () => void;
    const reader = new ReadableStream({
      start(controller) {
        finishStream = () => controller.close();
      },
    }).getReader();
    const activeResponse = mockResponse(jest.fn(() => reader.read()));
    jest
      .mocked(fetch)
      .mockResolvedValueOnce(failedResponse)
      .mockResolvedValueOnce(activeResponse);
    const { result } = renderHook(() => useSendMessageWithSse());
    let activeRequest: ReturnType<typeof result.current.send>;

    await act(async () => {
      const failedRequest = result.current.send('/completion', {
        chatBoxId: 'model-1',
      });
      activeRequest = result.current.send('/completion', {
        chatBoxId: 'model-2',
      });
      await failedRequest;
    });

    expect(result.current.doneRecord).toEqual({
      'model-1': true,
      'model-2': false,
    });
    expect(result.current.allDone).toBe(false);

    await act(async () => {
      finishStream();
      await activeRequest;
    });

    expect(failedRead).toHaveBeenCalledTimes(1);
    expect(result.current.doneRecord).toEqual({});
    expect(result.current.allDone).toBe(true);
  });

  it('finishes after cancellation', async () => {
    const read = jest
      .fn()
      .mockRejectedValue(new DOMException('cancelled', 'AbortError'));
    mockResponse(read);
    const { result } = renderHook(() => useSendMessageWithSse());

    await act(async () => {
      await result.current.send('/completion', {});
    });

    expect(read).toHaveBeenCalledTimes(1);
    expect(result.current.done).toBe(true);
  });

  it('continues past malformed events and finishes a valid answer', async () => {
    const read = jest
      .fn()
      .mockResolvedValueOnce({ done: false, value: { data: '{' } })
      .mockResolvedValueOnce({
        done: false,
        value: { data: JSON.stringify({ data: { answer: 'Hello' } }) },
      })
      .mockResolvedValueOnce({ done: true });
    mockResponse(read);
    const { result } = renderHook(() => useSendMessageWithSse());

    await act(async () => {
      await result.current.send('/completion', { session_id: 'session-1' });
    });

    expect(read).toHaveBeenCalledTimes(3);
    expect(result.current.answer.answer).toBe('Hello');
    expect(result.current.answer.conversationId).toBe('session-1');
    expect(result.current.done).toBe(true);
  });
});
