import {
  CanvasAutoLayoutSpacing,
  CanvasLayoutEdge,
  CanvasLayoutNode,
  layoutCanvasNodes,
} from '../canvas-auto-layout';

const WIDTH = 200;
const HEIGHT = 80;

function node(
  id: string,
  x: number,
  y: number,
  extra: Partial<CanvasLayoutNode> = {},
): CanvasLayoutNode {
  return {
    id,
    type: 'ragNode',
    position: { x, y },
    width: WIDTH,
    height: HEIGHT,
    measured: { width: WIDTH, height: HEIGHT },
    data: { label: id },
    ...extra,
  };
}

function edge(
  source: string,
  target: string,
  sourceHandle = 'start',
): CanvasLayoutEdge {
  return { source, target, sourceHandle };
}

function placed(
  nodes: CanvasLayoutNode[],
  updates = layoutCanvasNodes({ nodes, edges: [] }),
) {
  return nodes.map((item) => {
    const update = updates.get(item.id);
    if (!update) return item;
    return {
      ...item,
      position: { x: update.x, y: update.y },
      width: update.width ?? item.width,
      height: update.height ?? item.height,
      measured: {
        width: update.width ?? item.measured?.width,
        height: update.height ?? item.measured?.height,
      },
    };
  });
}

function leftOf(item: CanvasLayoutNode): number {
  return item.position.x - (item.measured?.width ?? WIDTH) / 2;
}

function rightOf(item: CanvasLayoutNode): number {
  return item.position.x + (item.measured?.width ?? WIDTH) / 2;
}

function centerY(item: CanvasLayoutNode): number {
  return item.position.y + (item.measured?.height ?? HEIGHT) / 2;
}

