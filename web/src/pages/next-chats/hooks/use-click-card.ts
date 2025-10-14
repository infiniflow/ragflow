import { useClickConversationCard } from '@/hooks/use-chat-request';
import { useCallback, useState } from 'react';

export function useHandleClickConversationCard() {
  const [controller, setController] = useState(new AbortController());
  const { handleClickConversation } = useClickConversationCard();

  const stopOutputMessage = useCallback(() => {
    setController((pre) => {
      pre.abort();
      return new AbortController();
    });
  }, []);

  const handleConversationCardClick = useCallback(
    (conversationId: string, isNew: boolean) => {
      handleClickConversation(conversationId, isNew ? 'true' : '');
      stopOutputMessage();
    },
    [handleClickConversation, stopOutputMessage],
  );

  return { controller, handleConversationCardClick, stopOutputMessage };
}
