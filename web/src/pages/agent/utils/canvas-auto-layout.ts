import dagre from '@dagrejs/dagre';

/**
 * Left-to-right tidy layout for the agent canvas, matching n8n's tidy-up:
 * each output keeps a vertical band so its downstream does not cross the
 * other outputs, disconnected parts stack vertically, and nodes attached
 * under an agent stay in a column beneath it.
 * Positions stay anchored to the current top-left so the graph does not jump.
 *
 * React Flow node origin on this canvas is [0.5, 0]: position.x is the
 * horizontal center, position.y is the top. Layout math uses top-left boxes
 * and converts back at the boundary.
 */

export const CanvasAutoLayoutSpacing = {
  grid: 16,
  rankGap: 16 * 8,
  nodeGap: 16 * 6,
  subgraphGap: 16 * 8,
  attachmentGapX: 16 * 3,
  attachmentGapY: 16 * 8,
  groupPadding: 16 * 3,
  noteBottomPadding: 16 * 4,
} as const;

const DEFAULT_NODE_WIDTH = 200;
const DEFAULT_NODE_HEIGHT = 72;
const MIN_GROUP_WIDTH = 16 * 20;
const MIN_GROUP_HEIGHT = 16 * 12;

const ATTACHMENT_SOURCE_HANDLES = new Set(['tool', 'agentBottom']);

export type CanvasLayoutNode = {
  id: string;
  type?: string;
  parentId?: string;
  position: { x: number; y: number };
  width?: number | null;
  height?: number | null;
  measured?: { width?: number | null; height?: number | null } | null;
  data?: {
    label?: string;
    form?: {
      items?: Array<{ uuid?: string } | null>;
      conditions?: unknown[];
    };
  };
};

export type CanvasLayoutEdge = {
  source: string;
  target: string;
  sourceHandle?: string | null;
};

export type CanvasLayoutUpdate = {
  x: number;
  y: number;
  width?: number;
  height?: number;
};

type Box = { x: number; y: number; width: number; height: number };
type Size = { width: number; height: number };

type Cluster = {
  rootId: string;
  relative: Map<string, Box>;
  width: number;
  height: number;
};

type DagreNodeConfig = {
  width: number;
  height: number;
  x?: number;
  y?: number;
};
type DagreGraph = dagre.graphlib.Graph;

function positive(value: number | null | undefined, fallback: number): number {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return value;
  }
  return fallback;
}

function snap(value: number): number {
  const grid = CanvasAutoLayoutSpacing.grid;
  return Math.round(value / grid) * grid;
}

function ceilToGrid(value: number): number {
  const grid = CanvasAutoLayoutSpacing.grid;
  return Math.ceil(value / grid) * grid;
}

function isAutoLayoutNote(node: CanvasLayoutNode): boolean {
  return node.type === 'noteNode' || node.data?.label === 'Note';
}

function isPlaceholder(node: CanvasLayoutNode): boolean {
  return node.type === 'placeholderNode' || node.data?.label === 'Placeholder';
}

function isAttachmentEdge(edge: CanvasLayoutEdge): boolean {
  return (
    edge.sourceHandle != null &&
    ATTACHMENT_SOURCE_HANDLES.has(edge.sourceHandle)
  );
}

function nodeSize(node: CanvasLayoutNode): Size {
  return {
    width: positive(node.measured?.width ?? node.width, DEFAULT_NODE_WIDTH),
    height: positive(node.measured?.height ?? node.height, DEFAULT_NODE_HEIGHT),
  };
}

function boxFromPosition(node: CanvasLayoutNode, size: Size): Box {
  return {
    x: node.position.x - size.width / 2,
    y: node.position.y,
    width: size.width,
    height: size.height,
  };
}

function positionFromBox(box: Box): { x: number; y: number } {
  return { x: box.x + box.width / 2, y: box.y };
}

function boundsOf(boxes: Iterable<Box>): Box | undefined {
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  let count = 0;
  for (const box of boxes) {
    count += 1;
    minX = Math.min(minX, box.x);
    minY = Math.min(minY, box.y);
    maxX = Math.max(maxX, box.x + box.width);
    maxY = Math.max(maxY, box.y + box.height);
  }
  if (count === 0) return undefined;
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
}

