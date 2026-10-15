import { useFetchPipelineDslByPipelineId } from '@/hooks/use-agent-request';
import { DSL } from '@/interfaces/database/agent';
import { useEffect, useLayoutEffect } from 'react';
import { useParams } from 'react-router';
import { useSetGraphInfo } from '../agent/hooks/use-set-graph';
import useGraphStore from '../agent/store';
import { dslToGraph } from '../agent/utils/dsl-bridge';

// Loads a built-in pipeline's DSL into the shared graph store for the
// read-only canvas. Mirrors useFetchDataOnMount (editing canvas) but sources
// the DSL from the built-in pipeline endpoint instead of a user agent.
export const useFetchBuiltinPipelineDataOnMount = () => {
  const { id } = useParams();
  const { dsl, loading } = useFetchPipelineDslByPipelineId(id, true);
  const setGraphInfo = useSetGraphInfo();
  const setClickedNodeId = useGraphStore((state) => state.setClickedNodeId);

  // The graph store outlives a route transition. Empty it before nodes mount
  // so nothing from a previously opened canvas leaks into this one.
  useLayoutEffect(() => {
    if (id) {
      setGraphInfo({ nodes: [], edges: [] });
    }
  }, [id, setGraphInfo]);

  useEffect(() => {
    if (!dsl?.graph) {
      return;
    }
    setGraphInfo(dslToGraph(dsl as DSL));
  }, [setGraphInfo, dsl]);

  // The store is a module-level singleton shared with the editing canvas:
  // leave it empty on the way out, and drop the clicked-node pointer so a
  // stale form-sheet target cannot survive the page switch.
  useEffect(() => {
    return () => {
      setGraphInfo({ nodes: [], edges: [] });
      setClickedNodeId('');
    };
  }, [setGraphInfo, setClickedNodeId]);

  return { loading };
};
