import { useCallback } from 'react';
import { useParams } from 'react-router';
import { useChatUrlParams } from './use-chat-url';
import { useSetConversation } from './use-set-conversation';

export const useCreateConversationBeforeUploadDocument = () => {
  const { setConversation } = useSetConversation();
  const { id: dialogId } = useParams();
  const { getIsNew } = useChatUrlParams();

  const createConversationBeforeUploadDocument = useCallback(
    async (message: string) => {
      const isNew = getIsNew();
      if (isNew === 'true') {
        const data = await setConversation(message, true);

        return data;
      }
    },
    [setConversation, getIsNew],
  );

  return {
    createConversationBeforeUploadDocument,
    dialogId,
  };
};
