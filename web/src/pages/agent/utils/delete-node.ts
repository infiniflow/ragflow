import { Edge } from '@xyflow/react';
import { filterAllDownstreamAgentAndToolNodeIds } from './filter-downstream-nodes';

// Delete all downstream agent and tool operators of the current agent operator
export function deleteAllDownstreamAgentsAndTool(
  nodeId: string,
  edges: Edge[],
) {
  const downstreamAgentAndToolNodeIds = filterAllDownstreamAgentAndToolNodeIds(
    edges,
    [nodeId],
  );

  const downstreamNodeIdSet = new Set(downstreamAgentAndToolNodeIds);
  const downstreamAgentAndToolEdges = edges.filter(
    (edge) =>
      downstreamNodeIdSet.has(edge.source) ||
      downstreamNodeIdSet.has(edge.target),
  );

  return {
    downstreamAgentAndToolNodeIds,
    downstreamAgentAndToolEdges,
  };
}
