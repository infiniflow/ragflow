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

import { MessageType } from '@/constants/chat';
import { IAnswer, IMessage } from '@/interfaces/database/chat';
import { buildMessageUuidWithRole } from '@/utils/chat';
import { act, renderHook } from '@testing-library/react';
import { useSelectDerivedMessages } from '../logic-hooks';

// Chatbot/embed SSE frames carry `id: null` (WriteChatbotFrame hardcodes it
// for python wire parity). IAnswer types id as string, so cast through null.
const embedAnswer = (text: string): IAnswer =>
  ({ answer: text, id: null }) as unknown as IAnswer;

describe('useSelectDerivedMessages addNewestAnswer', () => {
  it('keeps the placeholder id when the stream frame id is null (embed chat)', () => {
    const { result } = renderHook(() => useSelectDerivedMessages());

    act(() => {
      result.current.addNewestQuestion({
        id: 'q1',
        role: MessageType.User,
        content: '你好',
      } as IMessage);
    });

    // Two chunks of the same answer: the id must stay stable so the
    // virtualizer item key does not churn per chunk.
    act(() => {
      result.current.addNewestAnswer(embedAnswer('chunk1'));
    });
    const firstChunkId = result.current.derivedMessages.at(-1)?.id;
    act(() => {
      result.current.addNewestAnswer(embedAnswer('chunk1chunk2'));
    });

    expect(firstChunkId).toBe('q1');
    expect(result.current.derivedMessages.at(-1)?.id).toBe('q1');
    expect(result.current.derivedMessages).toHaveLength(2);
  });

  it('never leaves assistant messages on the shared assistant_null key', () => {
    const { result } = renderHook(() => useSelectDerivedMessages());

    // Embed opening handshake: the prologue arrives as an answer with id null.
    act(() => {
      result.current.addNewestAnswer(embedAnswer('Hi!'));
    });
    act(() => {
      result.current.addNewestQuestion({
        id: 'q1',
        role: MessageType.User,
        content: '贾雨村',
      } as IMessage);
    });
    act(() => {
      result.current.addNewestAnswer(embedAnswer('《红楼梦》第一回…'));
    });

    const ids = result.current.derivedMessages.map((x) => x.id);
    expect(ids.every((id) => typeof id === 'string' && id.length > 0)).toBe(
      true,
    );
    const keys = result.current.derivedMessages.map((x) =>
      buildMessageUuidWithRole(x),
    );
    expect(new Set(keys).size).toBe(keys.length);
  });

  it('prefers the backend message id when the frame carries one (in-app chat)', () => {
    const { result } = renderHook(() => useSelectDerivedMessages());

    act(() => {
      result.current.addNewestQuestion({
        id: 'q1',
        role: MessageType.User,
        content: 'hi',
      } as IMessage);
    });
    act(() => {
      result.current.addNewestAnswer({ answer: 'hello', id: 'msg-1' });
    });

    expect(result.current.derivedMessages.at(-1)?.id).toBe('msg-1');
  });
});
