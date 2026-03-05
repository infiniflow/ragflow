import { NextMessageInputOnPressEnterParameter } from '@/components/message-input/next';
import message from '@/components/ui/message';
import { MessageType, SharedFrom } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useCreateNextSharedConversation } from '@/hooks/use-chat-request';
import { Message } from '@/interfaces/database/chat';
import { get } from 'lodash';
import trim from 'lodash/trim';
import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router';
import { v4 as uuid } from 'uuid';

const isCompletionError = (res: any) =>
  res && (res?.response.status !== 200 || res?.data?.code !== 0);

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

const DATA_PREFIX = 'data_';

interface SharedChatSearchParams {
  from: SharedFrom;
  sharedId: string | null;
  release: string | null;
  locale: string | null;
  theme: string | null;
  data: Record<string, string>;
  visibleAvatar: boolean;
}

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();
  const data = Object.fromEntries(
    Array.from(searchParams.entries())
      .filter(([key]) => key.startsWith(DATA_PREFIX))
      .map(([key, value]) => [key.replace(DATA_PREFIX, ''), value]),
  );
  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
    release: searchParams.get('release'),
    locale: searchParams.get('locale'),
    theme: searchParams.get('theme'),
    data: data,
    visibleAvatar: searchParams.get('visible_avatar')
      ? searchParams.get('visible_avatar') !== '1'
      : true,
  } as SharedChatSearchParams;
};

export const useSendSharedMessage = () => {
  const {
    from,
    sharedId: conversationId,
    release,
    data: sharedData,
  } = useGetSharedChatSearchParams();
  const botType = from === SharedFrom.Agent ? 'agentbots' : 'chatbots';
  const releaseQuery = release ? `?release=${encodeURIComponent(release)}` : '';
  const completionUrl = `/api/v1/${botType}/${conversationId}/completions${releaseQuery}`;
  const { createSharedConversation: setConversation } =
    useCreateNextSharedConversation();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { send, answer, done, stopOutputMessage } =
    useSendMessageWithSse(completionUrl);
  const {
    derivedMessages,
    removeLatestMessage,
    addNewestAnswer,
    addNewestQuestion,
    scrollRef,
    messageContainerRef,
    removeAllMessages,
    removeAllMessagesExceptFirst,
  } = useSelectDerivedMessages();
  const [hasError, setHasError] = useState(false);

  const sendMessage = useCallback(
    async (
      message: Message,
      id?: string,
      enableThinking?: boolean,
      enableInternet?: boolean,
    ) => {
      const res = await send({
        conversation_id: id ?? conversationId,
        quote: true,
        question: message.content,
        session_id: get(derivedMessages, '0.session_id'),
        reasoning: enableThinking,
        internet: enableInternet,
        ...(release ? { release } : {}),
      });

      if (isCompletionError(res)) {
        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      }
    },
    [
      send,
      conversationId,
      derivedMessages,
      setValue,
      removeLatestMessage,
      release,
    ],
  );

  const handleSendMessage = useCallback(
    async (
      message: Message,
      enableThinking?: boolean,
      enableInternet?: boolean,
    ) => {
      if (conversationId !== '') {
        sendMessage(message, undefined, enableThinking, enableInternet);
      } else {
        const data = await setConversation('user id');
        if (data.code === 0) {
          const id = data.data.id;
          sendMessage(message, id, enableThinking, enableInternet);
        }
      }
    },
    [conversationId, setConversation, sendMessage],
  );

  const fetchSessionId = useCallback(async () => {
    const payload = { question: '' };
    const ret = await send({
      ...payload,
      ...sharedData,
      ...(release ? { release } : {}),
    });
    if (isCompletionError(ret)) {
      message.error(ret?.data.message);
      setHasError(true);
    }
  }, [sharedData, release, send]);

  useEffect(() => {
    fetchSessionId();
  }, [fetchSessionId]);

  useEffect(() => {
    if (answer.answer) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer]);

  const handlePressEnter = useCallback(
    ({
      enableThinking,
      enableInternet,
    }: NextMessageInputOnPressEnterParameter) => {
      if (trim(value) === '') return;
      const id = uuid();
      if (done) {
        setValue('');
        addNewestQuestion({
          content: value,
          doc_ids: [],
          id,
          role: MessageType.User,
        });
        handleSendMessage(
          {
            content: value.trim(),
            id,
            role: MessageType.User,
          },
          enableThinking,
          enableInternet,
        );
      }
    },
    [addNewestQuestion, done, handleSendMessage, setValue, value],
  );

  return {
    handlePressEnter,
    handleInputChange,
    value,
    sendLoading: !done,
    loading: false,
    derivedMessages,
    hasError,
    stopOutputMessage,
    scrollRef,
    messageContainerRef,
    removeAllMessages,
    removeAllMessagesExceptFirst,
  };
};
