import ELK from 'elkjs/lib/elk.bundled.js';
import { analyzeFlow, modelOrder } from './canvas-layout-analysis';

/**
 * React Flow's ELK multiple-handle layout, adapted to this canvas:
 * https://reactflow.dev/examples/layout/elkjs-multiple-handles
 *
 * Each handle is an ELK port with a fixed side and top-to-bottom index, so a
 * node with several outputs keeps that order. ELK returns a top-left box.
 * Tool and sub-agent links stay out of this graph; the caller places them
 * under the agent.
 */

const elk = new ELK();

const NODE_GAP = 16 * 6;
const RANK_GAP = 16 * 8;

export type ElkLayoutNode = {
  id: string;
  data?: {
    form?: {
      items?: Array<{ uuid?: string } | null>;
      conditions?: unknown[];
    };
  };
  handles?: Array<{ id: string; type: 'source' | 'target'; y: number }>;
};

export type ElkLayoutEdge = {
  id?: string;
  source: string;
  target: string;
  sourceHandle?: string | null;
  targetHandle?: string | null;
};

type Size = { width: number; height: number };
type Box = { x: number; y: number; width: number; height: number };

const layoutOptions: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.layered.layering.strategy': 'LONGEST_PATH',
  'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
  'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
  'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
  'elk.spacing.nodeNode': String(NODE_GAP),
  'elk.layered.spacing.nodeNodeBetweenLayers': String(RANK_GAP),
  'elk.layered.spacing.edgeNodeBetweenLayers': '40',
};

function handleRank(
  node: ElkLayoutNode | undefined,
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
  if (sourceHandle === 'tool' || sourceHandle === 'agentBottom') return 1000;
  return 0;
}

function portKey(nodeId: string, handleId: string): string {
  return `${nodeId}::${handleId}`;
}

function orderedHandleIds(
  node: ElkLayoutNode,
  edges: ElkLayoutEdge[],
  role: 'source' | 'target',
): string[] {
  const measured = (node.handles ?? [])
    .filter((handle) => handle.type === role)
    .filter((handle) => handle.id !== 'tool' && handle.id !== 'agentBottom')
    .sort((a, b) => a.y - b.y || a.id.localeCompare(b.id));
  const yOf = new Map(measured.map((handle) => [handle.id, handle.y]));
  const ids = new Set(measured.map((handle) => handle.id));
  for (const edge of edges) {
    if (role === 'source' && edge.source === node.id && edge.sourceHandle) {
      if (edge.sourceHandle !== 'tool' && edge.sourceHandle !== 'agentBottom') {
        ids.add(edge.sourceHandle);
      }
    }
    if (role === 'target' && edge.target === node.id) {
      ids.add(edge.targetHandle || 'end');
    }
  }
  return [...ids].sort((a, b) => {
    const ay = yOf.get(a);
    const by = yOf.get(b);
    if (ay !== undefined && by !== undefined && ay !== by) return ay - by;
    if (ay !== undefined && by === undefined) return -1;
    if (ay === undefined && by !== undefined) return 1;
    if (role === 'source') {
      const rank = handleRank(node, a) - handleRank(node, b);
      if (rank !== 0) return rank;
    }
    return a.localeCompare(b);
  });
}

export async function layoutNodesWithElk(input: {
  nodes: ElkLayoutNode[];
  edges: ElkLayoutEdge[];
  sizeOf: (id: string) => Size;
}): Promise<Map<string, Box>> {
  const placed = new Map<string, Box>();
  if (input.nodes.length === 0) return placed;

  const idSet = new Set(input.nodes.map((node) => node.id));
  const edges = input.edges.filter(
    (edge) =>
      edge.source !== edge.target &&
      idSet.has(edge.source) &&
      idSet.has(edge.target),
  );
  const analysis = analyzeFlow(input.nodes, edges);
  const rank = new Map(
    modelOrder(input.nodes, analysis).map((id, index) => [id, index]),
  );
  const orderedNodes = [...input.nodes].sort(
    (a, b) => (rank.get(a.id) ?? 0) - (rank.get(b.id) ?? 0),
  );
  const children = orderedNodes.map((node) => {
    const size = input.sizeOf(node.id);
    const targets = orderedHandleIds(node, edges, 'target');
    const sources = orderedHandleIds(node, edges, 'source');
    const ports = [
      ...targets.map((id, index) => ({
        id: portKey(node.id, id),
        layoutOptions: {
          'elk.port.side': 'WEST',
          'elk.port.index': String(index),
        },
      })),
      ...sources.map((id, index) => ({
        id: portKey(node.id, id),
        layoutOptions: {
          'elk.port.side': 'EAST',
          'elk.port.index': String(index),
        },
      })),
    ];
    if (ports.length === 0) {
      ports.push({
        id: node.id,
        layoutOptions: { 'elk.port.side': 'WEST', 'elk.port.index': '0' },
      });
    }
    return {
      id: node.id,
      width: size.width,
      height: size.height,
      layoutOptions: {
        'org.eclipse.elk.portConstraints': 'FIXED_ORDER',
      },
      ports,
    };
  });

  const graph = {
    id: 'root',
    layoutOptions,
    children,
    edges: [...edges]
      .sort((a, b) => {
        const source = (rank.get(a.source) ?? 0) - (rank.get(b.source) ?? 0);
        if (source !== 0) return source;
        return (
          handleRank(
            orderedNodes.find((node) => node.id === a.source),
            a.sourceHandle,
          ) -
          handleRank(
            orderedNodes.find((node) => node.id === b.source),
            b.sourceHandle,
          )
        );
      })
      .map((edge, index) => ({
        id:
          edge.id ??
          `${edge.source}-${edge.target}-${edge.sourceHandle ?? index}`,
        sources: [
          edge.sourceHandle
            ? portKey(edge.source, edge.sourceHandle)
            : edge.source,
        ],
        targets: [
          edge.targetHandle
            ? portKey(edge.target, edge.targetHandle)
            : portKey(edge.target, 'end'),
        ],
      })),
  };

  const layouted = await elk.layout(graph);
  for (const child of layouted.children ?? []) {
    if (child.x == null || child.y == null) continue;
    const size = input.sizeOf(child.id);
    placed.set(child.id, {
      x: child.x,
      y: child.y,
      width: child.width ?? size.width,
      height: child.height ?? size.height,
    });
  }
  return placed;
}