function intersects(container: Box, target: Box, padding = 0): boolean {
  const targetBox = {
    x: target.x - padding,
    y: target.y - padding,
    width: target.width + padding * 2,
    height: target.height + padding * 2,
  };
  return !(
    targetBox.x + targetBox.width < container.x ||
    targetBox.x > container.x + container.width ||
    targetBox.y + targetBox.height < container.y ||
    targetBox.y > container.y + container.height
  );
}

function containsBox(parent: Box, child: Box): boolean {
  return (
    child.x >= parent.x - 0.5 &&
    child.y >= parent.y - 0.5 &&
    child.x + child.width <= parent.x + parent.width + 0.5 &&
    child.y + child.height <= parent.y + parent.height + 0.5
  );
}

function comparePosition(
  a: { x: number; y: number },
  b: { x: number; y: number },
): number {
  if (a.y !== b.y) return a.y - b.y;
  return a.x - b.x;
}

function createGraph(options: {
  rankdir: 'LR' | 'TB';
  nodesep: number;
  ranksep: number;
  align?: 'UL';
}): DagreGraph {
  const graph = new dagre.graphlib.Graph();
  graph.setGraph({
    rankdir: options.rankdir,
    nodesep: options.nodesep,
    edgesep: options.nodesep,
    ranksep: options.ranksep,
    align: options.align,
    marginx: 0,
    marginy: 0,
  });
  graph.setDefaultEdgeLabel(() => ({}));
  return graph;
}

function runLayout(graph: DagreGraph): void {
  // n8n keeps the DFS visit order (edges inserted in handle order) and skips
  // the barycenter sweep, which otherwise inverts a node's outputs.
  dagre.layout(graph, { disableOptimalOrderHeuristic: true });
}

function boxFromDagreNode(node: DagreNodeConfig): Box {
  return {
    x: (node.x ?? 0) - node.width / 2,
    y: (node.y ?? 0) - node.height / 2,
    width: node.width,
    height: node.height,
  };
}

export function handleRank(
  node: { data?: CanvasLayoutNode['data'] } | undefined,
  sourceHandle?: string | null,
): number {
  if (!sourceHandle) return 0;
  const items = node?.data?.form?.items;
  if (Array.isArray(items)) {
    const index = items.findIndex(
      (item) => item != null && item.uuid === sourceHandle,
    );
    if (index >= 0) return index;
  }
  const conditions = node?.data?.form?.conditions;
  if (Array.isArray(conditions)) {
    if (sourceHandle === 'end_cpn_ids') return conditions.length;
    const match = /^Case (\d+)$/.exec(sourceHandle);
    if (match) return Number(match[1]) - 1;
  }
  if (sourceHandle === 'start') return 0;
  if (sourceHandle === 'agentException') return 1;
  return 0;
}

function compareFlowEdges(
  a: { source: string; target: string; sourceHandle?: string | null },
  b: { source: string; target: string; sourceHandle?: string | null },
  byId: Map<string, CanvasLayoutNode>,
  orderOf: (id: string) => { x: number; y: number },
): number {
  if (a.source === b.source) {
    const rank =
      handleRank(byId.get(a.source), a.sourceHandle) -
      handleRank(byId.get(b.source), b.sourceHandle);
    if (rank !== 0) return rank;
  } else {
    const sourceOrder = comparePosition(orderOf(a.source), orderOf(b.source));
    if (sourceOrder !== 0) return sourceOrder;
  }
  return comparePosition(orderOf(a.target), orderOf(b.target));
}

