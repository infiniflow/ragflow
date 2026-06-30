// DSL bridge — single wire shape for the agent canvas.
//
// The RAGFlow agent DSL has exactly one canonical wire shape, used
// for every operation (PUT/GET/create/export/import):
//
//   {
//     "globals":    {...},
//     "graph":      { "nodes": [...], "edges": [...] },   // React-Flow
//     "variables":  {...},
//     "components": { "<Name>:<UUID>": {                   // execution topology
//       "downstream": [...], "upstream": [...],
//       "obj": { "component_name": "Name", "params": {...} }
//     }},
//     "path": [...], "retrieval": {...}, "history": [...]
//   }
//
// `graph` is React-Flow's layout surface (positions, source/target
// handles). `components` is the topology the engine executes. The
// front-end rebuilds `components` from `graph` on every save so the
// two stay in lockstep; the back-end reads `components` only and
// ignores `graph`.
//
// `importDsl` reads the canonical `graph` block from a parsed
// import file. `_layout` is intentionally NOT consumed here — it
// was a one-shot import-time hint that historical v1 export files
// used to carry canvas positions, and it has not been a wire
// contract field since the v1/v2 split was removed. A payload with
// `_layout` but no `graph` (or with neither) falls through to the
// empty seed.

import { Edge } from '@xyflow/react';

import { DataflowOperator, EmptyDsl, Operator } from '@/constants/agent';
import {
  DSL,
  DSLComponents,
  GlobalVariableType,
  IOperator,
  RAGFlowNodeType,
} from '@/interfaces/database/agent';
import { DataflowEmptyDsl } from '@/pages/agent/empty-dsl';

import { buildDslComponentsByGraph, buildDslGlobalVariables } from '../utils';

const LEGACY_ITERATION_NODE_TYPE = 'group';
const ITERATION_NODE_TYPE = 'iterationNode';

const normalizeGraphNodes = (nodes: RAGFlowNodeType[]): RAGFlowNodeType[] =>
  nodes.map((node) => {
    if (
      node?.data?.label === Operator.Iteration &&
      node.type === LEGACY_ITERATION_NODE_TYPE
    ) {
      return {
        ...node,
        type: ITERATION_NODE_TYPE,
      };
    }

    return node;
  });

// ─── Public API ─────────────────────────────────────────────────────────

/** Initial empty DSL for a new canvas. `isAgent` picks agent vs dataflow seed. */
export const initialEmptyDsl = (isAgent: boolean): DSL =>
  isAgent ? (EmptyDsl as unknown as DSL) : (DataflowEmptyDsl as unknown as DSL);

/**
 * Convert a parsed JSON object from a user-uploaded file into a
 * renderable DSL. Caller must have already parsed the file (we take
 * the object, not the string). `isAgent` is the form-level flag, not
 * re-inferred here.
 *
 * Reads the canonical `graph` block from a parsed import file.
 * `raw.graph.nodes` must be present and non-empty for the input
 * to be rendered as-is; anything else falls through to the empty
 * seed (an empty canvas).
 *
 * `_layout` is NOT read — that field was a one-shot import-time
 * hint in historical v1 export files and is no longer part of the
 * wire contract. A payload that carries `_layout` (and nothing
 * else) is treated the same as an empty file.
 */
export const importDsl = (
  rawParsed: Record<string, any>,
  isAgent: boolean,
): DSL => {
  const seed = isAgent ? EmptyDsl : DataflowEmptyDsl;

  // Single precedence level: `raw.graph.nodes` is the canonical
  // wire shape. Every DSL the back-end returns has a populated
  // `graph` block, so anything else (a `_layout`-only payload from
  // a stale test fixture, a `components`-only payload from a
  // third-party tool, an empty file) falls through to the empty
  // seed.
  let graph: { nodes: RAGFlowNodeType[]; edges: Edge[] };
  let components: DSLComponents;

  if (Array.isArray(rawParsed?.graph?.nodes)) {
    const rawEdges = rawParsed.graph.edges;
    const edges: Edge[] = Array.isArray(rawEdges) ? rawEdges : [];
    graph = {
      nodes: normalizeGraphNodes(rawParsed.graph.nodes as RAGFlowNodeType[]),
      edges,
    };
    components =
      (rawParsed.components as DSLComponents | undefined) ??
      (buildDslComponentsByGraph(
        graph.nodes,
        graph.edges,
        seed.components as DSLComponents,
      ) as DSLComponents);
  } else {
    graph = { nodes: [], edges: [] };
    components = seed.components as DSLComponents;
  }

  return {
    ...seed,
    graph,
    components,
    retrieval: rawParsed.retrieval ?? seed.retrieval,
    history: rawParsed.history ?? seed.history,
    path: rawParsed.path ?? seed.path,
    variables: rawParsed.variables ?? seed.variables,
    globals: rawParsed.globals ?? seed.globals,
  } as unknown as DSL;
};

