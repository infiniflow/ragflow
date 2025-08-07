import { useClickConversationCard } from '@/hooks/use-chat-request';
import { useCallback, useState } from 'react';

export function useHandleClickConversationCard() {
  const [controller, setController] = useState(new AbortController());
  const { handleClickConversation } = useClickConversationCard();

  const handleConversationCardClick = useCallback(
    (conversationId: string, isNew: boolean) => {
      handleClickConversation(conversationId, isNew ? 'true' : '');
      setController((pre) => {
        pre.abort();
        return new AbortController();
      });
    },
    [handleClickConversation],
  );

  return { controller, handleConversationCardClick };
}