function layoutBoxes(
  ids: string[],
  sizeOfId: (id: string) => Size,
  edges: Array<{
    source: string;
    target: string;
    sourceHandle?: string | null;
  }>,
  direction: 'LR' | 'TB',
  spacing: { nodesep: number; ranksep: number },
  orderOf: (id: string) => { x: number; y: number },
  byId: Map<string, CanvasLayoutNode>,
): Map<string, Box> {
  const placed = new Map<string, Box>();
  if (ids.length === 0) return placed;

  const graph = createGraph({
    rankdir: direction,
    nodesep: spacing.nodesep,
    ranksep: spacing.ranksep,
  });
  const orderedIds = [...ids].sort((a, b) =>
    comparePosition(orderOf(a), orderOf(b)),
  );
  for (const id of orderedIds) {
    const size = sizeOfId(id);
    graph.setNode(id, { width: size.width, height: size.height });
  }
  const idSet = new Set(ids);
  const orderedEdges = edges
    .filter((edge) => idSet.has(edge.source) && idSet.has(edge.target))
    .filter((edge) => edge.source !== edge.target)
    .sort((a, b) => compareFlowEdges(a, b, byId, orderOf));
  for (const edge of orderedEdges) {
    if (graph.edge(edge.source, edge.target)) continue;
    graph.setEdge(edge.source, edge.target);
  }
  runLayout(graph);
  for (const id of graph.nodes()) {
    placed.set(id, boxFromDagreNode(graph.node(id) as DagreNodeConfig));
  }
  return placed;
}

function buildClusters(
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
  sizeOfNode: (node: CanvasLayoutNode) => Size,
): { clusters: Cluster[]; hiddenIds: Set<string> } {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const ids = new Set(byId.keys());
  const attachmentEdges = edges.filter(
    (edge) =>
      isAttachmentEdge(edge) && ids.has(edge.source) && ids.has(edge.target),
  );
  const childrenOf = new Map<string, string[]>();
  const attachmentIds = new Set<string>();
  for (const edge of attachmentEdges) {
    const children = childrenOf.get(edge.source) ?? [];
    children.push(edge.target);
    childrenOf.set(edge.source, children);
    attachmentIds.add(edge.target);
  }

  const clusters: Cluster[] = [];
  const hiddenIds = new Set<string>();
  for (const rootId of childrenOf.keys()) {
    if (attachmentIds.has(rootId)) continue;
    const memberIds = [rootId];
    const seen = new Set([rootId]);
    const queue = [...(childrenOf.get(rootId) ?? [])];
    while (queue.length > 0) {
      const next = queue.shift();
      if (!next || seen.has(next)) continue;
      seen.add(next);
      memberIds.push(next);
      hiddenIds.add(next);
      for (const child of childrenOf.get(next) ?? []) queue.push(child);
    }

    const memberEdges = attachmentEdges
      .filter((edge) => seen.has(edge.source) && seen.has(edge.target))
      .map((edge) => ({ source: edge.source, target: edge.target }));
    const raw = layoutBoxes(
      memberIds,
      (id) => sizeOfNode(byId.get(id)!),
      memberEdges,
      'TB',
      {
        nodesep: CanvasAutoLayoutSpacing.attachmentGapX,
        ranksep: CanvasAutoLayoutSpacing.attachmentGapY,
      },
      (id) => byId.get(id)!.position,
      byId,
    );
    const bounds = boundsOf(raw.values());
    if (!bounds) continue;
    const relative = new Map<string, Box>();
    for (const [id, box] of raw) {
      relative.set(id, {
        ...box,
        x: box.x - bounds.x,
        y: box.y - bounds.y,
      });
    }
    clusters.push({
      rootId,
      relative,
      width: bounds.width,
      height: bounds.height,
    });
  }

  return { clusters, hiddenIds };
}

function shiftClusterToFlowAxis(
  clusterBox: Box,
  cluster: Cluster,
  others: Map<string, Box>,
): Box {
  const root = cluster.relative.get(cluster.rootId);
  if (!root) return clusterBox;
  const rootCenter = root.y + root.height / 2;
  const correction = clusterBox.height / 2 - rootCenter;
  if (Math.abs(correction) < 0.5) return clusterBox;
  const shifted = { ...clusterBox, y: clusterBox.y + correction };
  const blocked = [...others.entries()].some(
    ([id, box]) =>
      id !== cluster.rootId &&
      intersects(shifted, box, CanvasAutoLayoutSpacing.nodeGap),
  );
  return blocked ? clusterBox : shifted;
}

function expandClusters(
  mainBoxes: Map<string, Box>,
  clusters: Cluster[],
): Map<string, Box> {
  const clusterByRoot = new Map(
    clusters.map((cluster) => [cluster.rootId, cluster]),
  );
  const placed = new Map<string, Box>();
  for (const [id, box] of mainBoxes) {
    const cluster = clusterByRoot.get(id);
    if (!cluster) {
      placed.set(id, box);
      continue;
    }
    const origin = shiftClusterToFlowAxis(box, cluster, mainBoxes);
    for (const [memberId, relative] of cluster.relative) {
      placed.set(memberId, {
        x: origin.x + relative.x,
        y: origin.y + relative.y,
        width: relative.width,
        height: relative.height,
      });
    }
  }
  return placed;
}

