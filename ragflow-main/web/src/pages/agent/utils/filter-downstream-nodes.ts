import { Edge } from '@xyflow/react';
import { NodeHandleId } from '../constant';

// Get all downstream node ids
export function filterAllDownstreamNodeIds(
  edges: Edge[],
  nodeIds: string[],
  predicate: (edge: Edge) => boolean,
) {
  return nodeIds.reduce<string[]>((pre, nodeId) => {
    const currentEdges = edges.filter(
      (x) => x.source === nodeId && predicate(x),
    );

    const downstreamNodeIds: string[] = currentEdges.map((x) => x.target);

    const ids = downstreamNodeIds.concat(
      filterAllDownstreamNodeIds(edges, downstreamNodeIds, predicate),
    );

    ids.forEach((x) => {
      if (pre.every((y) => y !== x)) {
        pre.push(x);
      }
    });

    return pre;
  }, []);
}

// Get all downstream agent and tool operators of the current agent operator
export function filterAllDownstreamAgentAndToolNodeIds(
  edges: Edge[],
  nodeIds: string[],
) {
  return filterAllDownstreamNodeIds(
    edges,
    nodeIds,
    (edge: Edge) =>
      edge.sourceHandle === NodeHandleId.AgentBottom ||
      edge.sourceHandle === NodeHandleId.Tool,
  );
}

// Get all downstream agent operators of the current agent operator
export function filterAllDownstreamAgentNodeIds(
  edges: Edge[],
  nodeIds: string[],
) {
  return filterAllDownstreamNodeIds(
    edges,
    nodeIds,
    (edge: Edge) => edge.sourceHandle === NodeHandleId.AgentBottom,
  );
}
// The direct child agent node of the current node
export function filterDownstreamAgentNodeIds(edges: Edge[], nodeId?: string) {
  return edges
    .filter(
      (x) => x.source === nodeId && x.sourceHandle === NodeHandleId.AgentBottom,
    )
    .map((x) => x.target);
}
