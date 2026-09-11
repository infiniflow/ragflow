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

  const downstreamAgentAndToolEdges = downstreamAgentAndToolNodeIds.reduce<
    Edge[]
  >((pre, cur) => {
    const relatedEdges = edges.filter(
      (x) => x.source === cur || x.target === cur,
    );

    relatedEdges.forEach((x) => {
      if (!pre.some((y) => y.id !== x.id)) {
        pre.push(x);
      }
    });

    return pre;
  }, []);

  return {
    downstreamAgentAndToolNodeIds,
    downstreamAgentAndToolEdges,
  };
}
