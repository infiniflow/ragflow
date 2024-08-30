import { MessageType, SharedFrom } from '@/constants/chat';
import {
  useCreateNextSharedConversation,
  useFetchNextSharedConversation,
} from '@/hooks/chat-hooks';
import { useSendMessageWithSse } from '@/hooks/logic-hooks';
import { IAnswer, Message } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { buildMessageUuid } from '@/utils/chat';
import trim from 'lodash/trim';
import {
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { useSearchParams } from 'umi';
import { v4 as uuid } from 'uuid';
import { useHandleMessageInputChange, useScrollToBottom } from './hooks';
import { IClientConversation, IMessage } from './interface';

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

export const useSelectCurrentSharedConversation = (conversationId: string) => {
  const [currentConversation, setCurrentConversation] =
    useState<IClientConversation>({} as IClientConversation);
  const { fetchConversation, loading } = useFetchNextSharedConversation();

  const ref = useScrollToBottom(currentConversation);

  const addNewestConversation = useCallback((message: Partial<Message>) => {
    setCurrentConversation((pre) => {
      return {
        ...pre,
        message: [
          ...(pre.message ?? []),
          {
            ...message,
            id: buildMessageUuid(message),
          } as IMessage,
          {
            role: MessageType.Assistant,
            content: '',
            id: buildMessageUuid({ ...message, role: MessageType.Assistant }),
            reference: {},
          } as IMessage,
        ],
      };
    });
  }, []);

  const addNewestAnswer = useCallback((answer: IAnswer) => {
    setCurrentConversation((pre) => {
      const latestMessage = pre.message?.at(-1);

      if (latestMessage) {
        return {
          ...pre,
          message: [
            ...pre.message.slice(0, -1),
            {
              ...latestMessage,
              content: answer.answer,
              reference: answer.reference,
              id: buildMessageUuid({
                id: answer.id,
                role: MessageType.Assistant,
              }),
              prompt: answer.prompt,
            } as IMessage,
          ],
        };
      }
      return pre;
    });
  }, []);

  const removeLatestMessage = useCallback(() => {
    setCurrentConversation((pre) => {
      const nextMessages = pre.message.slice(0, -2);
      return {
        ...pre,
        message: nextMessages,
      };
    });
  }, []);

  const fetchConversationOnMount = useCallback(async () => {
    if (conversationId) {
      const data = await fetchConversation(conversationId);
      if (data.retcode === 0) {
        setCurrentConversation(data.data);
      }
    }
  }, [conversationId, fetchConversation]);

  useEffect(() => {
    fetchConversationOnMount();
  }, [fetchConversationOnMount]);

  return {
    currentConversation,
    addNewestConversation,
    removeLatestMessage,
    loading,
    ref,
    setCurrentConversation,
    addNewestAnswer,
  };
};

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

export const useSendSharedMessage = (
  conversation: IClientConversation,
  addNewestConversation: (message: Partial<Message>, answer?: string) => void,
  removeLatestMessage: () => void,
  setCurrentConversation: Dispatch<SetStateAction<IClientConversation>>,
  addNewestAnswer: (answer: IAnswer) => void,
) => {
  const conversationId = conversation.id;
  const { createSharedConversation: setConversation } =
    useCreateNextSharedConversation();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const { send, answer, done } = useSendMessageWithSse(
    api.completeExternalConversation,
  );

  const sendMessage = useCallback(
    async (message: Message, id?: string) => {
      const res = await send({
        conversation_id: id ?? conversationId,
        quote: false,
        messages: [...(conversation?.message ?? []), message],
      });

      if (res && (res?.response.status !== 200 || res?.data?.retcode !== 0)) {
        // cancel loading
        setValue(message.content);
        removeLatestMessage();
      }
    },
    [
      conversationId,
      conversation?.message,
      // fetchConversation,
      removeLatestMessage,
      setValue,
      send,
      // setCurrentConversation,
    ],
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
        addNewestConversation({
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
    [addNewestConversation, done, handleSendMessage, setValue, value],
  );

  return {
    handlePressEnter,
    handleInputChange,
    value,
    loading: !done,
  };
};

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();

  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
  };
};
