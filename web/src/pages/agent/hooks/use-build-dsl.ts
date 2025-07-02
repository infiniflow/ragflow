import { useFetchFlow } from '@/hooks/flow-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { useCallback } from 'react';
import useGraphStore from '../store';
import { buildDslComponentsByGraph } from '../utils';

export const useBuildDslData = () => {
  const { data } = useFetchFlow();
  const { nodes, edges } = useGraphStore((state) => state);

  const buildDslData = useCallback(
    (currentNodes?: RAGFlowNodeType[]) => {
      const dslComponents = buildDslComponentsByGraph(
        currentNodes ?? nodes,
        edges,
        data.dsl.components,
      );

      return {
        ...data.dsl,
        graph: { nodes: currentNodes ?? nodes, edges },
        components: dslComponents,
      };
    },
    [data.dsl, edges, nodes],
  );

  return { buildDslData };
};
