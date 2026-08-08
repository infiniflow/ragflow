import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { Edge, Node, OnBeforeDelete } from '@xyflow/react';
import { Operator } from '../constant';
import useGraphStore, { collectDeletionNodeIds } from '../store';
import { deleteAllDownstreamAgentsAndTool } from '../utils/delete-node';

export function useBeforeDelete() {
  const {
    getOperatorTypeFromId,
    getNode,
    nodes: graphNodes,
    edges: graphEdges,
  } = useGraphStore((state) => state);

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

    toBeDeletedNodes
      .filter((node) => node.data?.label === Operator.Iteration)
      .forEach((node) => {
        collectDeletionNodeIds(graphNodes, graphEdges, node.id)
          .filter((nodeId) => nodeId !== node.id)
          .forEach((nodeId) => {
            const currentNode = getNode(nodeId);
            if (currentNode && toBeDeletedNodes.every((x) => x.id !== nodeId)) {
              toBeDeletedNodes.push(currentNode);
            }
          });
      });

    // Delete the agent and tool nodes downstream of the agent node
    if (nodes.some(agentPredicate)) {
      nodes.filter(agentPredicate).forEach((node) => {
        const { downstreamAgentAndToolNodeIds } =
          deleteAllDownstreamAgentsAndTool(node.id, graphEdges);

        downstreamAgentAndToolNodeIds.forEach((nodeId) => {
          const currentNode = getNode(nodeId);
          if (toBeDeletedNodes.every((x) => x.id !== nodeId) && currentNode) {
            toBeDeletedNodes.push(currentNode);
          }
        });
      });
    }

    const toBeDeletedNodeIdSet = new Set(
      toBeDeletedNodes.map((node) => node.id),
    );
    const discardedNodeIds = new Set(
      nodes
        .filter((node) => !toBeDeletedNodeIdSet.has(node.id))
        .map((node) => node.id),
    );

    const allowEdge = (edge: Edge) =>
      !discardedNodeIds.has(edge.source) && !discardedNodeIds.has(edge.target);

    const edgeById = new Map<string, Edge>();
    for (const edge of edges) {
      if (allowEdge(edge)) {
        edgeById.set(edge.id, edge);
      }
    }
    for (const edge of graphEdges) {
      if (
        (toBeDeletedNodeIdSet.has(edge.source) ||
          toBeDeletedNodeIdSet.has(edge.target)) &&
        allowEdge(edge)
      ) {
        edgeById.set(edge.id, edge);
      }
    }

    return {
      nodes: toBeDeletedNodes,
      edges: [...edgeById.values()],
    };
  };

  return { handleBeforeDelete };
}
