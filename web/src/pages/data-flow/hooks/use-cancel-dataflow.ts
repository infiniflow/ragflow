import { useCancelDataflow } from '@/hooks/use-agent-request';
import { useCallback } from 'react';

export function useCancelCurrentDataflow({
  messageId,
  setMessageId,
}: {
  messageId: string;
  setMessageId: (messageId: string) => void;
}) {
  const { cancelDataflow } = useCancelDataflow();

  const handleCancel = useCallback(async () => {
    const code = await cancelDataflow(messageId);
    if (code === 0) {
      setMessageId('');
    }
  }, [cancelDataflow, messageId, setMessageId]);

  return { handleCancel };
}
