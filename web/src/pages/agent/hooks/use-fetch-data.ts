import { useFetchFlow } from '@/hooks/flow-hooks';
import { IGraph } from '@/interfaces/database/flow';
import { useEffect } from 'react';
import { useSetGraphInfo } from './use-set-graph';

export const useFetchDataOnMount = () => {
  const { loading, data, refetch } = useFetchFlow();
  const setGraphInfo = useSetGraphInfo();

  useEffect(() => {
    setGraphInfo(data?.dsl?.graph ?? ({} as IGraph));
  }, [setGraphInfo, data]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { loading, flowDetail: data };
};
