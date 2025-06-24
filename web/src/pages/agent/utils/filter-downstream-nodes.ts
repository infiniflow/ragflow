import { Edge } from '@xyflow/react';
import { NodeHandleId } from '../constant';

// Get all downstream agent operators of the current agent operator
export function filterAllDownstreamAgentAndToolNodeIds(
  edges: Edge[],
  nodeIds: string[],
) {
  return nodeIds.reduce<string[]>((pre, nodeId) => {
    const currentEdges = edges.filter(
      (x) =>
        x.source === nodeId &&
        (x.sourceHandle === NodeHandleId.AgentBottom ||
          x.sourceHandle === NodeHandleId.Tool),
    );

    const downstreamNodeIds: string[] = currentEdges.map((x) => x.target);

    const ids = downstreamNodeIds.concat(
      filterAllDownstreamAgentAndToolNodeIds(edges, downstreamNodeIds),
    );

    ids.forEach((x) => {
      if (pre.every((y) => y !== x)) {
        pre.push(x);
      }
    });

    return pre;
  }, []);
}