function layoutConnected(
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
  sizeOfNode: (node: CanvasLayoutNode) => Size,
): Map<string, Box>[] {
  if (nodes.length === 0) return [];
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const { clusters, hiddenIds } = buildClusters(nodes, edges, sizeOfNode);
  const clusterByRoot = new Map(
    clusters.map((cluster) => [cluster.rootId, cluster]),
  );
  const mainNodes = nodes.filter((node) => !hiddenIds.has(node.id));
  const mainIds = new Set(mainNodes.map((node) => node.id));

  const componentGraph = new dagre.graphlib.Graph();
  componentGraph.setDefaultEdgeLabel(() => ({}));
  for (const node of mainNodes) componentGraph.setNode(node.id, {});
  for (const edge of edges) {
    if (isAttachmentEdge(edge)) continue;
    if (!byId.has(edge.source) || !byId.has(edge.target)) continue;
    const source = hiddenIds.has(edge.source)
      ? clusterRootOf(edge.source, clusters)
      : edge.source;
    const target = hiddenIds.has(edge.target)
      ? clusterRootOf(edge.target, clusters)
      : edge.target;
    if (!source || !target || source === target) continue;
    if (!mainIds.has(source) || !mainIds.has(target)) continue;
    if (componentGraph.edge(source, target)) continue;
    componentGraph.setEdge(source, target);
  }

  const components = dagre.graphlib.alg
    .components(componentGraph)
    .map((ids) => ids.filter((id): id is string => typeof id === 'string'))
    .filter((ids) => ids.length > 0)
    .sort((a, b) => {
      const boxA = boundsOf(
        a.map((id) =>
          boxFromPosition(byId.get(id)!, sizeOfNode(byId.get(id)!)),
        ),
      );
      const boxB = boundsOf(
        b.map((id) =>
          boxFromPosition(byId.get(id)!, sizeOfNode(byId.get(id)!)),
        ),
      );
      if (!boxA || !boxB) return 0;
      return comparePosition(boxA, boxB);
    });

  return components.map((ids) => {
    const flowEdges = edges
      .filter((edge) => !isAttachmentEdge(edge))
      .map((edge) => ({
        source: hiddenIds.has(edge.source)
          ? (clusterRootOf(edge.source, clusters) ?? edge.source)
          : edge.source,
        target: hiddenIds.has(edge.target)
          ? (clusterRootOf(edge.target, clusters) ?? edge.target)
          : edge.target,
        sourceHandle: edge.sourceHandle,
      }));
    const boxes = layoutFlowByPorts(
      ids,
      (id) => {
        const cluster = clusterByRoot.get(id);
        if (cluster) return { width: cluster.width, height: cluster.height };
        return sizeOfNode(byId.get(id)!);
      },
      flowEdges,
      (id) => byId.get(id)!.position,
      byId,
    );
    return expandClusters(
      boxes,
      clusters.filter((cluster) => ids.includes(cluster.rootId)),
    );
  });
}

function clusterRootOf(
  memberId: string,
  clusters: Cluster[],
): string | undefined {
  return clusters.find((cluster) => cluster.relative.has(memberId))?.rootId;
}

function stackComponents(components: Map<string, Box>[]): Map<string, Box> {
  if (components.length === 0) return new Map();
  if (components.length === 1) return components[0];

  const graph = createGraph({
    rankdir: 'TB',
    align: 'UL',
    nodesep: CanvasAutoLayoutSpacing.subgraphGap,
    ranksep: CanvasAutoLayoutSpacing.subgraphGap,
  });
  const localBounds = components.map((component) =>
    boundsOf(component.values()),
  );
  localBounds.forEach((box, index) => {
    if (!box) return;
    graph.setNode(String(index), { width: box.width, height: box.height });
  });
  for (let index = 0; index < components.length - 1; index += 1) {
    graph.setEdge(String(index), String(index + 1));
  }
  runLayout(graph);

  const placed = new Map<string, Box>();
  components.forEach((component, index) => {
    const local = localBounds[index];
    const slot = graph.node(String(index)) as DagreNodeConfig | undefined;
    if (!local || !slot) return;
    const slotTop = (slot.y ?? 0) - slot.height / 2;
    for (const [id, box] of component) {
      placed.set(id, {
        x: box.x - local.x,
        y: box.y - local.y + slotTop,
        width: box.width,
        height: box.height,
      });
    }
  });
  return placed;
}

