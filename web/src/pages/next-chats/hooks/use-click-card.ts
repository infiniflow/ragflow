import { useCallback, useState } from 'react';
import { useChatUrlParams } from './use-chat-url';

export function useHandleClickConversationCard() {
  const [controller, setController] = useState(new AbortController());
  const { setConversationBoth } = useChatUrlParams();

  const stopOutputMessage = useCallback(() => {
    setController((pre) => {
      pre.abort();
      return new AbortController();
    });
  }, []);

  const handleConversationCardClick = useCallback(
    (conversationId: string, isNew: boolean) => {
      setConversationBoth(conversationId, isNew ? 'true' : '');
      stopOutputMessage();
    },
    [setConversationBoth, stopOutputMessage],
  );

  return { controller, handleConversationCardClick, stopOutputMessage };
}
