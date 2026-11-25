import {
  CanvasAutoLayoutSpacing,
  CanvasLayoutEdge,
  CanvasLayoutNode,
  layoutCanvasNodes,
  relaxPlacement,
} from '../canvas-auto-layout';
import { buildElkLayoutOptions } from '../canvas-elk-layout';
import { describeOrthogonalEdge, orthogonalLanes } from '../canvas-edge-route';
import { measureLayoutQuality } from '../canvas-layout-analysis';

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

async function placed(
  nodes: CanvasLayoutNode[],
  updates?: Map<
    string,
    { x: number; y: number; width?: number; height?: number }
  >,
) {
  const resolved = updates ?? (await layoutCanvasNodes({ nodes, edges: [] }));
  return nodes.map((item) => {
    const update = resolved.get(item.id);
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
  it('uses orthogonal ELK routing only when that style is selected', () => {
    expect(buildElkLayoutOptions('bezier')['elk.edgeRouting']).toBeUndefined();
    expect(buildElkLayoutOptions('orthogonal')['elk.edgeRouting']).toBe(
      'ORTHOGONAL',
    );
    expect(
      buildElkLayoutOptions('bezier', { direction: 'TB' })['elk.direction'],
    ).toBe('DOWN');
    expect(
      buildElkLayoutOptions('bezier', { nodeSpacing: 80, rankSpacing: 140 })[
        'elk.spacing.nodeNode'
      ],
    ).toBe('80');
  });

  it('uses dagre as its own layout and can flow downward', async () => {
    const nodes = [
      node('s', 0, 200, {
        data: { label: 'Switch', form: { conditions: [{}, {}] } },
      }),
      node('a', 300, 0),
      node('a2', 700, 400),
      node('b', 320, 200),
      node('c', 340, 500),
    ];
    const edges = [
      edge('s', 'a', 'Case 1'),
      edge('a', 'a2'),
      edge('s', 'b', 'Case 2'),
      edge('s', 'c', 'end_cpn_ids'),
    ];
    const shared = {
      direction: 'LR' as const,
      nodeSpacing: 96,
      rankSpacing: 160,
      route: 'bezier' as const,
    };
    const elk = await placed(
      nodes,
      await layoutCanvasNodes({
        nodes,
        edges,
        layout: { ...shared, algorithm: 'elk' },
      }),
    );
    const dagreNodes = await placed(
      nodes,
      await layoutCanvasNodes({
        nodes,
        edges,
        layout: { ...shared, algorithm: 'dagre' },
      }),
    );
    const at = (list: typeof elk, id: string) =>
      list.find((item) => item.id === id)!.position;
    expect(Math.abs(at(elk, 'a').y - at(elk, 'a2').y)).toBeLessThanOrEqual(1);
    const identical = ['a', 'a2', 'b', 'c'].every((id) => {
      const left = at(elk, id);
      const right = at(dagreNodes, id);
      return Math.abs(left.x - right.x) < 8 && Math.abs(left.y - right.y) < 8;
    });
    expect(identical).toBe(false);

    const down = await placed(
      nodes,
      await layoutCanvasNodes({
        nodes,
        edges,
        layout: { ...shared, algorithm: 'elk', direction: 'TB' },
      }),
    );
    expect(at(down, 'a').y).toBeGreaterThan(at(down, 's').y);
  });

  it('staggers orthogonal elbows so parallel edges do not share one vertical line', () => {
    const edges = [
      { id: 'a', sourceX: 220, sourceY: 40, targetX: 760, targetY: 80 },
      { id: 'b', sourceX: 220, sourceY: 180, targetX: 760, targetY: 220 },
      { id: 'c', sourceX: 220, sourceY: 320, targetX: 760, targetY: 360 },
    ];
    const lanes = orthogonalLanes(edges);
    const bends = edges.map((edge) => {
      const lane = lanes.get(edge.id)!;
      return describeOrthogonalEdge(
        { x: edge.sourceX, y: edge.sourceY },
        { x: edge.targetX, y: edge.targetY },
        lane.step,
        lane.lane,
        lane.laneCount,
      ).path;
    });
    const elbowX = bends.map((path) => {
      const match = /Q([\d.]+),/.exec(path);
      return match ? Number(match[1]) : 0;
    });
    expect(new Set(elbowX).size).toBe(3);
    expect(elbowX[0]).toBeLessThan(elbowX[1]);
    expect(elbowX[1]).toBeLessThan(elbowX[2]);
  });

  it('places a chain left to right on one row with an even gap', async () => {
    const nodes = [node('a', 40, 900), node('b', 800, 40), node('c', 20, 500)];
    const edges = [edge('a', 'b'), edge('b', 'c')];
    const updates = await layoutCanvasNodes({ nodes, edges });
    const next = await placed(nodes, updates);
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

  it('stacks branches and centers them on the parent', async () => {
    const nodes = [
      node('a', 100, 100),
      node('b', 400, 40),
      node('c', 420, 400),
    ];
    const edges = [edge('a', 'b'), edge('a', 'c')];
    const next = await placed(nodes, await layoutCanvasNodes({ nodes, edges }));
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

  it('keeps fan-out in top-to-bottom handle order so edges do not cross', async () => {
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
    const next = await placed(nodes, await layoutCanvasNodes({ nodes, edges }));
    const tops = ['top', 'upper', 'lower', 'bottom'].map(
      (id) => next.find((item) => item.id === id)!.position.y,
    );
    expect(tops[0]).toBeLessThan(tops[1]);
    expect(tops[1]).toBeLessThan(tops[2]);
    expect(tops[2]).toBeLessThan(tops[3]);
  });

  it('splits two outputs of one node onto separate rows', async () => {
    const nodes = [
      node('src', 0, 200),
      node('upper', 500, 200),
      node('lower', 520, 200),
    ];
    const edges = [
      edge('src', 'upper', 'Case 1'),
      edge('src', 'lower', 'Case 2'),
    ];
    const next = await placed(
      nodes,
      await layoutCanvasNodes({
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

  it('does not shift a shared merge twice when outputs are separated', async () => {
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
    const next = await placed(nodes, await layoutCanvasNodes({ nodes, edges }));
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

  it('keeps each switch branch in its own band so later nodes do not cross', async () => {
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
    const next = await placed(nodes, await layoutCanvasNodes({ nodes, edges }));
    const y = (id: string) => next.find((item) => item.id === id)!.position.y;

    expect(y('a')).toBeLessThan(y('b'));
    expect(y('b')).toBeLessThan(y('c'));
    expect(y('a2')).toBeLessThan(y('b2'));
    expect(y('b2')).toBeLessThan(y('c2'));
    expect(y('a2')).toBeLessThan(y('b'));
    expect(y('b2')).toBeLessThan(y('c'));
  });

  it('puts a merge node after both branches', async () => {
    const nodes = [node('a', 0, 0), node('b', 10, 200), node('c', 500, 80)];
    const edges = [edge('a', 'c'), edge('b', 'c')];
    const next = await placed(nodes, await layoutCanvasNodes({ nodes, edges }));
    const a = next.find((item) => item.id === 'a')!;
    const b = next.find((item) => item.id === 'b')!;
    const c = next.find((item) => item.id === 'c')!;

    expect(c.position.x).toBeGreaterThan(a.position.x);
    expect(c.position.x).toBeGreaterThan(b.position.x);
    expect(Math.abs(a.position.x - b.position.x)).toBeLessThanOrEqual(1);
  });

  it('keeps tools under the agent and the next step to the right', async () => {
    const nodes = [
      node('agent', 300, 200),
      node('tool', 900, 20, { data: { label: 'Tool' } }),
      node('next', 100, 800),
    ];
    const edges = [edge('agent', 'tool', 'tool'), edge('agent', 'next')];
    const nextNodes = await placed(
      nodes,
      await layoutCanvasNodes({ nodes, edges }),
    );
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

  it('stacks disconnected flows without overlapping them', async () => {
    const nodes = [node('a', 1000, 100), node('b', 1400, 100)];
    const next = await placed(
      nodes,
      await layoutCanvasNodes({ nodes, edges: [] }),
    );
    const a = next.find((item) => item.id === 'a')!;
    const b = next.find((item) => item.id === 'b')!;
    const upper = a.position.y <= b.position.y ? a : b;
    const lower = upper === a ? b : a;

    expect(lower.position.y).toBeGreaterThanOrEqual(upper.position.y + HEIGHT);
    expect(Math.abs(leftOf(a) - leftOf(b))).toBeLessThanOrEqual(
      CanvasAutoLayoutSpacing.grid,
    );
  });

  it('leaves notes that cover nothing and reseats a note that covers a node', async () => {
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
    const updates = await layoutCanvasNodes({
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

  it('does not move placeholder nodes', async () => {
    const nodes = [
      node('a', 10, 10),
      node('b', 400, 300),
      node('ph', 50, 50, {
        type: 'placeholderNode',
        data: { label: 'Placeholder' },
      }),
    ];
    const updates = await layoutCanvasNodes({
      nodes,
      edges: [edge('a', 'b')],
    });
    expect(updates.has('ph')).toBe(false);
  });

  it('arranges the whole canvas even when some nodes are selected', async () => {
    const nodes = [node('a', 10, 10), node('b', 600, 400), node('c', 50, 700)];
    const updates = await layoutCanvasNodes({
      nodes,
      edges: [edge('a', 'b'), edge('b', 'c')],
      selectedNodeIds: ['a', 'b'],
    });
    expect(updates.has('a') || updates.has('b') || updates.has('c')).toBe(true);
    const next = await placed(nodes, updates);
    const a = next.find((item) => item.id === 'a')!;
    const b = next.find((item) => item.id === 'b')!;
    const c = next.find((item) => item.id === 'c')!;
    expect(a.position.x).toBeLessThan(b.position.x);
    expect(b.position.x).toBeLessThan(c.position.x);
  });

  it('arranges the full graph when a node lists only some of its handles', async () => {
    const nodes = Array.from({ length: 12 }, (_, index) =>
      node(`n${index}`, index * 30, 0, {
        handles:
          index === 3 ? [{ id: 'start', type: 'source', y: 10 }] : undefined,
      }),
    );
    const edges = [
      ...Array.from({ length: 11 }, (_, index) =>
        edge(`n${index}`, `n${index + 1}`),
      ),
      edge('n3', 'n8', 'Case 1'),
    ];
    const updates = await layoutCanvasNodes({ nodes, edges });
    expect(updates.size).toBeGreaterThan(0);
    const next = await placed(nodes, updates);
    expect(next.find((item) => item.id === 'n0')!.position.x).toBeLessThan(
      next.find((item) => item.id === 'n11')!.position.x,
    );
  });

  it('lays child nodes out inside a group and resizes the frame', async () => {
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
    const updates = await layoutCanvasNodes({
      nodes: [group, start, step, outside],
      edges: [edge('start', 'step'), edge('group', 'out')],
    });
    const next = await placed([group, start, step, outside], updates);
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

  it('keeps a switch with long branches in separate lanes and off each other', async () => {
    const sw = node('sw', 0, 400, {
      data: { label: 'Switch', form: { conditions: [{}, {}, {}] } },
    });
    const nodes = [
      node('begin', -400, 900),
      node('route', -200, 100),
      node('intent', 0, 700),
      sw,
      node('bicim', 400, 900),
      node('msg1', 800, 20),
      node('sablon', 380, 200),
      node('retrieval', 760, 800),
      node('msg2', 1100, 100),
      node('yaz', 420, 40),
      node('code', 780, 500),
      node('msg3', 1120, 900),
      node('fillup', 400, 600),
      node('final', 1500, 300),
    ];
    const edges = [
      edge('begin', 'route'),
      edge('route', 'intent'),
      edge('intent', 'sw'),
      edge('sw', 'bicim', 'Case 1'),
      edge('bicim', 'msg1'),
      edge('sw', 'sablon', 'Case 2'),
      edge('sablon', 'retrieval'),
      edge('retrieval', 'msg2'),
      edge('sw', 'yaz', 'Case 3'),
      edge('yaz', 'code'),
      edge('code', 'msg3'),
      edge('sw', 'fillup', 'end_cpn_ids'),
      edge('msg1', 'final'),
      edge('msg2', 'final'),
      edge('msg3', 'final'),
      edge('fillup', 'final'),
    ];
    const next = await placed(nodes, await layoutCanvasNodes({ nodes, edges }));
    const y = (id: string) => next.find((item) => item.id === id)!.position.y;
    const x = (id: string) => next.find((item) => item.id === id)!.position.x;

    expect(y('bicim')).toBeLessThan(y('sablon'));
    expect(y('sablon')).toBeLessThan(y('yaz'));
    expect(y('yaz')).toBeLessThan(y('fillup'));
    expect(Math.abs(y('msg1') - y('bicim'))).toBeLessThanOrEqual(1);
    expect(Math.abs(y('retrieval') - y('sablon'))).toBeLessThanOrEqual(1);
    expect(Math.abs(y('msg2') - y('retrieval'))).toBeLessThanOrEqual(1);
    expect(Math.abs(y('code') - y('yaz'))).toBeLessThanOrEqual(1);
    expect(Math.abs(y('msg3') - y('code'))).toBeLessThanOrEqual(1);
    expect(x('final')).toBeGreaterThan(x('msg2'));
    expect(x('begin')).toBeLessThan(x('route'));
    expect(x('route')).toBeLessThan(x('intent'));
    expect(x('intent')).toBeLessThan(x('sw'));

    const boxes = new Map(
      next.map((item) => [
        item.id,
        {
          x: item.position.x - WIDTH / 2,
          y: item.position.y,
          width: WIDTH,
          height: item.measured?.height ?? HEIGHT,
        },
      ]),
    );
    const quality = measureLayoutQuality(boxes, edges);
    expect(quality.nodeOverlaps).toBe(0);
    expect(quality.backwardEdges).toBe(0);
    expect(quality.edgeCrossings).toBe(0);
  });

  it('keeps a cross-linked retrieval on the else row and the case target on its own row', async () => {
    const nodes = [
      node('begin', 0, 200),
      node('sw', 300, 40, {
        data: { label: 'Switch', form: { conditions: [{}] } },
      }),
      node('agentKb', 900, 800),
      node('msgKb', 1200, 20),
      node('retrieval', 600, 400),
      node('agent', 900, 100),
      node('msg', 1200, 700),
    ];
    const edges = [
      edge('begin', 'sw'),
      edge('sw', 'agentKb', 'Case 1'),
      edge('agentKb', 'msgKb'),
      edge('sw', 'retrieval', 'end_cpn_ids'),
      edge('retrieval', 'agentKb'),
      edge('retrieval', 'agent'),
      edge('agent', 'msg'),
    ];
    const next = await placed(nodes, await layoutCanvasNodes({ nodes, edges }));
    const y = (id: string) => next.find((item) => item.id === id)!.position.y;
    const x = (id: string) => next.find((item) => item.id === id)!.position.x;
    const at = (id: string) => next.find((item) => item.id === id)!;

    expect(y('agentKb')).toBeLessThan(y('retrieval'));
    expect(y('agentKb')).toBeLessThan(y('agent'));
    expect(Math.abs(y('msgKb') - y('agentKb'))).toBeLessThanOrEqual(1);
    expect(Math.abs(y('agent') - y('retrieval'))).toBeLessThanOrEqual(1);
    expect(Math.abs(y('msg') - y('agent'))).toBeLessThanOrEqual(1);
    expect(
      Math.abs(centerY(at('begin')) - centerY(at('sw'))),
    ).toBeLessThanOrEqual(CanvasAutoLayoutSpacing.grid * 2);
    expect(x('begin')).toBeLessThan(x('sw'));
    expect(x('sw')).toBeLessThan(x('retrieval'));
    expect(x('retrieval')).toBeLessThan(x('agent'));
    expect(x('agentKb')).toBeGreaterThan(x('sw'));
  });

  it('survives a cycle and stays stable on a second pass', async () => {
    const nodes = [node('a', 0, 0), node('b', 300, 180)];
    const edges = [edge('a', 'b'), edge('b', 'a')];
    const first = await layoutCanvasNodes({ nodes, edges });
    expect(first.size).toBeGreaterThan(0);
    const once = await placed(nodes, first);
    const second = await layoutCanvasNodes({ nodes: once, edges });
    expect(second.size).toBe(0);
  });
});

describe('relaxPlacement', () => {
  it('separates overlapping cards by a small gap and leaves the row compact', async () => {
    const agent = node('agent', 200, 0, { data: { label: 'Agent' } });
    const message = node('message', 200, 40, { data: { label: 'Message' } });
    const neighbor = node('neighbor', 700, 0);
    const boxes = new Map([
      ['agent', { x: 100, y: 0, width: WIDTH, height: 80 }],
      ['message', { x: 100, y: 40, width: WIDTH, height: HEIGHT }],
      ['neighbor', { x: 700, y: 0, width: WIDTH, height: HEIGHT }],
    ]);
    relaxPlacement(boxes, [agent, message, neighbor], []);
    expect(boxes.get('message')!.y).toBeGreaterThanOrEqual(80 + 32 - 1);
    expect(boxes.get('message')!.y).toBeLessThan(200);
    expect(boxes.get('neighbor')!.y).toBe(0);
    expect(boxes.get('agent')!.y).toBe(0);
  });

  it('does not scatter a compact row to dodge a cable', async () => {
    const left = node('left', 100, 200);
    const mid = node('mid', 400, 80, { data: { label: 'Switch' } });
    const right = node('right', 800, 200);
    const boxes = new Map([
      ['left', { x: 0, y: 160, width: WIDTH, height: HEIGHT }],
      ['mid', { x: 360, y: 40, width: WIDTH, height: HEIGHT }],
      ['right', { x: 760, y: 160, width: WIDTH, height: HEIGHT }],
    ]);
    relaxPlacement(boxes, [left, mid, right], [edge('left', 'right')]);
    expect(boxes.get('left')!.y).toBe(160);
    expect(boxes.get('mid')!.y).toBe(40);
    expect(boxes.get('right')!.y).toBe(160);
  });

  it('moves an agent with its tool when the tool overlaps another card', async () => {
    const agent = node('agent', 400, 0, { data: { label: 'Agent' } });
    const tool = node('tool', 400, 100, { data: { label: 'Tool' } });
    const other = node('other', 400, 120);
    const boxes = new Map([
      ['agent', { x: 300, y: 0, width: WIDTH, height: HEIGHT }],
      ['tool', { x: 300, y: 100, width: WIDTH, height: HEIGHT }],
      ['other', { x: 320, y: 140, width: WIDTH, height: HEIGHT }],
    ]);
    const gap = boxes.get('tool')!.y - boxes.get('agent')!.y;
    relaxPlacement(
      boxes,
      [agent, tool, other],
      [edge('agent', 'tool', 'tool')],
    );
    expect(boxes.get('tool')!.y - boxes.get('agent')!.y).toBe(gap);
    expect(boxes.get('other')!.y).toBeGreaterThan(140);
  });
});
