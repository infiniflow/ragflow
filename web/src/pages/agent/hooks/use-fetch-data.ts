import { useFetchAgent } from '@/hooks/use-agent-request';
import { useEffect } from 'react';
import { dslToGraph } from '../utils/dsl-bridge';
import { useSetGraphInfo } from './use-set-graph';

export const useFetchDataOnMount = () => {
  const { loading, data, refetch } = useFetchAgent();
  const setGraphInfo = useSetGraphInfo();

  useEffect(() => {
    setGraphInfo(dslToGraph(data?.dsl));
  }, [setGraphInfo, data]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { loading, flowDetail: data };
};
