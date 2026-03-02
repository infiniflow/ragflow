import { MessageType, SharedFrom } from '@/constants/chat';
import {
  useHandleMessageInputChange,
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { useCreateNextSharedConversation } from '@/hooks/use-chat-request';
import { Message } from '@/interfaces/database/chat';
import { message } from 'antd';
import { get } from 'lodash';
import trim from 'lodash/trim';
import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'umi';
import { v4 as uuid } from 'uuid';

const isCompletionError = (res: any) =>
  res && (res?.response.status !== 200 || res?.data?.code !== 0);

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();
  const data_prefix = 'data_';
  const data = Object.fromEntries(
    searchParams
      .entries()
      .filter(([key]) => key.startsWith(data_prefix))
      .map(([key, value]) => [key.replace(data_prefix, ''), value]),
  );
  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
    conversationId: searchParams.get('conversation_id'), // Extract conversation_id from URL
    locale: searchParams.get('locale'),
    theme: searchParams.get('theme'),
    data: data,
    visibleAvatar: searchParams.get('visible_avatar')
      ? searchParams.get('visible_avatar') !== '1'
      : true,
  };
};

export const useSendSharedMessage = () => {
  const {
    from,
    sharedId,
    conversationId: urlConversationId, // Get conversation_id from URL
  } = useGetSharedChatSearchParams();
  const [searchParams] = useSearchParams();

  // Extract data parameters once to avoid recreating object
  const dataParams = Object.fromEntries(
    Array.from(searchParams.entries())
      .filter(([key]) => key.startsWith('data_'))
      .map(([key, value]) => [key.replace('data_', ''), value]),
  );

  const { createSharedConversation: setConversation } =
    useCreateNextSharedConversation();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { send, answer, done, stopOutputMessage } = useSendMessageWithSse(
    `/api/v1/${from === SharedFrom.Agent ? 'agentbots' : 'chatbots'}/${sharedId}/completions`,
  );
  const {
    derivedMessages,
    removeLatestMessage,
    addNewestAnswer,
    addNewestQuestion,
    scrollRef,
    messageContainerRef,
    removeAllMessages,
    removeAllMessagesExceptFirst,
    setDerivedMessages,
  } = useSelectDerivedMessages();
  const [hasError, setHasError] = useState(false);
  const [conversationId, setConversationId] = useState<string>(
    urlConversationId || '',
  );
  const [isLoadingHistory, setIsLoadingHistory] = useState(false);

  // Load conversation history if conversation_id is provided
  const loadConversationHistory = useCallback(
    async (convId: string) => {
      if (!convId) return;

      setIsLoadingHistory(true);
      try {
        const response = await fetch(
          `/v1/dialog/public/conversation/${convId}/messages`,
        );
        const result = await response.json();

        if (result.code === 0 && result.data.messages) {
          // Set the messages directly
          setDerivedMessages(result.data.messages);
          setConversationId(convId);
        } else {
          console.error('[loadConversationHistory] 加载失败:', result.message);
          message.error('加载历史消息失败');
        }
      } catch (error) {
        console.error('[loadConversationHistory] 错误:', error);
        message.error('加载历史消息失败');
      } finally {
        setIsLoadingHistory(false);
      }
    },
    [setDerivedMessages],
  );

  const sendMessage = useCallback(
    async (msg: Message, id?: string) => {
      const res = await send({
        conversation_id: id ?? conversationId,
        quote: true,
        question: msg.content,
        session_id: conversationId || get(derivedMessages, '0.session_id'),
      });

      if (isCompletionError(res)) {
        setValue(msg.content);
        removeLatestMessage();
      }
    },
    [send, conversationId, derivedMessages, setValue, removeLatestMessage],
  );

  const handleSendMessage = useCallback(
    async (msg: Message) => {
      if (sharedId !== '') {
        sendMessage(msg);
      } else {
        const data = await setConversation('user id');
        if (data.code === 0) {
          const id = data.data.id;
          sendMessage(msg, id);
        }
      }
    },
    [sharedId, setConversation, sendMessage],
  );

  // Initialize session or load history
  useEffect(() => {
    const initializeSession = async () => {
      // If we have a conversation_id from URL, load its history
      if (urlConversationId) {
        await loadConversationHistory(urlConversationId);
        return;
      }

      // Otherwise, initialize a new session
      const payload = { question: '', ...dataParams };
      const ret = await send(payload);
      if (isCompletionError(ret)) {
        message.error(ret?.data.message);
        setHasError(true);
      }
    };

    initializeSession();
  }, [urlConversationId]); // Only depend on urlConversationId

  useEffect(() => {
    if (answer.answer) {
      addNewestAnswer(answer);
    }

    // Extract id from answer and set as conversationId if we don't have one
    // The backend returns session_id in the data, but it's accessed as answer.id
    const sessionId = (answer as any).session_id || answer.id;

    if (sessionId && !conversationId && !urlConversationId) {
      setConversationId(sessionId);
    }
  }, [answer, addNewestAnswer, conversationId, urlConversationId]);

  const handlePressEnter = useCallback(
    (documentIds: string[]) => {
      if (trim(value) === '') return;
      const id = uuid();
      if (done) {
        setValue('');
        addNewestQuestion({
          content: value,
          doc_ids: documentIds,
          id,
          role: MessageType.User,
        });
        handleSendMessage({
          content: value.trim(),
          id,
          role: MessageType.User,
        });
      }
    },
    [addNewestQuestion, done, handleSendMessage, setValue, value],
  );

  return {
    handlePressEnter,
    handleInputChange,
    value,
    sendLoading: !done || isLoadingHistory,
    loading: isLoadingHistory,
    derivedMessages,
    hasError,
    stopOutputMessage,
    scrollRef,
    messageContainerRef,
    removeAllMessages,
    removeAllMessagesExceptFirst,
  };
};
