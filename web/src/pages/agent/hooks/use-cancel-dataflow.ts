import { useCancelDataflow } from '@/hooks/use-agent-request';
import { useCallback } from 'react';

export function useCancelCurrentDataflow({
  messageId,
  stopFetchTrace,
}: {
  messageId: string;
  stopFetchTrace(): void;
}) {
  const { cancelDataflow } = useCancelDataflow();

  const handleCancel = useCallback(async () => {
    const code = await cancelDataflow(messageId);
    if (code === 0) {
      stopFetchTrace();
    }
  }, [cancelDataflow, messageId, stopFetchTrace]);

  return { handleCancel };
}
