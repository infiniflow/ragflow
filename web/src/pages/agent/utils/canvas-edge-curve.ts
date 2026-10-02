/**
 * Cubic bezier used by the canvas edges (same control points as React Flow's
 * default bezier). When that curve would cut through another node, the
 * control points shift up or down so the edge bows around it. The path stays
 * a curve; it is not an orthogonal polyline.
 */

type Point = { x: number; y: number };
type Box = { x: number; y: number; width: number; height: number };

const MAX_OFFSET = 180;

function cubic(t: number, p0: number, p1: number, p2: number, p3: number) {
  const u = 1 - t;
  return (
    u * u * u * p0 + 3 * u * u * t * p1 + 3 * u * t * t * p2 + t * t * t * p3
  );
}

function pointAt(
  source: Point,
  target: Point,
  offset: number,
  t: number,
): Point {
  const dx = Math.abs(target.x - source.x);
  return {
    x: cubic(t, source.x, source.x + dx * 0.25, target.x - dx * 0.25, target.x),
    y: cubic(t, source.y, source.y + offset, target.y + offset, target.y),
  };
}

function hits(point: Point, box: Box) {
  const inset = 6;
  return (
    point.x > box.x + inset &&
    point.x < box.x + box.width - inset &&
    point.y > box.y + inset &&
    point.y < box.y + box.height - inset
  );
}

function hitCount(source: Point, target: Point, offset: number, boxes: Box[]) {
  let count = 0;
  for (const t of [0.25, 0.4, 0.5, 0.6, 0.75]) {
    const point = pointAt(source, target, offset, t);
    if (boxes.some((box) => hits(point, box))) count += 1;
  }
  return count;
}

function blockingBoxes(source: Point, target: Point, obstacles: Box[]) {
  const minX = Math.min(source.x, target.x);
  const maxX = Math.max(source.x, target.x);
  return obstacles.filter((box) => {
    if (box.width < 8 || box.height < 8) return false;
    return box.x < maxX && box.x + box.width > minX;
  });
}

/** Vertical shift for both bezier handles. Zero keeps the plain React Flow curve. */
export function bezierDetourOffset(
  source: Point,
  target: Point,
  obstacles: Box[],
): number {
  if (Math.abs(target.x - source.x) < 48) return 0;
  const blocking = blockingBoxes(source, target, obstacles);
  const straight = hitCount(source, target, 0, blocking);
  if (blocking.length === 0 || straight === 0) return 0;

  let best = 0;
  let bestHits = straight;
  let bestAbs = Number.POSITIVE_INFINITY;
  for (let offset = -MAX_OFFSET; offset <= MAX_OFFSET; offset += 24) {
    if (offset === 0) continue;
    const count = hitCount(source, target, offset, blocking);
    const magnitude = Math.abs(offset);
    if (count < bestHits || (count === bestHits && magnitude < bestAbs)) {
      best = offset;
      bestHits = count;
      bestAbs = magnitude;
    }
  }
  return best;
}

export function curveClearsNodes(
  source: Point,
  target: Point,
  obstacles: Box[],
): boolean {
  const blocking = blockingBoxes(source, target, obstacles);
  const offset = bezierDetourOffset(source, target, obstacles);
  return hitCount(source, target, offset, blocking) === 0;
}

export function describeDetourBezier(
  source: Point,
  target: Point,
  offset: number,
): { path: string; labelX: number; labelY: number } {
  const dx = Math.abs(target.x - source.x);
  const c1x = source.x + dx * 0.25;
  const c2x = target.x - dx * 0.25;
  const c1y = source.y + offset;
  const c2y = target.y + offset;
  const label = pointAt(source, target, offset, 0.5);
  return {
    path: `M${source.x},${source.y} C${c1x},${c1y} ${c2x},${c2y} ${target.x},${target.y}`,
    labelX: label.x,
    labelY: label.y,
  };
}