function anchorBoxes(
  placed: Map<string, Box>,
  originals: Map<string, Box>,
  mode: 'anchor' | 'padding',
): Map<string, Box> {
  const after = boundsOf(placed.values());
  if (!after) return new Map();

  let dx = 0;
  let dy = 0;
  if (mode === 'padding') {
    dx = CanvasAutoLayoutSpacing.groupPadding - after.x;
    dy = CanvasAutoLayoutSpacing.groupPadding - after.y;
  } else {
    const before = boundsOf(
      [...placed.keys()]
        .map((id) => originals.get(id))
        .filter((box): box is Box => !!box),
    );
    if (before) {
      dx = before.x - after.x;
      dy = before.y - after.y;
    }
  }

  const anchored = new Map<string, Box>();
  for (const [id, box] of placed) {
    const left = box.x + dx;
    const top = box.y + dy;
    const snappedLeft = snap(left + box.width / 2) - box.width / 2;
    const snappedTop = snap(top + box.height / 2) - box.height / 2;
    anchored.set(id, {
      x: snappedLeft,
      y: snappedTop,
      width: box.width,
      height: box.height,
    });
  }
  return anchored;
}

function assignForwardRanks(
  ids: string[],
  edges: Array<{ source: string; target: string }>,
): Map<string, number> {
  const idSet = new Set(ids);
  const incoming = new Map<string, string[]>();
  for (const id of ids) incoming.set(id, []);
  for (const edge of edges) {
    if (!idSet.has(edge.source) || !idSet.has(edge.target)) continue;
    if (edge.source === edge.target) continue;
    incoming.get(edge.target)?.push(edge.source);
  }

  const rank = new Map<string, number>();
  const visiting = new Set<string>();
  const visit = (id: string): number => {
    if (visiting.has(id)) return rank.get(id) ?? 0;
    const known = rank.get(id);
    if (known !== undefined) return known;
    visiting.add(id);
    let best = 0;
    for (const source of incoming.get(id) ?? []) {
      best = Math.max(best, visit(source) + 1);
    }
    visiting.delete(id);
    rank.set(id, best);
    return best;
  };
  for (const id of ids) visit(id);
  return rank;
}

/**
 * Left-to-right layout that keeps each node's outputs in handle order.
 * n8n does this by inserting edges in port order and disabling dagre's
 * crossing sweep. A sweep still inverts branches once the graph is tangled,
 * so each output owns a vertical band and its downstream stays inside it.
 * A node with several inputs sits in its nearest upstream band.
 */
