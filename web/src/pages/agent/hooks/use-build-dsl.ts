import { useFetchAgent } from '@/hooks/use-agent-request';
import { GlobalVariableType } from '@/interfaces/database/agent';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { useCallback } from 'react';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { buildDslComponentsByGraph, buildDslGlobalVariables } from '../utils';

export const useBuildDslData = () => {
  const { data } = useFetchAgent();
  const { nodes, edges } = useGraphStore((state) => state);

  const buildDslData = useCallback(
    (
      currentNodes?: RAGFlowNodeType[],
      otherParam?: { globalVariables: Record<string, GlobalVariableType> },
    ) => {
      const nodesToProcess = currentNodes ?? nodes;

      // Filter out placeholder nodes and related edges
      const filteredNodes = nodesToProcess.filter(
        (node) => node.data?.label !== Operator.Placeholder,
      );

      const filteredEdges = edges.filter((edge) => {
        const sourceNode = nodesToProcess.find(
          (node) => node.id === edge.source,
        );
        const targetNode = nodesToProcess.find(
          (node) => node.id === edge.target,
        );
        return (
          sourceNode?.data?.label !== Operator.Placeholder &&
          targetNode?.data?.label !== Operator.Placeholder
        );
      });

      const dslComponents = buildDslComponentsByGraph(
        filteredNodes,
        filteredEdges,
        data.dsl.components,
      );

      const globalVariables = buildDslGlobalVariables(
        data.dsl,
        otherParam?.globalVariables,
      );
      return {
        ...data.dsl,
        ...globalVariables,
        graph: { nodes: filteredNodes, edges: filteredEdges },
        components: dslComponents,
      };
    },
    [data.dsl, edges, nodes],
  );

  return { buildDslData };
};
