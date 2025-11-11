import { useCancelConversation } from '@/hooks/use-agent-request';
import { useCallback } from 'react';

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
  const handleBeforeUnload = () => {
    if (chatVisible) {
      stopMessage(taskId);
    }
  };

  window.addEventListener('beforeunload', handleBeforeUnload);

  return () => {
    window.removeEventListener('beforeunload', handleBeforeUnload);
  };
}
