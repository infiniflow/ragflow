import { useCallback } from 'react';
import { useParams } from 'umi';
import { useSetChatRouteParams } from './use-set-chat-route';
import { useSetConversation } from './use-set-conversation';

export const useCreateConversationBeforeUploadDocument = () => {
  const { setConversation } = useSetConversation();
  const { id: dialogId } = useParams();
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
