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

// Get the node ids hanging below one specific bottom handle of an agent node:
// the handle's own children, plus their whole sub-agent/tool subtree
export function filterBottomSubtreeNodeIds(
  edges: Edge[],
  nodeId: string,
  handleId: string,
) {
  return filterAllDownstreamNodeIds(edges, [nodeId], (edge: Edge) =>
    edge.source === nodeId
      ? edge.sourceHandle === handleId
      : edge.sourceHandle === NodeHandleId.AgentBottom ||
        edge.sourceHandle === NodeHandleId.Tool,
  );
}

export type CollapsedBottomHandles = Record<string, string[]>;

// Toggle one bottom handle in the collapsed map, dropping the node entry
// when it has no collapsed handles left
export function toggleCollapsedBottomHandle(
  collapsed: CollapsedBottomHandles,
  nodeId: string,
  handleId: string,
): CollapsedBottomHandles {
  const handleIds = collapsed[nodeId] ?? [];
  const nextHandleIds = handleIds.includes(handleId)
    ? handleIds.filter((x) => x !== handleId)
    : handleIds.concat(handleId);

  const next = { ...collapsed };
  if (nextHandleIds.length > 0) {
    next[nodeId] = nextHandleIds;
  } else {
    delete next[nodeId];
  }
  return next;
}

// Union of the node/edge ids hidden by all collapsed subtrees, so expanding
// one handle keeps nodes hidden by another (e.g. nested) collapse hidden
export function filterCollapsedHiddenIds(
  edges: Edge[],
  collapsed: CollapsedBottomHandles,
) {
  const hiddenNodeIds = new Set<string>();
  Object.entries(collapsed).forEach(([nodeId, handleIds]) => {
    handleIds.forEach((handleId) => {
      filterBottomSubtreeNodeIds(edges, nodeId, handleId).forEach((x) =>
        hiddenNodeIds.add(x),
      );
    });
  });

  const hiddenEdgeIds = new Set(
    edges
      .filter((x) => hiddenNodeIds.has(x.source) || hiddenNodeIds.has(x.target))
      .map((x) => x.id),
  );

  return { hiddenNodeIds, hiddenEdgeIds };
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
