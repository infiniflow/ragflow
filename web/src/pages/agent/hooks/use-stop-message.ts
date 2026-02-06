import { useCancelConversation } from '@/hooks/use-agent-request';
import { useCallback, useEffect } from 'react';

export function useStopMessage() {
  const { cancelConversation } = useCancelConversation();

  const stopMessage = useCallback(
    (taskId?: string) => {
      if (taskId) {
        cancelConversation(taskId);
      }
    },
    [cancelConversation],
  );

  return { stopMessage };
}

export function useStopMessageUnmount(chatVisible: boolean, taskId?: string) {
  const { stopMessage } = useStopMessage();

  const handleBeforeUnload = useCallback(() => {
    if (chatVisible) {
      stopMessage(taskId);
    }
  }, [chatVisible, stopMessage, taskId]);

  useEffect(() => {
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, [handleBeforeUnload]);

  return { stopMessage };
}
