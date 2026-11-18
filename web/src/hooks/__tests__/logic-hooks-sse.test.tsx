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

import { act, renderHook, waitFor } from '@testing-library/react';
import { ReadableStream, TextDecoderStream } from 'node:stream/web';
import { TextEncoder } from 'node:util';
import { useSendMessageWithSse } from '../logic-hooks';

const createSseResponse = (payload: Record<string, unknown>) => {
  const body = new ReadableStream({
    start(controller) {
      controller.enqueue(
        new TextEncoder().encode(`data:${JSON.stringify(payload)}\n\n`),
      );
      controller.close();
    },
  });
  return {
    body,
    clone: () => ({ json: jest.fn() }),
    headers: { get: () => 'text/event-stream' },
    status: 200,
  } as unknown as Response;
};

describe('useSendMessageWithSse', () => {
  const originalTextDecoderStream = global.TextDecoderStream;

  beforeAll(() => {
    global.TextDecoderStream =
      TextDecoderStream as typeof global.TextDecoderStream;
  });

  afterAll(() => {
    global.TextDecoderStream = originalTextDecoderStream;
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('retains envelope session metadata and returns SSE success', async () => {
    jest.spyOn(global, 'fetch').mockResolvedValue(
      createSseResponse({
        code: 0,
        data: { answer: '' },
        message: 'success',
        session_id: 'agent-session',
      }),
    );
    const { result } = renderHook(() => useSendMessageWithSse());

    let response: Awaited<ReturnType<typeof result.current.send>>;
    await act(async () => {
      response = await result.current.send('/completion', {
        chatBoxId: 'initialization-1',
      });
    });

    expect(response!.data.code).toBe(0);
    await waitFor(() =>
      expect(result.current.answer).toEqual(
        expect.objectContaining({
          chatBoxId: 'initialization-1',
          session_id: 'agent-session',
        }),
      ),
    );
  });

  it('returns an error carried by an SSE frame', async () => {
    jest
      .spyOn(global, 'fetch')
      .mockResolvedValue(
        createSseResponse({ code: 500, data: false, message: 'failed' }),
      );
    const { result } = renderHook(() => useSendMessageWithSse());

    let response: Awaited<ReturnType<typeof result.current.send>>;
    await act(async () => {
      response = await result.current.send('/completion', {
        chatBoxId: 'initialization-1',
      });
    });

    expect(response!.data).toEqual(
      expect.objectContaining({ code: 500, message: 'failed' }),
    );
  });
});
