// Tests for the single-mode dsl-bridge.
//
// The bridge has exactly one wire shape. Public API under test:
// `initialEmptyDsl`, `importDsl`, `dslToGraph`, `graphToDsl`,
// `exportDsl`, `inferIsAgentFromImport`. The bridge has one wire
// shape and a single import path.

// ─── Round-trip stability integration tests ────────────────────────────
//
// These exercise the full bridge: `importDsl` → `dslToGraph` →
// `graphToDsl` → `dslToGraph` → `exportDsl`, and assert that the
// re-emitted payload matches the input modulo React-Flow-internal
// fields (`dragging`, `selected`, `measured`, `data.isHovered`).
// Those fields are transient UI state that React Flow re-derives on
// every mount, so a round-trip that flips them is harmless. Any
// other mismatch (semantic field, position, edge topology, …) is
// a real bug and fails the test.
//
// To add a new fixture, drop a JSON file under
// `internal/agent/dsl/testdata/` and add a `describe`
// block keyed by the fixture name. The diff helper classifies
// React-Flow internals automatically — no per-fixture work needed.

import * as bridge from '../dsl-bridge';
const REACT_FLOW_NODE_INTERNALS = new Set(['dragging', 'selected', 'measured']);
const REACT_FLOW_EDGE_INTERNALS = new Set(['isHovered']);

