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

const mockSend = jest.fn().mockResolvedValue(undefined);
const mockSetDerivedMessages = jest.fn();

jest.mock('@/hooks/logic-hooks', () => ({
  useHandleMessageInputChange: () => ({
    handleInputChange: jest.fn(),
    setValue: jest.fn(),
    value: '',
  }),
  useSelectDerivedMessages: () => ({
    addNewestAnswer: jest.fn(),
    addNewestQuestion: jest.fn(),
    derivedMessages: [],
    messageContainerRef: { current: null },
    removeAllMessages: jest.fn(),
    removeAllMessagesExceptFirst: jest.fn(),
    removeLatestMessage: jest.fn(),
    setDerivedMessages: mockSetDerivedMessages,
    virtualizer: {},
  }),
  useSendMessageWithSse: () => ({
    answer: {},
    done: true,
    send: mockSend,
    stopOutputMessage: jest.fn(),
  }),
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
    mockSend.mockClear();
    mockSetDerivedMessages.mockClear();
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
      { question: '', region: 'eu', tier: 'pro' },
    );
  });
});
