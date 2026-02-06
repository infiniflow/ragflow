import { ChatSearchParams } from '@/constants/chat';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IMessage } from '@/interfaces/database/chat';
import { generateConversationId } from '@/utils/chat';
import { useCallback, useMemo } from 'react';
import { useSearchParams } from 'react-router';
import { useSetConversation } from './use-set-conversation';

/**
 * Consolidated hook for managing chat URL parameters (conversationId and isNew)
 * Replaces: useClickConversationCard from use-chat-request.ts and useSetChatRouteParams from use-set-chat-route.ts
 */
export const useChatUrlParams = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );

  const setConversationId = useCallback(
    (conversationId: string) => {
      newQueryParameters.set(ChatSearchParams.ConversationId, conversationId);
      setSearchParams(newQueryParameters);
    },
    [setSearchParams, newQueryParameters],
  );

  const setIsNew = useCallback(
    (isNew: string) => {
      newQueryParameters.set(ChatSearchParams.isNew, isNew);
      setSearchParams(newQueryParameters);
    },
    [setSearchParams, newQueryParameters],
  );

  const getIsNew = useCallback(() => {
    return newQueryParameters.get(ChatSearchParams.isNew);
  }, [newQueryParameters]);

  const setConversationBoth = useCallback(
    (conversationId: string, isNew: string) => {
      newQueryParameters.set(ChatSearchParams.ConversationId, conversationId);
      newQueryParameters.set(ChatSearchParams.isNew, isNew);
      setSearchParams(newQueryParameters);
    },
    [setSearchParams, newQueryParameters],
  );

  return {
    setConversationId,
    setIsNew,
    getIsNew,
    setConversationBoth,
  };
};

export function useCreateConversationBeforeSendMessage() {
  const { conversationId, isNew } = useGetChatSearchParams();
  const { setConversation } = useSetConversation();
  const { setIsNew, setConversationBoth } = useChatUrlParams();

  // Create conversation if it doesn't exist
  const createConversationBeforeSendMessage = useCallback(
    async (value: string) => {
      let currentMessages: Array<IMessage> = [];
      const currentConversationId = generateConversationId();
      if (conversationId === '' || isNew === 'true') {
        if (conversationId === '') {
          setConversationBoth(currentConversationId, 'true');
        }
        const data = await setConversation(
          value,
          true,
          conversationId || currentConversationId,
        );
        if (data.code !== 0) {
          return;
        } else {
          setIsNew('');
          currentMessages = data.data.message;
        }
      }

      const targetConversationId = conversationId || currentConversationId;

      return {
        targetConversationId,
        currentMessages,
      };
    },
    [conversationId, isNew, setConversation, setConversationBoth, setIsNew],
  );

  return {
    createConversationBeforeSendMessage,
  };
}
