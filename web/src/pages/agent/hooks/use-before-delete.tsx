import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { Node, OnBeforeDelete } from '@xyflow/react';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { deleteAllDownstreamAgentsAndTool } from '../utils/delete-node';

const UndeletableNodes = [Operator.Begin, Operator.IterationStart];

export function useBeforeDelete() {
  const { getOperatorTypeFromId, getNode } = useGraphStore((state) => state);

  const agentPredicate = (node: Node) => {
    return getOperatorTypeFromId(node.id) === Operator.Agent;
  };

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

    // Delete the agent and tool nodes downstream of the agent node
    if (nodes.some(agentPredicate)) {
      nodes.filter(agentPredicate).forEach((node) => {
        const { downstreamAgentAndToolEdges, downstreamAgentAndToolNodeIds } =
          deleteAllDownstreamAgentsAndTool(node.id, edges);

        downstreamAgentAndToolNodeIds.forEach((nodeId) => {
          const currentNode = getNode(nodeId);
          if (toBeDeletedNodes.every((x) => x.id !== nodeId) && currentNode) {
            toBeDeletedNodes.push(currentNode);
          }
        });

        downstreamAgentAndToolEdges.forEach((edge) => {
          if (toBeDeletedEdges.every((x) => x.id !== edge.id)) {
            toBeDeletedEdges.push(edge);
          }
        });
      }, []);
    }

    return {
      nodes: toBeDeletedNodes,
      edges: toBeDeletedEdges,
    };
  };

  return { handleBeforeDelete };
}
