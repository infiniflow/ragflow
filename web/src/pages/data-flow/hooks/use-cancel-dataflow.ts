import { useCancelDataflow } from '@/hooks/use-agent-request';
import { useCallback } from 'react';

export function useCancelCurrentDataflow({
  messageId,
  setMessageId,
  hideLogSheet,
}: {
  messageId: string;
  setMessageId: (messageId: string) => void;
  hideLogSheet(): void;
}) {
  const { cancelDataflow } = useCancelDataflow();

  const handleCancel = useCallback(async () => {
    const code = await cancelDataflow(messageId);
    if (code === 0) {
      setMessageId('');
      hideLogSheet();
    }
  }, [cancelDataflow, hideLogSheet, messageId, setMessageId]);

  return { handleCancel };
}
