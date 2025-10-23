import { useFetchMessageTrace } from '@/hooks/use-agent-request';
import { isEmpty } from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';

export function useFetchPipelineLog(logSheetVisible: boolean) {
  const {
    setMessageId,
    data,
    loading,
    messageId,
    setISStopFetchTrace,
    isStopFetchTrace,
  } = useFetchMessageTrace();

  const isCompleted = useMemo(() => {
    if (Array.isArray(data)) {
      const latest = data?.at(-1);
      return (
        latest?.component_id === 'END' && !isEmpty(latest?.trace[0].message)
      );
    }
    return false;
  }, [data]);

  const isLogEmpty = !data || !data.length;

  const stopFetchTrace = useCallback(() => {
    setISStopFetchTrace(true);
  }, [setISStopFetchTrace]);

  // cancel request
  useEffect(() => {
    if (isCompleted) {
      stopFetchTrace();
    }
  }, [isCompleted, stopFetchTrace]);

  useEffect(() => {
    if (logSheetVisible) {
      setISStopFetchTrace(false);
    }
  }, [logSheetVisible, setISStopFetchTrace]);

  return {
    logs: data,
    isLogEmpty,
    isCompleted,
    loading,
    isParsing: !isLogEmpty && !isCompleted && !isStopFetchTrace,
    messageId,
    setMessageId,
    stopFetchTrace,
  };
}

export type UseFetchLogReturnType = ReturnType<typeof useFetchPipelineLog>;
