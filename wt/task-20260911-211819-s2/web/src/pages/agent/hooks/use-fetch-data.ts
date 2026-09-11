import { useFetchAgent } from '@/hooks/use-agent-request';
import { useEffect, useLayoutEffect } from 'react';
import { useParams } from 'react-router';
import { dslToGraph } from '../utils/dsl-bridge';
import { useSetGraphInfo } from './use-set-graph';

export const useFetchDataOnMount = () => {
  const { id } = useParams();
  const { loading, data, refetch } = useFetchAgent();
  const setGraphInfo = useSetGraphInfo();

  // The graph store outlives a route transition. Empty it before retrieval
  // nodes mount for the next agent, so they cannot query the previous agent's
  // dataset ids while the new detail request is pending.
  useLayoutEffect(() => {
    if (id) {
      setGraphInfo({ nodes: [], edges: [] });
    }
  }, [id, setGraphInfo]);

  useEffect(() => {
    if (!data?.dsl) {
      return;
    }
    setGraphInfo(dslToGraph(data.dsl));
  }, [setGraphInfo, data]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { loading, flowDetail: data };
};
