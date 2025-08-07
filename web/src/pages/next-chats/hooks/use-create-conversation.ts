import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { useCallback } from 'react';
import {
  useSetChatRouteParams,
  useSetConversation,
} from './use-send-chat-message';

export const useCreateConversationBeforeUploadDocument = () => {
  const { setConversation } = useSetConversation();
  const { dialogId } = useGetChatSearchParams();
  const { getConversationIsNew } = useSetChatRouteParams();

  const createConversationBeforeUploadDocument = useCallback(
    async (message: string) => {
      const isNew = getConversationIsNew();
      if (isNew === 'true') {
        const data = await setConversation(message, true);

        return data;
      }
    },
    [setConversation, getConversationIsNew],
  );

  return {
    createConversationBeforeUploadDocument,
    dialogId,
  };
};