const isInternalField = (path: string, key: string): boolean => {
  if (REACT_FLOW_NODE_INTERNALS.has(key)) return true;
  // Nested measured.* properties (measured.width, measured.height)
  // are React-Flow-managed and should be treated as internals.
  if (path.split('.').some((seg) => REACT_FLOW_NODE_INTERNALS.has(seg)))
    return true;
  if (REACT_FLOW_EDGE_INTERNALS.has(key)) {
    // Top-level `isHovered` on an edge, or nested under `edges[].data`.
    return /(^|\.)edges(\.|$|\[)/.test(path) || path.endsWith('.data');
  }
  return false;
};

interface Diff {
  warnings: string[];
  failures: string[];
}

const diffDsl = (expected: any, actual: any, path = ''): Diff => {
  const out: Diff = { warnings: [], failures: [] };
  compareInto(expected, actual, path, out);
  return out;
};

const compareInto = (
  expected: any,
  actual: any,
  path: string,
  out: Diff,
): void => {
  if (expected === actual) return;
  if (
    expected === null ||
    actual === null ||
    typeof expected !== typeof actual
  ) {
    record(out, path, 'value', expected, actual, '');
    return;
  }
  if (typeof expected !== 'object') {
    record(out, path, 'value', expected, actual, '');
    return;
  }
  const expArr = Array.isArray(expected);
  const actArr = Array.isArray(actual);
  if (expArr !== actArr) {
    out.failures.push(`${path}: array/object mismatch`);
    return;
  }
  if (expArr) {
    if (expected.length !== actual.length) {
      out.failures.push(
        `${path}: length ${expected.length} != ${actual.length}`,
      );
    }
    const n = Math.min(expected.length, actual.length);
    for (let i = 0; i < n; i++) {
      compareInto(expected[i], actual[i], `${path}[${i}]`, out);
    }
    return;
  }
  const allKeys = new Set([...Object.keys(expected), ...Object.keys(actual)]);
  for (const key of allKeys) {
    const sub = path ? `${path}.${key}` : key;
    if (!(key in expected)) {
      record(out, sub, 'missing in expected', undefined, actual[key], key);
      continue;
    }
    if (!(key in actual)) {
      record(out, sub, 'missing in actual', expected[key], undefined, key);
      continue;
    }
    if (typeof expected[key] === 'object' && expected[key] !== null) {
      compareInto(expected[key], actual[key], sub, out);
    } else if (expected[key] !== actual[key]) {
      record(out, sub, 'value', expected[key], actual[key], key);
    }
  }
};

const record = (
  out: Diff,
  path: string,
  kind: string,
  exp: any,
  act: any,
  key: string,
): void => {
  const msg = `${path}: ${kind} (${stableStr(exp)} vs ${stableStr(act)})`;
  if (isInternalField(path, key)) {
    out.warnings.push(msg);
  } else {
    out.failures.push(msg);
  }
};

const stableStr = (v: any): string => {
  if (v === undefined) return 'undefined';
  if (typeof v === 'string') return JSON.stringify(v);
  try {
    return JSON.stringify(v);
  } catch {
    return String(v);
  }
};

// roundTrip imports `input`, re-derives the React-Flow state, and
// re-exports — returning the export payload. The bridge has one
// wire shape; `importDsl` reads the canonical `graph` block and
// normalises the rest of the dsl shape.
const roundTrip = (bridge: any, input: any): any => {
  const imported = bridge.importDsl(input, true);
  const { nodes, edges } = bridge.dslToGraph(imported);
  // Re-derive dsl from React-Flow state (mirrors what the canvas
  // store does on every edit), then re-import to flatten transient
  // fields the way GetAgent would, then export.
  const redrawn = bridge.graphToDsl(nodes, edges, imported);
  const { nodes: n2, edges: e2 } = bridge.dslToGraph(redrawn);
  return bridge.exportDsl(n2, e2, redrawn);
};

describe('dsl-bridge round-trip stability', () => {
  // Realistic v2 fixture with React-Flow-internal fields. Mirrors
  // the structure of internal/agent/dsl/testdata/browser.json
  // but inlined here so the test is self-contained.
  const v2BrowserLike = {
    graph: {
      nodes: [
        {
          id: 'begin',
          type: 'beginNode',
          position: { x: 218.5, y: 138.5 },
          data: { label: 'Begin', name: 'begin', form: { prologue: 'Hi' } },
          sourcePosition: 'left',
          targetPosition: 'right',
          dragging: false,
          selected: false,
          measured: { width: 200, height: 81 },
        },
        {
          id: 'Browser:BusyHatsSink',
          type: 'ragNode',
          position: { x: 385.29, y: 264.35 },
          data: {
            label: 'Browser',
            name: 'Browser_0',
            form: { headless: true, max_steps: 30 },
          },
          sourcePosition: 'right',
          targetPosition: 'left',
          dragging: false,
          selected: true,
          measured: { width: 200, height: 49 },
        },
        {
          id: 'Message:QuietMonkeysLead',
          type: 'messageNode',
          position: { x: 554.79, y: 120.35 },
          data: {
            label: 'Message',
            name: 'Reply_0',
            form: { content: ['{Browser:BusyHatsSink@content}'] },
          },
          sourcePosition: 'right',
          targetPosition: 'left',
          dragging: false,
          selected: false,
          measured: { width: 200, height: 85 },
        },
      ],
      edges: [
        {
          id: 'xy-edge__beginstart-Browser:BusyHatsSinkend',
          source: 'begin',
          sourceHandle: 'start',
          target: 'Browser:BusyHatsSink',
          targetHandle: 'end',
          data: { isHovered: false },
        },
        {
          id: 'xy-edge__Browser:BusyHatsSinkstart-Message:QuietMonkeysLeadend',
          source: 'Browser:BusyHatsSink',
          sourceHandle: 'start',
          target: 'Message:QuietMonkeysLead',
          targetHandle: 'end',
          data: { isHovered: false },
        },
      ],
    },
    components: {
      begin: {
        obj: {
          component_name: 'Begin',
          params: { prologue: 'Hi', mode: 'conversational' },
        },
        downstream: ['Browser:BusyHatsSink'],
        upstream: [],
      },
      'Browser:BusyHatsSink': {
        obj: {
          component_name: 'Browser',
          params: { headless: true, max_steps: 30 },
        },
        downstream: ['Message:QuietMonkeysLead'],
        upstream: ['begin'],
      },
      'Message:QuietMonkeysLead': {
        obj: {
          component_name: 'Message',
          params: { content: ['{Browser:BusyHatsSink@content}'] },
        },
        downstream: [],
        upstream: ['Browser:BusyHatsSink'],
      },
    },
    retrieval: [],
    history: [],
    path: [],
    variables: [],
    globals: {
      'sys.conversation_turns': 0,
      'sys.date': '',
      'sys.files': [],
      'sys.history': [],
      'sys.query': '',
      'sys.user_id': '',
    },
  };

  // importDsl strict-graph contract: only payloads with
  // `raw.graph.nodes` render as-is. Everything else — a
  // components-only payload, a `_layout`-only payload (legacy
  // v1 export, intentionally ignored), or an empty file —
  // falls through to the empty seed. This is the documented
  // contract as of 2026-06: the bridge has one wire shape and
  // import accepts only the canonical graph block.
  describe('importDsl strict-graph contract', () => {
    it('components-only payload → empty seed (no fallback derivation)', () => {
      const componentsOnly = {
        components: {
          begin: {
            obj: { component_name: 'Begin', params: {} },
            downstream: [],
            upstream: [],
          },
        },
      };
      const out = bridge.importDsl(componentsOnly, true) as any;
      expect(out.graph.nodes).toEqual([]);
      expect(out.graph.edges).toEqual([]);
      // components-only payload (no `graph.nodes`) falls through
      // to the empty seed; components from the input are NOT
      // propagated because the strict-graph contract only
      // renders what comes in via `graph`.
      expect(out.components).toEqual({});
    });

    it('_layout-only payload (legacy v1 export) → empty seed', () => {
      // The front-end previously read `dsl._layout` to recover
      // canvas positions from historical v1 export files. As of
      // the v1/v2 split removal, `_layout` is no longer part of
      // the wire contract and is silently dropped — a payload
      // that carries only `_layout` (and nothing else) renders
      // as an empty canvas.
      const layoutOnly = {
        _layout: {
          nodes: [
            {
              id: 'begin',
              type: 'beginNode',
              position: { x: 50, y: 200 },
              data: { label: 'Begin', name: 'begin' },
            },
          ],
          edges: [],
        },
      };
      const out = bridge.importDsl(layoutOnly, true) as any;
      expect(out.graph.nodes).toEqual([]);
      expect(out.graph.edges).toEqual([]);
    });

    it('empty file → empty seed', () => {
      const out = bridge.importDsl({}, true) as any;
      expect(out.graph.nodes).toEqual([]);
      expect(out.graph.edges).toEqual([]);
    });
  });

  it('v2 input round-trip: graph + components survive export (modulo RF internals)', () => {
    const exported = roundTrip(bridge, v2BrowserLike) as any;

    // The structural parts (graph, components) must be byte-stable.
    // Top-level envelope fields like `retrieval`/`history` are
    // stripped by v2 exportDsl on purpose, so we focus on `graph`
    // and `components` — the two payloads that carry the canvas
    // state.
    const diff = diffDsl(v2BrowserLike.graph, exported.graph, 'graph');
    if (diff.warnings.length > 0) {
      // eslint-disable-next-line no-console
      console.warn(
        '[v2 round-trip] React-Flow-internal mismatches (warnings, not failures):',
        diff.warnings,
      );
    }
    expect(diff.failures).toEqual([]);

    // Components round-trip too
    expect(exported.components).toBeDefined();
    expect(Object.keys(exported.components)).toHaveLength(3);
    expect(exported.components['Browser:BusyHatsSink'].obj.component_name).toBe(
      'Browser',
    );
  });

  it('diffDsl: semantic field mismatch → failure, RF internal → warning', () => {
    // Direct unit test of the diff classifier, independent of the
    // bridge. Verifies that the warning/failure split behaves the
    // way downstream tests rely on.
    const expected = {
      id: 'n1',
      type: 'beginNode',
      position: { x: 100, y: 100 },
      dragging: false,
      selected: false,
      measured: { width: 200, height: 81 },
    };
    const actual = {
      id: 'n1',
      type: 'beginNode',
      position: { x: 999, y: 100 }, // semantic mismatch
      dragging: true, // RF internal
      selected: true, // RF internal
      measured: { width: 999, height: 81 }, // RF internal
    };
    const diff = diffDsl(expected, actual);
    expect(diff.warnings).toEqual(
      expect.arrayContaining([
        expect.stringMatching(/dragging/),
        expect.stringMatching(/selected/),
        expect.stringMatching(/measured/),
      ]),
    );
    expect(diff.failures).toEqual(['position.x: value (100 vs 999)']);
  });
});
