import dagre from "@dagrejs/dagre";
import { layoutNodesWithElk, type ElkEdgeRouting } from "./canvas-elk-layout";
import {
  type CanvasLayoutSettings,
  defaultCanvasLayout,
} from "./canvas-edge-route";
import {
  analyzeFlow,
  separateSwitchLanes,
  straightenChains,
} from "./canvas-layout-analysis";

/**
 * Left-to-right tidy layout for the agent canvas.
 * ELK layered places nodes from a branch-aware order. Each switch output
 * then keeps its own horizontal lane. Tool links stay under the agent.
 * React Flow draws cubic curves; ELK is not asked to route edges.
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

const ATTACHMENT_SOURCE_HANDLES = new Set(["tool", "agentBottom"]);

export type CanvasLayoutNode = {
  id: string;
  type?: string;
  parentId?: string;
  position: { x: number; y: number };
  width?: number | null;
  height?: number | null;
  measured?: { width?: number | null; height?: number | null } | null;
  handles?: Array<{ id: string; type: "source" | "target"; y: number }>;
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
  targetHandle?: string | null;
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
  if (typeof value === "number" && Number.isFinite(value) && value > 0) {
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
  return node.type === "noteNode" || node.data?.label === "Note";
}

function isPlaceholder(node: CanvasLayoutNode): boolean {
  return node.type === "placeholderNode" || node.data?.label === "Placeholder";
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
  rankdir: "LR" | "RL" | "TB" | "BT";
  nodesep: number;
  ranksep: number;
  align?: "UL";
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

function runLayout(graph: DagreGraph, keepInputOrder = true): void {
  dagre.layout(
    graph,
    keepInputOrder ? { disableOptimalOrderHeuristic: true } : undefined,
  );
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
  node: { data?: CanvasLayoutNode["data"] } | undefined,
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
    if (sourceHandle === "end_cpn_ids") return conditions.length;
    const match = /^Case (\d+)$/.exec(sourceHandle);
    if (match) return Number(match[1]) - 1;
  }
  if (sourceHandle === "start") return 0;
  if (sourceHandle === "agentException") return 1;
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
  direction: "LR" | "RL" | "TB" | "BT",
  spacing: { nodesep: number; ranksep: number },
  orderOf: (id: string) => { x: number; y: number },
  byId: Map<string, CanvasLayoutNode>,
  keepInputOrder = true,
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
  runLayout(graph, keepInputOrder);
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
      "TB",
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
  layout: CanvasLayoutSettings = defaultCanvasLayout,
): Promise<Map<string, Box>[]> {
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
    .map((ids) => ids.filter((id): id is string => typeof id === "string"))
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

  return Promise.all(
    components.map(async (ids) => {
      const flowEdges = edges
        .filter((edge) => !isAttachmentEdge(edge))
        .map((edge) => ({
          id: edge.id,
          source: hiddenIds.has(edge.source)
            ? (clusterRootOf(edge.source, clusters) ?? edge.source)
            : edge.source,
          target: hiddenIds.has(edge.target)
            ? (clusterRootOf(edge.target, clusters) ?? edge.target)
            : edge.target,
          sourceHandle: edge.sourceHandle,
          targetHandle: edge.targetHandle,
        }));
      const boxes = await layoutMainFlow(
        ids,
        (id) => {
          const cluster = clusterByRoot.get(id);
          if (cluster) return { width: cluster.width, height: cluster.height };
          return sizeOfNode(byId.get(id)!);
        },
        flowEdges,
        (id) => byId.get(id)!.position,
        byId,
        layout,
      );
      return expandClusters(
        boxes,
        clusters.filter((cluster) => ids.includes(cluster.rootId)),
      );
    }),
  );
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
    rankdir: "TB",
    align: "UL",
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
  mode: "anchor" | "padding",
): Map<string, Box> {
  const after = boundsOf(placed.values());
  if (!after) return new Map();

  let dx = 0;
  let dy = 0;
  if (mode === "padding") {
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

// ELK layered, left to right. Node order comes from branch depth; each switch
// output is then pulled onto its own lane. React Flow draws the curves.
async function layoutMainFlow(
  ids: string[],
  sizeOfId: (id: string) => Size,
  edges: Array<{
    source: string;
    target: string;
    sourceHandle?: string | null;
    targetHandle?: string | null;
    id?: string;
  }>,
  _orderOf: (id: string) => { x: number; y: number },
  byId: Map<string, CanvasLayoutNode>,
  layout: CanvasLayoutSettings = defaultCanvasLayout,
): Promise<Map<string, Box>> {
  const nodes = ids
    .map((id) => byId.get(id))
    .filter((node): node is CanvasLayoutNode => !!node);
  const flowEdges = edges.filter((edge) => !isAttachmentEdge(edge));
  const horizontal = layout.direction === "LR" || layout.direction === "RL";
  const boxes =
    layout.algorithm === "dagre"
      ? layoutBoxes(
          nodes.map((node) => node.id),
          sizeOfId,
          flowEdges,
          layout.direction,
          { nodesep: layout.nodeSpacing, ranksep: layout.rankSpacing },
          (id) => byId.get(id)?.position ?? { x: 0, y: 0 },
          byId,
          false,
        )
      : await layoutNodesWithElk({
          nodes,
          edges: flowEdges,
          sizeOf: sizeOfId,
          edgeRouting: layout.route,
          direction: layout.direction,
          nodeSpacing: layout.nodeSpacing,
          rankSpacing: layout.rankSpacing,
        });
  if (layout.algorithm === "elk" && horizontal) {
    straightenChains(boxes, nodes, flowEdges);
    separateSwitchLanes(boxes, nodes, flowEdges, layout.nodeSpacing);
  }
  return boxes;
}

function shiftMembers(boxes: Map<string, Box>, ids: string[], dy: number) {
  if (Math.abs(dy) < 0.5) return;
  for (const id of ids) {
    const box = boxes.get(id);
    if (!box) continue;
    boxes.set(id, { ...box, y: box.y + dy });
  }
}

function groupBox(ids: string[], boxes: Map<string, Box>): Box | undefined {
  return boundsOf(
    ids.map((id) => boxes.get(id)).filter((box): box is Box => !!box),
  );
}

/**
 * Only boxes that actually intersect are separated. An agent moves with its
 * tools, and a switch branch moves as one row.
 */
