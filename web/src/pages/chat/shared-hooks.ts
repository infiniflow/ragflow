import { MessageType } from '@/constants/chat';
import {
  useCompleteSharedConversation,
  useCreateSharedConversation,
  useFetchSharedConversation,
} from '@/hooks/chatHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import omit from 'lodash/omit';
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

  const createConversation = useCreateSharedConversation();
  const sharedId = currentQueryParameters.get('shared_id');
  const userId = currentQueryParameters.get('user_id');

  const setConversation = useCallback(async () => {
    console.info(sharedId);
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
  const fetchConversation = useFetchSharedConversation();
  const loading = useOneNamespaceEffectsLoading('chatModel', [
    'getExternalConversation',
  ]);

  const ref = useScrollToBottom(currentConversation);

  const addNewestConversation = useCallback((message: string) => {
    setCurrentConversation((pre) => {
      return {
        ...pre,
        message: [
          ...(pre.message ?? []),
          {
            role: MessageType.User,
            content: message,
            id: uuid(),
          } as IMessage,
          {
            role: MessageType.Assistant,
            content: '',
            id: uuid(),
            reference: [],
          } as IMessage,
        ],
      };
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
  };
};

export const useSendSharedMessage = (
  conversation: IClientConversation,
  addNewestConversation: (message: string) => void,
  removeLatestMessage: () => void,
  setCurrentConversation: Dispatch<SetStateAction<IClientConversation>>,
) => {
  const conversationId = conversation.id;
  const loading = useOneNamespaceEffectsLoading('chatModel', [
    'completeExternalConversation',
  ]);
  const setConversation = useCreateSharedConversation();
  const { handleInputChange, value, setValue } = useHandleMessageInputChange();

  const fetchConversation = useFetchSharedConversation();
  const completeConversation = useCompleteSharedConversation();

  const sendMessage = useCallback(
    async (message: string, id?: string) => {
      const retcode = await completeConversation({
        conversation_id: id ?? conversationId,
        quote: false,
        messages: [
          ...(conversation?.message ?? []).map((x: IMessage) => omit(x, 'id')),
          {
            role: MessageType.User,
            content: message,
          },
        ],
      });

      if (retcode === 0) {
        const data = await fetchConversation(conversationId);
        if (data.retcode === 0) {
          setCurrentConversation(data.data);
        }
      } else {
        // cancel loading
        setValue(message);
        removeLatestMessage();
      }
    },
    [
      conversationId,
      conversation?.message,
      fetchConversation,
      removeLatestMessage,
      setValue,
      completeConversation,
      setCurrentConversation,
    ],
  );

  const handleSendMessage = useCallback(
    async (message: string) => {
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

  const handlePressEnter = () => {
    if (!loading) {
      setValue('');
      addNewestConversation(value);
      handleSendMessage(value.trim());
    }
  };

  return {
    handlePressEnter,
    handleInputChange,
    value,
    loading,
  };
};
