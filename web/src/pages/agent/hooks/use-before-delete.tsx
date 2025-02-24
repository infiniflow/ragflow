import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { OnBeforeDelete } from '@xyflow/react';
import { Operator } from '../constant';
import useGraphStore from '../store';

const UndeletableNodes = [Operator.Begin, Operator.IterationStart];

export function useBeforeDelete() {
  const getOperatorTypeFromId = useGraphStore(
    (state) => state.getOperatorTypeFromId,
  );
  const handleBeforeDelete: OnBeforeDelete<RAGFlowNodeType> = async ({
    nodes, // Nodes to be deleted
    edges, // Edges to be deleted
  }) => {
    const toBeDeletedNodes = nodes.filter((node) => {
      const operatorType = node.data?.label as Operator;
      if (operatorType === Operator.Begin) {
        return false;
      }

      if (
        operatorType === Operator.IterationStart &&
        !nodes.some((x) => x.id === node.parentId)
      ) {
        return false;
      }

      return true;
    });

    const toBeDeletedEdges = edges.filter((edge) => {
      const sourceType = getOperatorTypeFromId(edge.source) as Operator;
      const downStreamNodes = nodes.filter((x) => x.id === edge.target);

      // This edge does not need to be deleted, the range of edges that do not need to be deleted is smaller, so consider the case where it does not need to be deleted
      if (
        UndeletableNodes.includes(sourceType) && // Upstream node is Begin or IterationStart
        downStreamNodes.length === 0 // Downstream node does not exist in the nodes to be deleted
      ) {
        if (!nodes.some((x) => x.id === edge.source)) {
          return true; // Can be deleted
        }
        return false; // Cannot be deleted
      }

      return true;
    });

    return {
      nodes: toBeDeletedNodes,
      edges: toBeDeletedEdges,
    };
  };

  return { handleBeforeDelete };
}