function layoutFlowByPorts(
  ids: string[],
  sizeOfId: (id: string) => Size,
  edges: Array<{
    source: string;
    target: string;
    sourceHandle?: string | null;
  }>,
  orderOf: (id: string) => { x: number; y: number },
  byId: Map<string, CanvasLayoutNode>,
): Map<string, Box> {
  const placed = new Map<string, Box>();
  if (ids.length === 0) return placed;

  const idSet = new Set(ids);
  const flowEdges = edges.filter(
    (edge) =>
      idSet.has(edge.source) &&
      idSet.has(edge.target) &&
      edge.source !== edge.target,
  );
  const ranks = assignForwardRanks(ids, flowEdges);
  const primaryParent = new Map<string, string>();
  for (const id of ids) {
    const candidates = flowEdges
      .filter(
        (edge) =>
          edge.target === id &&
          (ranks.get(edge.source) ?? 0) < (ranks.get(id) ?? 0),
      )
      .sort((a, b) => {
        const rankDelta =
          (ranks.get(b.source) ?? 0) - (ranks.get(a.source) ?? 0);
        if (rankDelta !== 0) return rankDelta;
        const positionDelta = comparePosition(
          orderOf(a.source),
          orderOf(b.source),
        );
        if (positionDelta !== 0) return positionDelta;
        return (
          handleRank(byId.get(a.source), a.sourceHandle) -
          handleRank(byId.get(b.source), b.sourceHandle)
        );
      });
    const parent = candidates[0]?.source;
    if (parent) primaryParent.set(id, parent);
  }

  const children = new Map<string, string[]>();
  const roots: string[] = [];
  for (const id of ids) {
    const parent = primaryParent.get(id);
    if (!parent || !idSet.has(parent)) {
      roots.push(id);
      continue;
    }
    const list = children.get(parent) ?? [];
    list.push(id);
    children.set(parent, list);
  }
  for (const [parent, list] of children) {
    const parentNode = byId.get(parent);
    list.sort((a, b) => {
      const handleA = flowEdges.find(
        (edge) => edge.source === parent && edge.target === a,
      )?.sourceHandle;
      const handleB = flowEdges.find(
        (edge) => edge.source === parent && edge.target === b,
      )?.sourceHandle;
      const handleDelta =
        handleRank(parentNode, handleA) - handleRank(parentNode, handleB);
      if (handleDelta !== 0) return handleDelta;
      return comparePosition(orderOf(a), orderOf(b));
    });
  }
  roots.sort((a, b) => comparePosition(orderOf(a), orderOf(b)));

  const yOf = new Map<string, number>();
  const layoutTree = (
    id: string,
  ): { positions: Map<string, number>; height: number } => {
    const size = sizeOfId(id);
    const kids = children.get(id) ?? [];
    if (kids.length === 0) {
      return { positions: new Map([[id, 0]]), height: size.height };
    }

    const positions = new Map<string, number>();
    const centers: number[] = [];
    let cursor = 0;
    kids.forEach((child, index) => {
      if (index > 0) cursor += CanvasAutoLayoutSpacing.nodeGap;
      const subtree = layoutTree(child);
      for (const [childId, childY] of subtree.positions) {
        positions.set(childId, childY + cursor);
      }
      const childTop = positions.get(child) ?? cursor;
      centers.push(childTop + sizeOfId(child).height / 2);
      cursor += subtree.height;
    });

    const mid = (centers[0] + centers[centers.length - 1]) / 2;
    positions.set(id, mid - size.height / 2);

    let minY = Infinity;
    let maxY = -Infinity;
    for (const [nodeId, top] of positions) {
      const nodeSize = sizeOfId(nodeId);
      minY = Math.min(minY, top);
      maxY = Math.max(maxY, top + nodeSize.height);
    }
    if (minY !== 0) {
      for (const [nodeId, top] of positions) positions.set(nodeId, top - minY);
    }
    return { positions, height: maxY - minY };
  };

  let bandTop = 0;
  roots.forEach((root, index) => {
    if (index > 0) bandTop += CanvasAutoLayoutSpacing.nodeGap;
    const tree = layoutTree(root);
    for (const [id, top] of tree.positions) yOf.set(id, top + bandTop);
    bandTop += tree.height;
  });

  const maxRank = Math.max(0, ...[...ranks.values()]);
  const columnWidth: number[] = [];
  for (let rank = 0; rank <= maxRank; rank += 1) columnWidth[rank] = 0;
  for (const id of ids) {
    const rank = ranks.get(id) ?? 0;
    columnWidth[rank] = Math.max(columnWidth[rank], sizeOfId(id).width);
  }
  const columnX: number[] = [];
  let cursorX = 0;
  for (let rank = 0; rank <= maxRank; rank += 1) {
    columnX[rank] = cursorX;
    if (columnWidth[rank] > 0) {
      cursorX += columnWidth[rank] + CanvasAutoLayoutSpacing.rankGap;
    }
  }

  for (const id of ids) {
    const size = sizeOfId(id);
    const rank = ranks.get(id) ?? 0;
    const column = columnWidth[rank] || size.width;
    placed.set(id, {
      x: columnX[rank] + (column - size.width) / 2,
      y: yOf.get(id) ?? 0,
      width: size.width,
      height: size.height,
    });
  }
  return placed;
}

