import { MessageType, SharedFrom } from '@/constants/chat';
import { useCreateNextSharedConversation } from '@/hooks/chat-hooks';
import {
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { Message } from '@/interfaces/database/chat';
import { get } from 'lodash';
import trim from 'lodash/trim';
import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { useHandleMessageInputChange } from './hooks';

export const useSelectNextSharedMessages = () => {
  // const { data, loading } = useFetchNextSharedConversation(conversationId);

  const {
    derivedMessages,
    ref,
    setDerivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
  } = useSelectDerivedMessages();

  // useEffect(() => {
  //   setDerivedMessages(data?.data?.message);
  // }, [setDerivedMessages, data]);

  return {
    derivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
    // loading,
    ref,
    setDerivedMessages,
  };
};

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();

  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
  };
};

export const useSendSharedMessage = () => {
  const { from, sharedId: conversationId } = useGetSharedChatSearchParams();
  const { createSharedConversation: setConversation } =
    useCreateNextSharedConversation();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { send, answer, done } = useSendMessageWithSse(
    `/api/v1/${from === SharedFrom.Agent ? 'agentbots' : 'chatbots'}/${conversationId}/completions`,
  );
  const {
    derivedMessages,
    ref,
    removeLatestMessage,
    addNewestAnswer,
    addNewestQuestion,
  } = useSelectNextSharedMessages();

  const sendMessage = useCallback(
    async (message: Message, id?: string) => {
      const res = await send({
        conversation_id: id ?? conversationId,
        quote: true,
        question: message.content,
        session_id: get(derivedMessages, '0.session_id'),
      });

      if (res && (res?.response.status !== 200 || res?.data?.code !== 0)) {
        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      }
    },
    [send, conversationId, derivedMessages, setValue, removeLatestMessage],
  );

  const handleSendMessage = useCallback(
    async (message: Message) => {
      if (conversationId !== '') {
        sendMessage(message);
      } else {
        const data = await setConversation('user id');
        if (data.code === 0) {
          const id = data.data.id;
          sendMessage(message, id);
        }
      }
    },
    [conversationId, setConversation, sendMessage],
  );

  const fetchSessionId = useCallback(async () => {
    const ret = await send({ question: '' });
    console.log('🚀 ~ fetchSessionId ~ ret:', ret);
  }, [send]);

  useEffect(() => {
    fetchSessionId();
  }, [fetchSessionId, send]);

  useEffect(() => {
    if (answer.answer) {
      addNewestAnswer(answer);
    }
  }, [answer, addNewestAnswer]);

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
    sendLoading: !done,
    ref,
    loading: false,
    derivedMessages,
  };
};
