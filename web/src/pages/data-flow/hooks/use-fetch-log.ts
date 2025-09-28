import { useFetchMessageTrace } from '@/hooks/use-agent-request';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';

export function useFetchLog() {
  const { setMessageId, data, loading, messageId } =
    useFetchMessageTrace(false);

  const isCompleted = useMemo(() => {
    if (Array.isArray(data)) {
      const latest = data?.at(-1);
      return (
        latest?.component_id === 'END' && !isEmpty(latest?.trace[0].message)
      );
    }
    return true;
  }, [data]);

  const isLogEmpty = !data || !data.length;

  return {
    data,
    isLogEmpty,
    isCompleted,
    loading,
    isParsing: !isLogEmpty && !isCompleted,
    messageId,
    setMessageId,
  };
}
