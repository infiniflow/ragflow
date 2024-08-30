import { MessageType, SharedFrom } from '@/constants/chat';
import {
  useCreateNextSharedConversation,
  useFetchNextSharedConversation,
} from '@/hooks/chat-hooks';
import {
  useSelectDerivedMessages,
  useSendMessageWithSse,
} from '@/hooks/logic-hooks';
import { Message } from '@/interfaces/database/chat';
import api from '@/utils/api';
import trim from 'lodash/trim';
import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { useHandleMessageInputChange } from './hooks';

export const useCreateSharedConversationOnMount = () => {
  const [currentQueryParameters] = useSearchParams();
  const [conversationId, setConversationId] = useState('');

  const { createSharedConversation: createConversation } =
    useCreateNextSharedConversation();
  const sharedId = currentQueryParameters.get('shared_id');
  const userId = currentQueryParameters.get('user_id');

  const setConversation = useCallback(async () => {
    if (sharedId) {
      const data = await createConversation(userId ?? undefined);
      const id = data.data?.id;
      if (id) {
        setConversationId(id);
      }
    }
  }, [createConversation, sharedId, userId]);

  useEffect(() => {
    setConversation();
  }, [setConversation]);

  return { conversationId };
};

export const useSelectNextSharedMessages = (conversationId: string) => {
  const { data, loading } = useFetchNextSharedConversation(conversationId);

  const {
    derivedMessages,
    ref,
    setDerivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
  } = useSelectDerivedMessages();

  useEffect(() => {
    setDerivedMessages(data?.data?.message);
  }, [setDerivedMessages, data]);

  return {
    derivedMessages,
    addNewestAnswer,
    addNewestQuestion,
    removeLatestMessage,
    loading,
    ref,
    setDerivedMessages,
  };
};

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

export const useSendSharedMessage = (conversationId: string) => {
  const { createSharedConversation: setConversation } =
    useCreateNextSharedConversation();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();
  const { send, answer, done } = useSendMessageWithSse(
    api.completeExternalConversation,
  );
  const {
    derivedMessages,
    ref,
    removeLatestMessage,
    addNewestAnswer,
    addNewestQuestion,
    loading,
  } = useSelectNextSharedMessages(conversationId);

  const sendMessage = useCallback(
    async (message: Message, id?: string) => {
      const res = await send({
        conversation_id: id ?? conversationId,
        quote: false,
        messages: [...(derivedMessages ?? []), message],
      });

      if (res && (res?.response.status !== 200 || res?.data?.retcode !== 0)) {
        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      }
    },
    [conversationId, derivedMessages, removeLatestMessage, setValue, send],
  );

  const handleSendMessage = useCallback(
    async (message: Message) => {
      if (conversationId !== '') {
        sendMessage(message);
      } else {
        const data = await setConversation('user id');
        if (data.retcode === 0) {
          const id = data.data.id;
          sendMessage(message, id);
        }
      }
    },
    [conversationId, setConversation, sendMessage],
  );

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
    loading,
    derivedMessages,
  };
};

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();

  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
  };
};