function layoutLevel(
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
  mode: 'anchor' | 'padding',
  sizeOfNode: (node: CanvasLayoutNode) => Size = nodeSize,
): Map<string, Box> {
  const originals = new Map(
    nodes.map((node) => [node.id, boxFromPosition(node, nodeSize(node))]),
  );
  const components = layoutConnected(nodes, edges, sizeOfNode);
  const stacked = stackComponents(components);
  return anchorBoxes(stacked, originals, mode);
}

function resolveScope(
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
  selectedNodeIds: string[] | undefined,
): Set<string> {
  const flowNodes = nodes.filter(
    (node) => !isAutoLayoutNote(node) && !isPlaceholder(node),
  );
  const selected = new Set(selectedNodeIds ?? []);
  const selectedFlow = flowNodes.filter((node) => selected.has(node.id));
  const scope =
    selectedFlow.length >= 2
      ? new Set(selectedFlow.map((node) => node.id))
      : new Set(flowNodes.map((node) => node.id));

  const expandAttachments = () => {
    let changed = true;
    while (changed) {
      changed = false;
      for (const edge of edges) {
        if (!isAttachmentEdge(edge)) continue;
        if (scope.has(edge.source) && !scope.has(edge.target)) {
          scope.add(edge.target);
          changed = true;
        }
        if (scope.has(edge.target) && !scope.has(edge.source)) {
          scope.add(edge.source);
          changed = true;
        }
      }
    }
  };

  expandAttachments();
  let changed = true;
  while (changed) {
    changed = false;
    for (const node of nodes) {
      if (!node.parentId || !scope.has(node.parentId) || scope.has(node.id)) {
        continue;
      }
      if (isAutoLayoutNote(node) || isPlaceholder(node)) continue;
      scope.add(node.id);
      changed = true;
    }
  }
  expandAttachments();
  return scope;
}

function parentDepth(id: string, byId: Map<string, CanvasLayoutNode>): number {
  let depth = 0;
  let current = byId.get(id);
  const seen = new Set<string>();
  while (current?.parentId && !seen.has(current.id)) {
    seen.add(current.id);
    depth += 1;
    current = byId.get(current.parentId);
  }
  return depth;
}

function fitFrame(boxes: Map<string, Box>): Size {
  const bounds = boundsOf(boxes.values());
  if (!bounds) {
    return { width: MIN_GROUP_WIDTH, height: MIN_GROUP_HEIGHT };
  }
  const padding = CanvasAutoLayoutSpacing.groupPadding;
  return {
    width: Math.max(
      MIN_GROUP_WIDTH,
      ceilToGrid(bounds.x + bounds.width + padding),
    ),
    height: Math.max(
      MIN_GROUP_HEIGHT,
      ceilToGrid(bounds.y + bounds.height + padding),
    ),
  };
}

function repositionNotes(
  notes: CanvasLayoutNode[],
  before: Map<string, Box>,
  after: Map<string, Box>,
): Map<string, CanvasLayoutUpdate> {
  const updates = new Map<string, CanvasLayoutUpdate>();
  for (const note of notes) {
    const noteBox = boxFromPosition(note, nodeSize(note));
    const coveredIds = [...before.entries()]
      .filter(([, box]) => containsBox(noteBox, box))
      .map(([id]) => id);
    if (coveredIds.length === 0) continue;
    const coveredAfter = coveredIds
      .map((id) => after.get(id))
      .filter((box): box is Box => !!box);
    const target = boundsOf(coveredAfter);
    if (!target) continue;
    const left = target.x + target.width / 2 - noteBox.width / 2;
    const top =
      target.y +
      target.height -
      noteBox.height +
      CanvasAutoLayoutSpacing.noteBottomPadding;
    updates.set(note.id, {
      x: snap(left + noteBox.width / 2),
      y: snap(top),
    });
  }
  return updates;
}

function changed(node: CanvasLayoutNode, update: CanvasLayoutUpdate): boolean {
  if (Math.abs(node.position.x - update.x) > 0.5) return true;
  if (Math.abs(node.position.y - update.y) > 0.5) return true;
  const size = nodeSize(node);
  if (update.width !== undefined && Math.abs(size.width - update.width) > 0.5) {
    return true;
  }
  if (
    update.height !== undefined &&
    Math.abs(size.height - update.height) > 0.5
  ) {
    return true;
  }
  return false;
}

