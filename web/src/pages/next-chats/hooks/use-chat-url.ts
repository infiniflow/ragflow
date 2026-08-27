import { ChatSearchParams } from '@/constants/chat';
import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { IMessage } from '@/interfaces/database/chat';
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
  const { setConversationBoth } = useChatUrlParams();

  // Create conversation if it doesn't exist
  const createConversationBeforeSendMessage = useCallback(
    async (value: string) => {
      let currentMessages: Array<IMessage> = [];
      if (conversationId === '' || isNew === 'true') {
        const data = await setConversation(value);
        if (!data || data.code !== 0) {
          return;
        }
        const backendConvId = data.data.id;
        setConversationBoth(backendConvId, '');
        currentMessages = data.data.messages;
        return {
          targetConversationId: backendConvId,
          currentMessages,
        };
      }

      return {
        targetConversationId: conversationId,
        currentMessages,
      };
    },
    [conversationId, isNew, setConversation, setConversationBoth],
  );

  return {
    createConversationBeforeSendMessage,
  };
}

export type CreateConversationBeforeSendMessageType = ReturnType<
  typeof useCreateConversationBeforeSendMessage
>['createConversationBeforeSendMessage'];

export type CreateConversationBeforeSendMessageReturnType = Awaited<
  ReturnType<CreateConversationBeforeSendMessageType>
>;