/**
 * Convert a server-returned DSL into the React-Flow `{nodes, edges}`
 * shape the store consumes. Reads `dsl.graph` only; absent
 * `graph.nodes` returns an empty canvas (see function body for
 * the rationale).
 */
export const dslToGraph = (
  dsl: DSL,
): { nodes: RAGFlowNodeType[]; edges: Edge[] } => {
  // Single source of truth: server always populates `graph`, so a
  // server-returned dsl that lacks `graph.nodes` is treated as
  // empty (the back-end should never produce such a payload;
  // treating it as empty keeps the canvas from blowing up if a
  // historical row slips through). No components-only fallback —
  // the strict-graph import contract in `importDsl` ensures the
  // top-level DSL always carries `graph`.
  const graphNodes = dsl?.graph?.nodes;
  if (Array.isArray(graphNodes) && graphNodes.length > 0) {
    const rawEdges = dsl?.graph?.edges;
    return {
      nodes: normalizeGraphNodes(graphNodes as RAGFlowNodeType[]),
      edges: (Array.isArray(rawEdges) ? rawEdges : []) as Edge[],
    };
  }
  return { nodes: [], edges: [] };
};

/**
 * Build a fresh DSL from the current React-Flow state. Emits
 * `graph` + `components` plus a spread of the previous DSL for any
 * untouched fields (`messages`, `path`, `retrieval`, etc.).
 */
export const graphToDsl = (
  currentNodes: RAGFlowNodeType[],
  currentEdges: Edge[],
  oldDsl: DSL,
  globalVariables?: Record<string, GlobalVariableType>,
): DSL => {
  const filteredNodes = currentNodes.filter(
    (n) => n.data?.label !== Operator.Placeholder,
  );
  const filteredEdges = currentEdges.filter((edge) => {
    const s = currentNodes.find((n) => n.id === edge.source);
    const t = currentNodes.find((n) => n.id === edge.target);
    return (
      s?.data?.label !== Operator.Placeholder &&
      t?.data?.label !== Operator.Placeholder
    );
  });

  const dslComponents = buildDslComponentsByGraph(
    filteredNodes,
    filteredEdges,
    oldDsl?.components ?? {},
  );
  const globals = buildDslGlobalVariables(
    oldDsl ?? ({} as DSL),
    globalVariables,
  );

  return {
    ...oldDsl,
    ...globals,
    graph: { nodes: filteredNodes, edges: filteredEdges },
    components: dslComponents,
  };
};

/**
 * Build a downloadable JSON for the export button. Returns the
 * conventional wire shape (`graph` + `components` + `globals` +
 * `variables` + spread of any untouched fields). The caller is
 * responsible for stripping sensitive fields (api_key) before
 * handing the result to the file writer.
 */
export const exportDsl = (
  currentNodes: RAGFlowNodeType[],
  currentEdges: Edge[],
  oldDsl: DSL,
  globalVariables?: Record<string, GlobalVariableType>,
): Record<string, any> => {
  return graphToDsl(
    currentNodes,
    currentEdges,
    oldDsl,
    globalVariables,
  ) as Record<string, any>;
};

/**
 * Detect whether an imported JSON is a dataflow canvas. Looks at
 * both shapes (v1 components / v2 graph.nodes) for the dataflow
 * markers ("File" begin + "Parser"). Defaults to `true` (agent)
 * when ambiguous.
 */
export const inferIsAgentFromImport = (raw: Record<string, any>): boolean => {
  const graph = raw?.graph;
  if (graph && Array.isArray(graph.nodes)) {
    const labels = (graph.nodes as any[]).map((n: any) => n?.data?.label);
    if (
      labels.includes(DataflowOperator.Begin) &&
      labels.includes(DataflowOperator.Parser)
    ) {
      return false;
    }
    return true;
  }
  // No `graph` block — treat as agent. The strict-graph
  // import contract in `importDsl` already handles the
  // empty-payload case elsewhere.
  return true;
};

export type { IOperator };
