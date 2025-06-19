import { useFetchAgent } from '@/hooks/use-agent-request';
import { IGraph } from '@/interfaces/database/flow';
import { useEffect } from 'react';
import { useSetGraphInfo } from './use-set-graph';

export const useFetchDataOnMount = () => {
  const { loading, data, refetch } = useFetchAgent();
  const setGraphInfo = useSetGraphInfo();

  useEffect(() => {
    setGraphInfo(data?.dsl?.graph ?? ({} as IGraph));
  }, [setGraphInfo, data]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { loading, flowDetail: data };
};