describe('layoutCanvasNodes', () => {
  it('places a chain left to right on one row with an even gap', () => {
    const nodes = [node('a', 40, 900), node('b', 800, 40), node('c', 20, 500)];
    const edges = [edge('a', 'b'), edge('b', 'c')];
    const updates = layoutCanvasNodes({ nodes, edges });
    const next = placed(nodes, updates);
    const [a, b, c] = ['a', 'b', 'c'].map((id) =>
      next.find((item) => item.id === id)!,
    );

    expect(a.position.x).toBeLessThan(b.position.x);
    expect(b.position.x).toBeLessThan(c.position.x);
    expect(a.position.y).toBe(b.position.y);
    expect(b.position.y).toBe(c.position.y);

    const gap = leftOf(b) - rightOf(a);
    expect(gap).toBeGreaterThanOrEqual(
      CanvasAutoLayoutSpacing.rankGap - CanvasAutoLayoutSpacing.grid,
    );
    expect(gap).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.rankGap + CanvasAutoLayoutSpacing.grid,
    );

    const originalLeft = Math.min(...nodes.map(leftOf));
    expect(Math.abs(leftOf(a) - originalLeft)).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid,
    );
  });

  it('stacks branches and centers them on the parent', () => {
    const nodes = [
      node('a', 100, 100),
      node('b', 400, 40),
      node('c', 420, 400),
    ];
    const edges = [edge('a', 'b'), edge('a', 'c')];
    const next = placed(nodes, layoutCanvasNodes({ nodes, edges }));
    const a = next.find((item) => item.id === 'a')!;
    const b = next.find((item) => item.id === 'b')!;
    const c = next.find((item) => item.id === 'c')!;
    const upper = b.position.y < c.position.y ? b : c;
    const lower = upper === b ? c : b;

    expect(Math.abs(b.position.x - c.position.x)).toBeLessThanOrEqual(1);
    expect(b.position.x).toBeGreaterThan(a.position.x);
    const verticalGap = lower.position.y - (upper.position.y + HEIGHT);
    expect(verticalGap).toBeGreaterThanOrEqual(
      CanvasAutoLayoutSpacing.nodeGap - CanvasAutoLayoutSpacing.grid,
    );
    expect(verticalGap).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.nodeGap + CanvasAutoLayoutSpacing.grid,
    );
    const mid = (centerY(b) + centerY(c)) / 2;
    expect(Math.abs(mid - centerY(a))).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid,
    );
  });

  it('keeps fan-out in top-to-bottom handle order so edges do not cross', () => {
    const nodes = [
      node('src', 0, 400, {
        data: {
          label: 'Switch',
          form: { conditions: [{}, {}, {}] },
        },
      }),
      node('top', 500, 900),
      node('upper', 500, 600),
      node('lower', 500, 200),
      node('bottom', 500, 0),
    ];
    const edges = [
      edge('src', 'top', 'Case 1'),
      edge('src', 'upper', 'Case 2'),
      edge('src', 'lower', 'Case 3'),
      edge('src', 'bottom', 'end_cpn_ids'),
    ];
    const next = placed(nodes, layoutCanvasNodes({ nodes, edges }));
    const tops = ['top', 'upper', 'lower', 'bottom'].map(
      (id) => next.find((item) => item.id === id)!.position.y,
    );
    expect(tops[0]).toBeLessThan(tops[1]);
    expect(tops[1]).toBeLessThan(tops[2]);
    expect(tops[2]).toBeLessThan(tops[3]);
  });

  it('splits two outputs of one node onto separate rows', () => {
    const nodes = [
      node('src', 0, 200),
      node('upper', 500, 200),
      node('lower', 520, 200),
    ];
    const edges = [
      edge('src', 'upper', 'Case 1'),
      edge('src', 'lower', 'Case 2'),
    ];
    const next = placed(
      nodes,
      layoutCanvasNodes({
        nodes: nodes.map((item) =>
          item.id === 'src'
            ? {
                ...item,
                data: { label: 'Switch', form: { conditions: [{}, {}] } },
              }
            : item,
        ),
        edges,
      }),
    );
    const upper = next.find((item) => item.id === 'upper')!;
    const lower = next.find((item) => item.id === 'lower')!;
    const source = next.find((item) => item.id === 'src')!;

    expect(upper.position.y).toBeLessThan(lower.position.y);
    expect(
      lower.position.y - (upper.position.y + HEIGHT),
    ).toBeGreaterThanOrEqual(
      CanvasAutoLayoutSpacing.nodeGap - CanvasAutoLayoutSpacing.grid,
    );
    const mid = (centerY(upper) + centerY(lower)) / 2;
    expect(Math.abs(mid - centerY(source))).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid * 2,
    );
  });

  it('does not shift a shared merge twice when outputs are separated', () => {
    const nodes = [
      node('src', 0, 80, {
        data: { label: 'Switch', form: { conditions: [{}, {}] } },
      }),
      node('high', 400, 80, {
        height: 40,
        measured: { width: WIDTH, height: 40 },
      }),
      node('low', 400, 80, {
        height: 160,
        measured: { width: WIDTH, height: 160 },
      }),
      node('merge', 900, 80),
    ];
    const edges = [
      edge('src', 'high', 'Case 1'),
      edge('src', 'low', 'Case 2'),
      edge('high', 'merge'),
      edge('low', 'merge'),
    ];
    const next = placed(nodes, layoutCanvasNodes({ nodes, edges }));
    const source = next.find((item) => item.id === 'src')!;
    const high = next.find((item) => item.id === 'high')!;
    const low = next.find((item) => item.id === 'low')!;
    const merge = next.find((item) => item.id === 'merge')!;

    expect(high.position.y).toBeLessThan(low.position.y);
    const mid = (centerY(high) + centerY(low)) / 2;
    expect(Math.abs(mid - centerY(source))).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid * 2,
    );
    const top = Math.min(high.position.y, low.position.y);
    const bottom = Math.max(
      high.position.y + (high.measured?.height ?? HEIGHT),
      low.position.y + (low.measured?.height ?? HEIGHT),
    );
    expect(merge.position.y).toBeGreaterThanOrEqual(
      top - CanvasAutoLayoutSpacing.nodeGap,
    );
    expect(merge.position.y + HEIGHT).toBeLessThanOrEqual(
      bottom + CanvasAutoLayoutSpacing.nodeGap,
    );
  });

  it('keeps each switch branch in its own band so later nodes do not cross', () => {
    const nodes = [
      node('src', 0, 200, {
        data: { label: 'Switch', form: { conditions: [{}, {}] } },
      }),
      node('a', 400, 800),
      node('a2', 700, 900),
      node('b', 420, 100),
      node('b2', 680, 0),
      node('c', 410, 400),
      node('c2', 690, 500),
    ];
    const edges = [
      edge('src', 'a', 'Case 1'),
      edge('a', 'a2'),
      edge('src', 'b', 'Case 2'),
      edge('b', 'b2'),
      edge('src', 'c', 'end_cpn_ids'),
      edge('c', 'c2'),
    ];
    const next = placed(nodes, layoutCanvasNodes({ nodes, edges }));
    const y = (id: string) => next.find((item) => item.id === id)!.position.y;

    expect(y('a')).toBeLessThan(y('b'));
    expect(y('b')).toBeLessThan(y('c'));
    expect(y('a2')).toBeLessThan(y('b2'));
    expect(y('b2')).toBeLessThan(y('c2'));
    expect(y('a2')).toBeLessThan(y('b'));
    expect(y('b2')).toBeLessThan(y('c'));
  });

  it('puts a merge node after both branches', () => {
    const nodes = [node('a', 0, 0), node('b', 10, 200), node('c', 500, 80)];
    const edges = [edge('a', 'c'), edge('b', 'c')];
    const next = placed(nodes, layoutCanvasNodes({ nodes, edges }));
    const a = next.find((item) => item.id === 'a')!;
    const b = next.find((item) => item.id === 'b')!;
    const c = next.find((item) => item.id === 'c')!;

    expect(c.position.x).toBeGreaterThan(a.position.x);
    expect(c.position.x).toBeGreaterThan(b.position.x);
    expect(Math.abs(a.position.x - b.position.x)).toBeLessThanOrEqual(1);
  });

  it('keeps tools under the agent and the next step to the right', () => {
    const nodes = [
      node('agent', 300, 200),
      node('tool', 900, 20, { data: { label: 'Tool' } }),
      node('next', 100, 800),
    ];
    const edges = [edge('agent', 'tool', 'tool'), edge('agent', 'next')];
    const nextNodes = placed(nodes, layoutCanvasNodes({ nodes, edges }));
    const agent = nextNodes.find((item) => item.id === 'agent')!;
    const tool = nextNodes.find((item) => item.id === 'tool')!;
    const step = nextNodes.find((item) => item.id === 'next')!;

    expect(tool.position.y).toBeGreaterThanOrEqual(agent.position.y + HEIGHT);
    expect(Math.abs(tool.position.x - agent.position.x)).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid,
    );
    expect(step.position.x).toBeGreaterThan(agent.position.x);
    expect(Math.abs(centerY(step) - centerY(agent))).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid,
    );
  });

  it('stacks disconnected flows without overlapping them', () => {
    const nodes = [node('a', 1000, 100), node('b', 1400, 100)];
    const next = placed(nodes, layoutCanvasNodes({ nodes, edges: [] }));
    const a = next.find((item) => item.id === 'a')!;
    const b = next.find((item) => item.id === 'b')!;
    const upper = a.position.y <= b.position.y ? a : b;
    const lower = upper === a ? b : a;

    expect(lower.position.y).toBeGreaterThanOrEqual(upper.position.y + HEIGHT);
    expect(Math.abs(leftOf(a) - leftOf(b))).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid,
    );
  });

  it('leaves notes that cover nothing and reseats a note that covers a node', () => {
    const flow = node('a', 500, 200);
    const covering = node('note', 500, 150, {
      type: 'noteNode',
      data: { label: 'Note' },
      width: 400,
      height: 300,
      measured: { width: 400, height: 300 },
    });
    const loose = node('loose', 2000, 2000, {
      type: 'noteNode',
      data: { label: 'Note' },
      width: 220,
      height: 140,
      measured: { width: 220, height: 140 },
    });
    const updates = layoutCanvasNodes({
      nodes: [flow, covering, loose],
      edges: [],
    });

    expect(updates.has('loose')).toBe(false);
    expect(updates.has('note')).toBe(true);
    const noteTop = updates.get('note')!.y;
    const flowTop = updates.get('a')?.y ?? flow.position.y;
    const flowBottom = flowTop + HEIGHT;
    const noteBottom = noteTop + 300;
    expect(noteBottom).toBeGreaterThan(flowBottom);
    expect(
      Math.abs(
        noteBottom - (flowBottom + CanvasAutoLayoutSpacing.noteBottomPadding),
      ),
    ).toBeLessThanOrEqual(CanvasAutoLayoutSpacing.grid);
    expect(
      Math.abs((updates.get('note')!.x ?? 0) - flow.position.x),
    ).toBeLessThanOrEqual(CanvasAutoLayoutSpacing.grid);
  });

  it('does not move placeholder nodes', () => {
    const nodes = [
      node('a', 10, 10),
      node('b', 400, 300),
      node('ph', 50, 50, {
        type: 'placeholderNode',
        data: { label: 'Placeholder' },
      }),
    ];
    const updates = layoutCanvasNodes({
      nodes,
      edges: [edge('a', 'b')],
    });
    expect(updates.has('ph')).toBe(false);
  });

  it('arranges only the selection when two or more flow nodes are selected', () => {
    const nodes = [node('a', 10, 10), node('b', 600, 400), node('c', 50, 700)];
    const updates = layoutCanvasNodes({
      nodes,
      edges: [edge('a', 'b'), edge('b', 'c')],
      selectedNodeIds: ['a', 'b'],
    });
    expect(updates.has('c')).toBe(false);
    expect(updates.has('a') || updates.has('b')).toBe(true);
    const next = placed(nodes, updates);
    const a = next.find((item) => item.id === 'a')!;
    const b = next.find((item) => item.id === 'b')!;
    expect(a.position.x).toBeLessThan(b.position.x);
    expect(next.find((item) => item.id === 'c')!.position).toEqual({
      x: 50,
      y: 700,
    });
  });

  it('lays child nodes out inside a group and resizes the frame', () => {
    const group = node('group', 400, 120, {
      type: 'iterationNode',
      data: { label: 'Iteration' },
      width: 500,
      height: 250,
      measured: { width: 500, height: 250 },
    });
    const start = node('start', 80, 200, {
      parentId: 'group',
      width: 80,
      height: 40,
      measured: { width: 80, height: 40 },
    });
    const step = node('step', 40, 40, { parentId: 'group' });
    const outside = node('out', 900, 500);
    const updates = layoutCanvasNodes({
      nodes: [group, start, step, outside],
      edges: [edge('start', 'step'), edge('group', 'out')],
    });
    const next = placed([group, start, step, outside], updates);
    const nextStart = next.find((item) => item.id === 'start')!;
    const nextStep = next.find((item) => item.id === 'step')!;
    const nextGroup = next.find((item) => item.id === 'group')!;

    expect(nextStart.position.x).toBeLessThan(nextStep.position.x);
    expect(leftOf(nextStart)).toBeGreaterThanOrEqual(0);
    expect(leftOf(nextStep)).toBeGreaterThanOrEqual(0);
    expect(nextGroup.width).toBeGreaterThanOrEqual(
      rightOf(nextStep) + CanvasAutoLayoutSpacing.groupPadding,
    );
    expect(nextGroup.height).toBeGreaterThanOrEqual(
      nextStep.position.y + HEIGHT + CanvasAutoLayoutSpacing.groupPadding,
    );
  });

  it('does not shift a shared merge twice or pull the source', () => {
    const nodes = [
      node('src', 80, 240, {
        data: { label: 'Switch', form: { conditions: [{}, {}] } },
      }),
      node('top', 480, 420),
      node('bottom', 500, 440),
      node('merge', 900, 200),
    ];
    const edges = [
      edge('src', 'top', 'Case 1'),
      edge('src', 'bottom', 'Case 2'),
      edge('top', 'merge'),
      edge('bottom', 'merge'),
      edge('merge', 'src'),
    ];
    const next = placed(nodes, layoutCanvasNodes({ nodes, edges }));
    const src = next.find((item) => item.id === 'src')!;
    const top = next.find((item) => item.id === 'top')!;
    const bottom = next.find((item) => item.id === 'bottom')!;
    const merge = next.find((item) => item.id === 'merge')!;

    expect(top.position.y).toBeLessThan(bottom.position.y);
    expect(
      bottom.position.y - (top.position.y + HEIGHT),
    ).toBeGreaterThanOrEqual(
      CanvasAutoLayoutSpacing.nodeGap - CanvasAutoLayoutSpacing.grid,
    );
    expect(src.position.x).toBeLessThan(top.position.x);
    expect(
      Math.abs(centerY(src) - (centerY(top) + centerY(bottom)) / 2),
    ).toBeLessThanOrEqual(HEIGHT);
    expect(merge.position.x).toBeGreaterThan(top.position.x);
    expect(merge.position.y).toBeGreaterThanOrEqual(
      top.position.y - CanvasAutoLayoutSpacing.grid,
    );
    expect(merge.position.y + HEIGHT).toBeLessThanOrEqual(
      bottom.position.y + HEIGHT + CanvasAutoLayoutSpacing.grid,
    );
  });

  it('survives a cycle and stays stable on a second pass', () => {
    const nodes = [node('a', 0, 0), node('b', 300, 180)];
    const edges = [edge('a', 'b'), edge('b', 'a')];
    const first = layoutCanvasNodes({ nodes, edges });
    expect(first.size).toBeGreaterThan(0);
    const once = placed(nodes, first);
    const second = layoutCanvasNodes({ nodes: once, edges });
    expect(second.size).toBe(0);
  });
});
