import ELK from 'elkjs/lib/elk.bundled.js';

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
  'elk.spacing.nodeNode': String(NODE_GAP),
  'elk.layered.spacing.nodeNodeBetweenLayers': String(RANK_GAP),
  'elk.layered.spacing.edgeNodeBetweenLayers': '40',
  'elk.layered.nodePlacement.strategy': 'SIMPLE',
  'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
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
    .sort((a, b) => a.y - b.y || a.id.localeCompare(b.id))
    .map((handle) => handle.id)
    .filter((id) => id !== 'tool' && id !== 'agentBottom');
  if (measured.length > 0) return [...new Set(measured)];

  const ids = new Set<string>();
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
  const children = input.nodes.map((node) => {
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
    edges: edges.map((edge, index) => ({
      id:
        edge.id ??
        `${edge.source}-${edge.target}-${edge.sourceHandle ?? index}`,
      sources: [
        edge.sourceHandle ? portKey(edge.source, edge.sourceHandle) : edge.source,
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