/**
 * Returns new positions (and, for iteration/loop frames, new sizes) for the
 * nodes that should move. An empty map means the canvas is already tidy.
 * Two or more selected flow nodes are arranged on their own; otherwise the
 * whole canvas is arranged. Notes stay put unless one fully covers nodes
 * that moved, in which case it sits on their new bottom edge.
 */
export function layoutCanvasNodes(input: {
  nodes: CanvasLayoutNode[];
  edges: CanvasLayoutEdge[];
  selectedNodeIds?: string[];
}): Map<string, CanvasLayoutUpdate> {
  const { nodes, edges } = input;
  const scope = resolveScope(nodes, edges, input.selectedNodeIds);
  const updates = new Map<string, CanvasLayoutUpdate>();
  if (scope.size === 0) return updates;

  const byId = new Map(nodes.map((node) => [node.id, node]));
  const childrenByParent = new Map<string, CanvasLayoutNode[]>();
  for (const node of nodes) {
    if (!node.parentId) continue;
    const children = childrenByParent.get(node.parentId) ?? [];
    children.push(node);
    childrenByParent.set(node.parentId, children);
  }

  const frameSize = new Map<string, Size>();
  const sizeWithFrame = (node: CanvasLayoutNode): Size =>
    frameSize.get(node.id) ?? nodeSize(node);
  const parentIds = [...childrenByParent.keys()].sort(
    (a, b) => parentDepth(b, byId) - parentDepth(a, byId),
  );
  for (const parentId of parentIds) {
    const children = childrenByParent.get(parentId) ?? [];
    const flowChildren = children.filter(
      (node) => !isAutoLayoutNote(node) && !isPlaceholder(node),
    );
    const scopedChildren = flowChildren.filter((node) => scope.has(node.id));
    if (scopedChildren.length === 0) continue;
    const allScoped = flowChildren.every((node) => scope.has(node.id));
    const boxes = layoutLevel(
      scopedChildren,
      edges,
      allScoped ? 'padding' : 'anchor',
      sizeWithFrame,
    );
    for (const [id, box] of boxes) {
      const node = byId.get(id);
      if (!node) continue;
      const frame = frameSize.get(id);
      const update: CanvasLayoutUpdate = {
        ...positionFromBox(box),
        ...(frame ? { width: frame.width, height: frame.height } : {}),
      };
      if (changed(node, update)) updates.set(id, update);
    }
    if (!allScoped) continue;
    const fitted = fitFrame(boxes);
    const parent = byId.get(parentId);
    if (!parent) continue;
    frameSize.set(parentId, fitted);
    if (scope.has(parentId)) continue;
    const current = nodeSize(parent);
    const update: CanvasLayoutUpdate = {
      x: parent.position.x + (fitted.width - current.width) / 2,
      y: parent.position.y,
      width: fitted.width,
      height: fitted.height,
    };
    if (changed(parent, update)) updates.set(parentId, update);
  }

  const rootNodes = nodes.filter(
    (node) =>
      !node.parentId &&
      scope.has(node.id) &&
      !isAutoLayoutNote(node) &&
      !isPlaceholder(node),
  );
  const sizeForRoot = sizeWithFrame;
  const beforeRoot = new Map(
    rootNodes.map((node) => [node.id, boxFromPosition(node, nodeSize(node))]),
  );
  const rootBoxes = layoutLevel(rootNodes, edges, 'anchor', sizeForRoot);
  for (const [id, box] of rootBoxes) {
    const node = byId.get(id);
    if (!node) continue;
    const frame = frameSize.get(id);
    const update: CanvasLayoutUpdate = {
      ...positionFromBox(box),
      ...(frame ? { width: frame.width, height: frame.height } : {}),
    };
    if (changed(node, update)) updates.set(id, update);
  }

  const arrangingAll =
    (input.selectedNodeIds ?? []).filter((id) => {
      const node = byId.get(id);
      return node && !isAutoLayoutNote(node) && !isPlaceholder(node);
    }).length < 2;
  if (arrangingAll) {
    const notes = nodes.filter(
      (node) => isAutoLayoutNote(node) && !node.parentId,
    );
    for (const [id, update] of repositionNotes(notes, beforeRoot, rootBoxes)) {
      const node = byId.get(id);
      if (node && changed(node, update)) updates.set(id, update);
    }
  }

  return updates;
}
