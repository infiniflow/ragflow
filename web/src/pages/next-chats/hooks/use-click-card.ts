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
      // Switch to a fresh controller WITHOUT aborting the previous one.
      // Aborting the in-flight completion request can cancel server-side
      // persistence of the just-asked question, so switching conversations
      // right after asking would lose the question. Let the old SSE finish
      // in the background; the unmounted SingleChatBox's setState becomes
      // a no-op and the server keeps the message.
      setController(new AbortController());
    },
    [setConversationBoth],
  );

  return { controller, handleConversationCardClick, stopOutputMessage };
}
