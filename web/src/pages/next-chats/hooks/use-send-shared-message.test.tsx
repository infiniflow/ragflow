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
import { MemoryRouter, useNavigate } from 'react-router';
import {
  useGetSharedChatSearchParams,
  useSendSharedMessage,
} from './use-send-shared-message';

const successfulResponse = {
  data: { code: 0 },
  response: { status: 200 },
};
const mockSend = jest.fn().mockResolvedValue(successfulResponse);
const mockSetDerivedMessages = jest.fn();
const mockAddNewestAnswer = jest.fn();
const mockAddNewestQuestion = jest.fn();
const mockStopOutputMessage = jest.fn();
let mockAnswer: Record<string, unknown> = {};
let mockValue = '';

jest.mock('@/hooks/logic-hooks', () => ({
  useHandleMessageInputChange: () => ({
    handleInputChange: jest.fn(),
    setValue: jest.fn(),
    value: mockValue,
  }),
  useSelectDerivedMessages: () => ({
    addNewestAnswer: mockAddNewestAnswer,
    addNewestQuestion: mockAddNewestQuestion,
    derivedMessages: [],
    messageContainerRef: { current: null },
    removeAllMessages: jest.fn(),
    removeAllMessagesExceptFirst: jest.fn(),
    removeLatestMessage: jest.fn(),
    setDerivedMessages: mockSetDerivedMessages,
    virtualizer: {},
  }),
  useSendMessageWithSse: () => ({
    answer: mockAnswer,
    done: true,
    send: mockSend,
    stopOutputMessage: mockStopOutputMessage,
  }),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock('@/hooks/use-chat-request', () => ({
  useFetchExternalChatInfo: () => ({ data: undefined }),
}));

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <MemoryRouter
    initialEntries={[
      '/?from=chat&shared_id=conversation-1&data_region=us&data_tier=free',
    ]}
  >
    {children}
  </MemoryRouter>
);

describe('useGetSharedChatSearchParams', () => {
  it('keeps parsed data stable until the search parameters change', async () => {
    const { result, rerender } = renderHook(
      () => ({
        navigate: useNavigate(),
        params: useGetSharedChatSearchParams(),
      }),
      { wrapper },
    );

    const initialData = result.current.params.data;
    expect(initialData).toEqual({ region: 'us', tier: 'free' });

    rerender();
    expect(result.current.params.data).toBe(initialData);

    act(() => {
      result.current.navigate(
        '/?from=chat&shared_id=conversation-1&theme=dark&data_tier=free&data_region=us',
      );
    });

    await waitFor(() => expect(result.current.params.theme).toBe('dark'));
    expect(result.current.params.data).toBe(initialData);

    act(() => {
      result.current.navigate(
        '/?from=chat&shared_id=conversation-1&data_region=eu&data_tier=pro',
      );
    });

    await waitFor(() =>
      expect(result.current.params.data).toEqual({ region: 'eu', tier: 'pro' }),
    );
    expect(result.current.params.data).not.toBe(initialData);
  });
});

describe('useSendSharedMessage', () => {
  beforeEach(() => {
    mockAnswer = {};
    mockValue = '';
    mockSend.mockReset().mockResolvedValue(successfulResponse);
    mockSetDerivedMessages.mockClear();
    mockAddNewestAnswer.mockClear();
    mockAddNewestQuestion.mockClear();
    mockStopOutputMessage.mockClear();
  });

  it('clears the previous session before initializing changed data', async () => {
    const { result } = renderHook(
      () => ({
        navigate: useNavigate(),
        sharedMessage: useSendSharedMessage(),
      }),
      { wrapper },
    );

    await waitFor(() => expect(mockSend).toHaveBeenCalledTimes(1));
    mockSend.mockClear();
    mockSetDerivedMessages.mockClear();

    act(() => {
      result.current.navigate(
        '/?from=chat&shared_id=conversation-1&data_region=eu&data_tier=pro',
      );
    });

    await waitFor(() => expect(mockSend).toHaveBeenCalledTimes(1));
    expect(mockSetDerivedMessages).toHaveBeenCalledWith([]);
    expect(mockSetDerivedMessages.mock.invocationCallOrder[0]).toBeLessThan(
      mockSend.mock.invocationCallOrder[0],
    );
    expect(mockSend).toHaveBeenCalledWith(
      '/api/v1/chatbots/conversation-1/completions',
      expect.objectContaining({
        chatBoxId: expect.any(String),
        question: '',
        region: 'eu',
        tier: 'pro',
      }),
      expect.any(AbortController),
    );
  });

  it('retains the initialized session when the prologue is empty', async () => {
    const { result, rerender } = renderHook(() => useSendSharedMessage(), {
      wrapper,
    });

    await waitFor(() => expect(mockSend).toHaveBeenCalledTimes(1));
    const initializationBody = mockSend.mock.calls[0][1];

    mockAnswer = {
      answer: '',
      chatBoxId: initializationBody.chatBoxId,
      session_id: 'session-1',
    };
    rerender();

    await waitFor(() => expect(result.current.sendLoading).toBe(false));
    mockValue = 'hello';
    rerender();

    act(() => {
      result.current.handlePressEnter({});
    });

    await waitFor(() => expect(mockSend).toHaveBeenCalledTimes(2));
    expect(mockSend.mock.calls[1][1]).toEqual(
      expect.objectContaining({ question: 'hello', session_id: 'session-1' }),
    );
  });

  it('keeps sending disabled after failure and allows initialization retry', async () => {
    mockSend.mockResolvedValueOnce(undefined);
    const { result } = renderHook(() => useSendSharedMessage(), { wrapper });

    await waitFor(() => expect(result.current.hasError).toBe(true));
    expect(result.current.sendLoading).toBe(false);

    act(() => {
      result.current.retryInitialization();
    });

    await waitFor(() => expect(mockSend).toHaveBeenCalledTimes(2));
    expect(result.current.hasError).toBe(false);
    expect(result.current.sendLoading).toBe(true);
  });

  it('ignores a superseded initialization response', async () => {
    const { result, rerender } = renderHook(
      () => ({
        navigate: useNavigate(),
        sharedMessage: useSendSharedMessage(),
      }),
      { wrapper },
    );

    await waitFor(() => expect(mockSend).toHaveBeenCalledTimes(1));
    const firstBody = mockSend.mock.calls[0][1];
    const firstController = mockSend.mock.calls[0][2] as AbortController;

    act(() => {
      result.current.navigate(
        '/?from=chat&shared_id=conversation-1&data_region=eu&data_tier=pro',
      );
    });

    await waitFor(() => expect(mockSend).toHaveBeenCalledTimes(2));
    expect(firstController.signal.aborted).toBe(true);
    const secondBody = mockSend.mock.calls[1][1];

    mockAnswer = {
      answer: 'old',
      chatBoxId: firstBody.chatBoxId,
      session_id: 'old-session',
    };
    rerender();
    expect(result.current.sharedMessage.sendLoading).toBe(true);
    expect(mockAddNewestAnswer).not.toHaveBeenCalled();

    mockAnswer = {
      answer: '',
      chatBoxId: secondBody.chatBoxId,
      session_id: 'new-session',
    };
    rerender();

    await waitFor(() =>
      expect(result.current.sharedMessage.sendLoading).toBe(false),
    );
  });
});
