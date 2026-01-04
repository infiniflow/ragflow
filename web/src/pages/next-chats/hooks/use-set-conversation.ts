import { MessageType } from '@/constants/chat';
import { useUpdateConversation } from '@/hooks/use-chat-request';
import { useCallback } from 'react';
import { useParams } from 'react-router';

export const useSetConversation = () => {
  const { id: dialogId } = useParams();
  const { updateConversation } = useUpdateConversation();

  const setConversation = useCallback(
    async (
      message: string,
      isNew: boolean = false,
      conversationId?: string,
    ) => {
      const data = await updateConversation({
        dialog_id: dialogId,
        name: message,
        is_new: isNew,
        conversation_id: conversationId,
        message: [
          {
            role: MessageType.Assistant,
            content: message,
            conversationId,
          },
        ],
      });

      return data;
    },
    [updateConversation, dialogId],
  );

  return { setConversation };
};
