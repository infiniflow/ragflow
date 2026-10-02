/**
 * Workflow semantics for the canvas layout.
 *
 * ELK layered (left to right) still places the nodes. This module tells it
 * which order to prefer, then keeps each switch output in its own horizontal
 * lane so branches do not sit on top of each other. Edge curves stay in
 * React Flow; nothing here asks ELK for an orthogonal route.
 */

const ATTACHMENT = new Set(['tool', 'agentBottom']);

type NodeLike = {
  id: string;
  position: { x: number; y: number };
  data?: {
    form?: {
      items?: Array<{ uuid?: string } | null>;
      conditions?: unknown[];
    };
  };
};

type EdgeLike = {
  source: string;
  target: string;
  sourceHandle?: string | null;
};

type Box = { x: number; y: number; width: number; height: number };

export type FlowAnalysis = {
  depth: Map<string, number>;
  branchOf: Map<string, { switchId: string; index: number }>;
  merges: Set<string>;
  /** Deepest switch first, so an inner branch is claimed before an outer one. */
  switches: string[];
};

function handleRank(node: NodeLike | undefined, sourceHandle?: string | null) {
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

function isFlowEdge(edge: EdgeLike): boolean {
  return (
    edge.source !== edge.target && !ATTACHMENT.has(edge.sourceHandle ?? '')
  );
}

export function analyzeFlow(
  nodes: NodeLike[],
  edges: EdgeLike[],
): FlowAnalysis {
  const flow = edges.filter(isFlowEdge);
  const outgoing = new Map<string, EdgeLike[]>();
  const incoming = new Map<string, EdgeLike[]>();
  for (const node of nodes) {
    outgoing.set(node.id, []);
    incoming.set(node.id, []);
  }
  for (const edge of flow) {
    outgoing.get(edge.source)?.push(edge);
    incoming.get(edge.target)?.push(edge);
  }

  const depth = new Map<string, number>();
  const visiting = new Set<string>();
  const visit = (id: string): number => {
    const known = depth.get(id);
    if (known !== undefined && !visiting.has(id)) return known;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    let best = 0;
    for (const edge of incoming.get(id) ?? []) {
      best = Math.max(best, visit(edge.source) + 1);
    }
    visiting.delete(id);
    depth.set(id, best);
    return best;
  };
  for (const node of nodes) visit(node.id);

  const switches = nodes
    .filter((node) => (outgoing.get(node.id) ?? []).length >= 2)
    .sort(
      (a, b) =>
        (depth.get(b.id) ?? 0) - (depth.get(a.id) ?? 0) ||
        a.position.y - b.position.y ||
        a.id.localeCompare(b.id),
    )
    .map((node) => node.id);

  const branchOf = new Map<string, { switchId: string; index: number }>();
  const merges = new Set<string>();
  const byId = new Map(nodes.map((node) => [node.id, node]));

  for (const switchId of switches) {
    const outs = [...(outgoing.get(switchId) ?? [])].sort((a, b) => {
      const rank =
        handleRank(byId.get(switchId), a.sourceHandle) -
        handleRank(byId.get(switchId), b.sourceHandle);
      if (rank !== 0) return rank;
      return (
        (byId.get(a.target)?.position.y ?? 0) -
        (byId.get(b.target)?.position.y ?? 0)
      );
    });
    outs.forEach((edge, index) => {
      const queue = [edge.target];
      const seen = new Set<string>();
      while (queue.length > 0) {
        const id = queue.shift();
        if (!id || seen.has(id) || id === switchId) continue;
        seen.add(id);
        const preds = incoming.get(id) ?? [];
        const owned = preds.filter((pred) => {
          if (pred.source === switchId) return true;
          const branch = branchOf.get(pred.source);
          return branch?.switchId === switchId && branch.index === index;
        });
        const claimed = branchOf.get(id);
        const crossed =
          (preds.length > 1 && owned.length !== preds.length) ||
          (claimed !== undefined &&
            (claimed.switchId !== switchId || claimed.index !== index));
        if (crossed && id !== edge.target) {
          merges.add(id);
          continue;
        }
        if (crossed) merges.add(id);
        if (claimed && claimed.switchId !== switchId) continue;
        branchOf.set(id, { switchId, index });
        for (const next of outgoing.get(id) ?? []) queue.push(next.target);
      }
    });
  }

  return { depth, branchOf, merges, switches };
}

/** Stable left-to-right order: depth, then branch index, then the user's Y. */
export function modelOrder(
  nodes: NodeLike[],
  analysis: FlowAnalysis,
): string[] {
  return [...nodes]
    .sort((a, b) => {
      const depthDelta =
        (analysis.depth.get(a.id) ?? 0) - (analysis.depth.get(b.id) ?? 0);
      if (depthDelta !== 0) return depthDelta;
      const branchA = analysis.branchOf.get(a.id)?.index ?? -1;
      const branchB = analysis.branchOf.get(b.id)?.index ?? -1;
      if (branchA !== branchB) return branchA - branchB;
      if (a.position.y !== b.position.y) return a.position.y - b.position.y;
      return a.id.localeCompare(b.id);
    })
    .map((node) => node.id);
}

function shiftBranchTree(
  rootId: string,
  dy: number,
  boxes: Map<string, Box>,
  analysis: FlowAnalysis,
  seen: Set<string>,
) {
  if (Math.abs(dy) < 0.5 || seen.has(rootId)) return;
  seen.add(rootId);
  for (const [id, branch] of analysis.branchOf) {
    if (branch.switchId !== rootId || !boxes.has(id)) continue;
    const box = boxes.get(id);
    if (!box) continue;
    boxes.set(id, { ...box, y: box.y + dy });
    shiftBranchTree(id, dy, boxes, analysis, seen);
  }
}

/**
 * Nodes that share one switch output sit on one horizontal lane.
 * Lanes follow handle order and stay a `gap` apart. Nested branches move
 * with their switch so an inner row is not torn off its parent.
 */
export function separateSwitchLanes(
  boxes: Map<string, Box>,
  nodes: NodeLike[],
  edges: EdgeLike[],
  gap: number,
): void {
  const analysis = analyzeFlow(nodes, edges);
  const bySwitch = new Map<string, Map<number, string[]>>();
  for (const [id, branch] of analysis.branchOf) {
    if (!boxes.has(id)) continue;
    const lanes = bySwitch.get(branch.switchId) ?? new Map();
    const members = lanes.get(branch.index) ?? [];
    members.push(id);
    lanes.set(branch.index, members);
    bySwitch.set(branch.switchId, lanes);
  }

  for (const switchId of analysis.switches) {
    const lanes = bySwitch.get(switchId);
    if (!lanes || lanes.size < 2) continue;
    const indexes = [...lanes.keys()].sort((a, b) => a - b);
    let cursor = Math.min(
      ...indexes.flatMap((index) =>
        (lanes.get(index) ?? []).map((id) => boxes.get(id)?.y ?? 0),
      ),
    );
    for (const index of indexes) {
      const ids = lanes.get(index) ?? [];
      const height = Math.max(
        ...ids.map((id) => boxes.get(id)?.height ?? 0),
        1,
      );
      for (const id of ids) {
        const box = boxes.get(id);
        if (!box) continue;
        const y = cursor + (height - box.height) / 2;
        const dy = y - box.y;
        boxes.set(id, { ...box, y });
        shiftBranchTree(id, dy, boxes, analysis, new Set([id]));
      }
      cursor += height + gap;
    }
    const switchBox = boxes.get(switchId);
    const first = boxes.get((lanes.get(indexes[0]) ?? [])[0] ?? '');
    const last = boxes.get(
      (lanes.get(indexes[indexes.length - 1]) ?? [])[0] ?? '',
    );
    if (!switchBox || !first || !last) continue;
    const mid = (first.y + first.height / 2 + last.y + last.height / 2) / 2;
    boxes.set(switchId, { ...switchBox, y: mid - switchBox.height / 2 });
  }

  placeMerges(boxes, edges, analysis);
}

function placeMerges(
  boxes: Map<string, Box>,
  edges: EdgeLike[],
  analysis: FlowAnalysis,
) {
  for (const id of analysis.merges) {
    const box = boxes.get(id);
    if (!box) continue;
    const preds = edges.filter(
      (edge) =>
        edge.target === id && isFlowEdge(edge) && boxes.has(edge.source),
    );
    if (preds.length < 2) continue;
    const mid =
      preds.reduce((sum, edge) => {
        const pred = boxes.get(edge.source)!;
        return sum + pred.y + pred.height / 2;
      }, 0) / preds.length;
    const y = mid - box.height / 2;
    const dy = y - box.y;
    boxes.set(id, { ...box, y });
    shiftBranchTree(id, dy, boxes, analysis, new Set([id]));
  }
}

/** A single-successor chain stays on one row. Fan-out is left to the lanes. */
export function straightenChains(
  boxes: Map<string, Box>,
  nodes: NodeLike[],
  edges: EdgeLike[],
): void {
  const flow = edges.filter(isFlowEdge);
  const outgoing = new Map<string, EdgeLike[]>();
  const incoming = new Map<string, EdgeLike[]>();
  for (const id of boxes.keys()) {
    outgoing.set(id, []);
    incoming.set(id, []);
  }
  for (const edge of flow) {
    outgoing.get(edge.source)?.push(edge);
    incoming.get(edge.target)?.push(edge);
  }
  const analysis = analyzeFlow(nodes, edges);
  const ordered = [...boxes.keys()].sort(
    (a, b) => (analysis.depth.get(a) ?? 0) - (analysis.depth.get(b) ?? 0),
  );
  for (const id of ordered) {
    const preds = incoming.get(id) ?? [];
    if (preds.length !== 1) continue;
    const predId = preds[0].source;
    if ((outgoing.get(predId) ?? []).length !== 1) continue;
    const pred = boxes.get(predId);
    const box = boxes.get(id);
    if (!pred || !box) continue;
    const y = pred.y + pred.height / 2 - box.height / 2;
    boxes.set(id, { ...box, y });
  }
}

export type LayoutQuality = {
  nodeOverlaps: number;
  edgeCrossings: number;
  edgeNodeHits: number;
  backwardEdges: number;
};

function center(box: Box, side: 'left' | 'right') {
  return {
    x: side === 'right' ? box.x + box.width : box.x,
    y: box.y + box.height / 2,
  };
}

function segmentsCross(
  a1: { x: number; y: number },
  a2: { x: number; y: number },
  b1: { x: number; y: number },
  b2: { x: number; y: number },
): boolean {
  const direction = (
    p: { x: number; y: number },
    q: { x: number; y: number },
    r: { x: number; y: number },
  ) => (q.x - p.x) * (r.y - p.y) - (q.y - p.y) * (r.x - p.x);
  const d1 = direction(a1, a2, b1);
  const d2 = direction(a1, a2, b2);
  const d3 = direction(b1, b2, a1);
  const d4 = direction(b1, b2, a2);
  return d1 * d2 < 0 && d3 * d4 < 0;
}

function bezierPoint(
  source: { x: number; y: number },
  target: { x: number; y: number },
  t: number,
) {
  const dx = Math.abs(target.x - source.x);
  const c1x = source.x + dx * 0.25;
  const c2x = target.x - dx * 0.25;
  const u = 1 - t;
  return {
    x:
      u * u * u * source.x +
      3 * u * u * t * c1x +
      3 * u * t * t * c2x +
      t * t * t * target.x,
    y:
      u * u * u * source.y +
      3 * u * u * t * source.y +
      3 * u * t * t * target.y +
      t * t * t * target.y,
  };
}

function pointInBox(point: { x: number; y: number }, box: Box, inset: number) {
  return (
    point.x > box.x + inset &&
    point.x < box.x + box.width - inset &&
    point.y > box.y + inset &&
    point.y < box.y + box.height - inset
  );
}

export function measureLayoutQuality(
  boxes: Map<string, Box>,
  edges: EdgeLike[],
): LayoutQuality {
  const ids = [...boxes.keys()];
  let nodeOverlaps = 0;
  for (let i = 0; i < ids.length; i += 1) {
    for (let j = i + 1; j < ids.length; j += 1) {
      const a = boxes.get(ids[i]);
      const b = boxes.get(ids[j]);
      if (!a || !b) continue;
      const overlaps =
        a.x < b.x + b.width &&
        b.x < a.x + a.width &&
        a.y < b.y + b.height &&
        b.y < a.y + a.height;
      if (overlaps) nodeOverlaps += 1;
    }
  }

  const flow = edges.filter(
    (edge) =>
      isFlowEdge(edge) && boxes.has(edge.source) && boxes.has(edge.target),
  );
  let edgeCrossings = 0;
  let edgeNodeHits = 0;
  for (let i = 0; i < flow.length; i += 1) {
    const first = flow[i];
    const a = boxes.get(first.source);
    const b = boxes.get(first.target);
    if (!a || !b) continue;
    const from = center(a, 'right');
    const to = center(b, 'left');
    for (const id of ids) {
      if (id === first.source || id === first.target) continue;
      const box = boxes.get(id);
      if (!box) continue;
      for (const t of [0.35, 0.5, 0.65]) {
        if (pointInBox(bezierPoint(from, to, t), box, 4)) {
          edgeNodeHits += 1;
          break;
        }
      }
    }
    for (let j = i + 1; j < flow.length; j += 1) {
      const second = flow[j];
      if (
        first.source === second.source ||
        first.source === second.target ||
        first.target === second.source ||
        first.target === second.target
      ) {
        continue;
      }
      const c = boxes.get(second.source);
      const d = boxes.get(second.target);
      if (!c || !d) continue;
      if (segmentsCross(from, to, center(c, 'right'), center(d, 'left'))) {
        edgeCrossings += 1;
      }
    }
  }

  let backwardEdges = 0;
  for (const edge of flow) {
    const source = boxes.get(edge.source);
    const target = boxes.get(edge.target);
    if (source && target && target.x + target.width <= source.x + 1) {
      backwardEdges += 1;
    }
  }

  return { nodeOverlaps, edgeCrossings, edgeNodeHits, backwardEdges };
}
