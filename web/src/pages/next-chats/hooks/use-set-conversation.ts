import { useCreateSession } from '@/hooks/use-chat-request';
import { useCallback } from 'react';
import { useParams } from 'react-router';

export const useSetConversation = () => {
  const { id: chatId } = useParams();
  const { createSession } = useCreateSession();

  const setConversation = useCallback(
    async (name: string) => {
      const data = await createSession({ chatId: chatId!, name });
      return data;
    },
    [createSession, chatId],
  );

  return { setConversation };
};
