import { create } from 'zustand';

export type CanvasEdgeRouting = 'bezier' | 'orthogonal';

const storageKey = 'ragflow-canvas-edge-route';

function readRoute(): CanvasEdgeRouting {
  if (typeof window === 'undefined') return 'bezier';
  try {
    return window.localStorage.getItem(storageKey) === 'orthogonal'
      ? 'orthogonal'
      : 'bezier';
  } catch {
    return 'bezier';
  }
}

type EdgeRouteState = {
  route: CanvasEdgeRouting;
  setRoute: (route: CanvasEdgeRouting) => void;
};

export const useCanvasEdgeRoute = create<EdgeRouteState>((set) => ({
  route: readRoute(),
  setRoute: (route) => {
    try {
      window.localStorage.setItem(storageKey, route);
    } catch {
      // The canvas still switches for this session when storage is blocked.
    }
    set({ route });
  },
}));

export function orthogonalLanes(
  edges: Array<{
    id: string;
    sourceX: number;
    sourceY: number;
    targetX: number;
    targetY: number;
  }>,
): Map<string, { step: number; lane: number; laneCount: number }> {
  const groups = new Map<string, typeof edges>();
  for (const edge of edges) {
    const key = `${Math.round(edge.sourceX / 36)}:${Math.round(edge.targetX / 64)}`;
    const list = groups.get(key) ?? [];
    list.push(edge);
    groups.set(key, list);
  }
  const placed = new Map<
    string,
    { step: number; lane: number; laneCount: number }
  >();
  for (const list of groups.values()) {
    list.sort((a, b) => a.sourceY - b.sourceY || a.id.localeCompare(b.id));
    list.forEach((edge, index) => {
      const span = Math.max(list.length - 1, 1);
      const step = list.length === 1 ? 0.62 : 0.2 + (0.6 * index) / span;
      placed.set(edge.id, { step, lane: index, laneCount: list.length });
    });
  }
  return placed;
}

type Point = { x: number; y: number };

function bend(a: Point, b: Point, c: Point, radius: number): string {
  const bendSize = Math.min(distance(a, b) / 2, distance(b, c) / 2, radius);
  if ((a.x === b.x && b.x === c.x) || (a.y === b.y && b.y === c.y)) {
    return `L${b.x},${b.y}`;
  }
  if (a.y === b.y) {
    const xDir = a.x < c.x ? -1 : 1;
    const yDir = a.y < c.y ? 1 : -1;
    return `L${b.x + bendSize * xDir},${b.y}Q${b.x},${b.y} ${b.x},${b.y + bendSize * yDir}`;
  }
  const xDir = a.x < c.x ? 1 : -1;
  const yDir = a.y < c.y ? -1 : 1;
  return `L${b.x},${b.y + bendSize * yDir}Q${b.x},${b.y} ${b.x + bendSize * xDir},${b.y}`;
}

function distance(a: Point, b: Point): number {
  return Math.hypot(b.x - a.x, b.y - a.y);
}

function trace(points: Point[], radius: number) {
  let path = `M${points[0].x},${points[0].y}`;
  for (let index = 1; index < points.length - 1; index += 1) {
    path += bend(points[index - 1], points[index], points[index + 1], radius);
  }
  const last = points[points.length - 1];
  path += `L${last.x},${last.y}`;
  const label = points[Math.floor(points.length / 2)];
  return { path, labelX: label.x, labelY: label.y };
}

/** Orthogonal path whose elbow is staggered so parallel edges stay apart. */
export function describeOrthogonalEdge(
  source: Point,
  target: Point,
  step: number,
  lane: number,
  laneCount: number,
): { path: string; labelX: number; labelY: number } {
  const direction = target.x >= source.x ? 1 : -1;
  const stub = 28;
  const x1 = source.x + direction * stub;
  const x2 = target.x - direction * stub;
  const low = Math.min(x1, x2);
  const high = Math.max(x1, x2);
  const bendX = Math.min(high, Math.max(low, x1 + (x2 - x1) * step));
  const sameRow = Math.abs(source.y - target.y) < 16;
  const shift = sameRow ? (lane - (laneCount - 1) / 2) * 18 : 0;
  if (sameRow && Math.abs(shift) < 0.5) {
    return {
      path: `M${source.x},${source.y}L${target.x},${target.y}`,
      labelX: (source.x + target.x) / 2,
      labelY: source.y,
    };
  }
  const points = sameRow
    ? [
        source,
        { x: x1, y: source.y },
        { x: x1, y: source.y + shift },
        { x: x2, y: source.y + shift },
        { x: x2, y: target.y },
        target,
      ]
    : [source, { x: bendX, y: source.y }, { x: bendX, y: target.y }, target];
  return trace(points, 10);
}