export function relaxPlacement(
  boxes: Map<string, Box>,
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
) {
  const gap = CanvasAutoLayoutSpacing.grid * 2;
  const members = movingGroups(boxes, nodes, edges);
  const roots = [...members.keys()];
  for (let pass = 0; pass < 8; pass += 1) {
    let moved = false;
    for (const upperId of roots) {
      for (const lowerId of roots) {
        if (upperId === lowerId) continue;
        const upper = groupBox(members.get(upperId) ?? [], boxes);
        const lower = groupBox(members.get(lowerId) ?? [], boxes);
        if (!upper || !lower || upper.y > lower.y) continue;
        const overlapsX =
          upper.x < lower.x + lower.width && lower.x < upper.x + upper.width;
        if (!overlapsX) continue;
        const overlap = upper.y + upper.height - lower.y;
        if (overlap <= 0) continue;
        shiftMembers(boxes, members.get(lowerId) ?? [], overlap + gap);
        moved = true;
      }
    }
    if (!moved) break;
  }
}

function movingGroups(
  boxes: Map<string, Box>,
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
): Map<string, string[]> {
  const parent = new Map<string, string>();
  const find = (id: string): string => {
    const current = parent.get(id) ?? id;
    if (current === id) return id;
    const root = find(current);
    parent.set(id, root);
    return root;
  };
  const union = (a: string, b: string) => {
    const ra = find(a);
    const rb = find(b);
    if (ra !== rb) parent.set(rb, ra);
  };
  for (const id of boxes.keys()) parent.set(id, id);
  for (const edge of edges) {
    if (!isAttachmentEdge(edge)) continue;
    if (boxes.has(edge.source) && boxes.has(edge.target)) {
      union(edge.source, edge.target);
    }
  }
  const analysis = analyzeFlow(nodes, edges);
  const lanes = new Map<string, string>();
  for (const [id, branch] of analysis.branchOf) {
    if (!boxes.has(id)) continue;
    const key = `${branch.switchId}:${branch.index}`;
    const first = lanes.get(key);
    if (first) union(first, id);
    else lanes.set(key, id);
  }
  const members = new Map<string, string[]>();
  for (const id of boxes.keys()) {
    const root = find(id);
    const list = members.get(root) ?? [];
    list.push(id);
    members.set(root, list);
  }
  return members;
}

async function layoutLevel(
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
  mode: "anchor" | "padding",
  sizeOfNode: (node: CanvasLayoutNode) => Size = nodeSize,
  layout: CanvasLayoutSettings = defaultCanvasLayout,
): Promise<Map<string, Box>> {
  const originals = new Map(
    nodes.map((node) => [node.id, boxFromPosition(node, nodeSize(node))]),
  );
  const components = await layoutConnected(nodes, edges, sizeOfNode, layout);
  const stacked = stackComponents(components);
  relaxPlacement(stacked, nodes, edges);
  return anchorBoxes(stacked, originals, mode);
}

function resolveScope(
  nodes: CanvasLayoutNode[],
  edges: CanvasLayoutEdge[],
): Set<string> {
  const scope = new Set(
    nodes
      .filter((node) => !isAutoLayoutNote(node) && !isPlaceholder(node))
      .map((node) => node.id),
  );

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
export async function layoutCanvasNodes(input: {
  nodes: CanvasLayoutNode[];
  edges: CanvasLayoutEdge[];
  selectedNodeIds?: string[];
  edgeRouting?: ElkEdgeRouting;
  layout?: Partial<CanvasLayoutSettings>;
}): Promise<Map<string, CanvasLayoutUpdate>> {
  const { nodes, edges } = input;
  const layout: CanvasLayoutSettings = {
    ...defaultCanvasLayout,
    ...input.layout,
    route:
      input.layout?.route ?? input.edgeRouting ?? defaultCanvasLayout.route,
  };
  const scope = resolveScope(nodes, edges);
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
    const boxes = await layoutLevel(
      scopedChildren,
      edges,
      allScoped ? "padding" : "anchor",
      sizeWithFrame,
      layout,
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
  const rootBoxes = await layoutLevel(
    rootNodes,
    edges,
    "anchor",
    sizeForRoot,
    layout,
  );
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

  const notes = nodes.filter(
    (node) => isAutoLayoutNote(node) && !node.parentId,
  );
  for (const [id, update] of repositionNotes(notes, beforeRoot, rootBoxes)) {
    const node = byId.get(id);
    if (node && changed(node, update)) updates.set(id, update);
  }

  return updates;
}
