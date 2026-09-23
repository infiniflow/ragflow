/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { filterAllUpstreamNodeIds, getNodeOutputs } from '@/utils/canvas-util';
import { Edge } from '@xyflow/react';
import { isEmpty, isPlainObject } from 'lodash';
import {
  BeginId,
  NodeHandleId,
  Operator,
  RetrievalFrom,
  VariableRegex,
} from '../constant';
import { FormSchema as ParserFormSchema } from '../form/parser-form/schema';
import { isEmptyMessageContent } from '../utils';

/**
 * Canvas checklist issue collection. Pure functions fed by `useCanvasChecklist`:
 * everything graph-derived (orphan steps, dangling variable references, missing
 * required fields) is computed synchronously from nodes/edges, while external
 * resources (memories, models, datasets, compilation template groups) arrive as
 * ready-made Sets — `undefined` means "lookup not settled yet" and skips that
 * check so a slow request never produces false issues.
 */

export enum CanvasIssueType {
  InvalidVariable = 'invalidVariable',
  MissingRequired = 'missingRequired',
  Orphan = 'orphan',
}

export type CanvasIssue = {
  nodeId: string;
  toolId?: string;
  nodeName: string;
  operatorLabel: string;
  type: CanvasIssueType;
  messageKey: string;
  messageParams?: Record<string, string>;
};

export type CanvasChecklistInputs = {
  nodes: RAGFlowNodeType[];
  edges: Edge[];
  editedNodeFormIds: string[];
  memoryIds?: Set<string>;
  modelValidIds?: Set<string>;
  staleDatasetIds?: Set<string>;
  compilationTemplateGroupIds?: Set<string>;
  variables?: Record<string, any>;
};

type IssueTarget = Pick<
  CanvasIssue,
  'nodeId' | 'toolId' | 'nodeName' | 'operatorLabel'
>;

// Node ids are `${operator}:${humanId()}` (e.g. `Agent:HipSignsRhyme`); legacy
// DSLs use numeric suffixes (`answer:0`). The colon is mandatory so llm ids
// (`model@instance@provider`), e-mail addresses and URLs never parse as node
// references.
const BareNodeReferenceRegex =
  /^([A-Za-z][A-Za-z0-9_]*):([A-Za-z0-9][A-Za-z0-9_-]*)@(\S+)$/;

// Bare begin-input references (`begin@key` as a whole field value); anchored so
// prose merely starting with `begin@` is not mistaken for a reference.
const BareBeginReferenceRegex = new RegExp(`^${BeginId}@\\S+$`);
const ConversationVariablePrefix = 'env.';

// Free-text fields whose content must never be scanned for references.
const SkippedFormKeys = new Set(['code', 'password']);

// Structural nodes that legitimately have no edges.
const OrphanExemptOperators: string[] = [
  Operator.Begin,
  Operator.Note,
  Operator.Placeholder,
  Operator.IterationStart,
  Operator.LoopStart,
  Operator.ExitLoop,
];

function collectStrings(value: unknown): string[] {
  if (typeof value === 'string') {
    return [value];
  }
  if (Array.isArray(value)) {
    return value.flatMap(collectStrings);
  }
  if (isPlainObject(value)) {
    return Object.values(value as Record<string, unknown>).flatMap(
      collectStrings,
    );
  }
  return [];
}

function collectFormStrings(
  form: Record<string, any>,
  skippedKeys: Set<string>,
): string[] {
  return Object.entries(form).flatMap(([key, value]) =>
    skippedKeys.has(key) ? [] : collectStrings(value),
  );
}

function extractReferencesFromText(text: string): string[] {
  const references: string[] = [];
  for (const match of text.matchAll(VariableRegex)) {
    if (match[1]) {
      references.push(match[1]);
    }
  }
  if (BareNodeReferenceRegex.test(text) || BareBeginReferenceRegex.test(text)) {
    references.push(text);
  }
  return references;
}

type ReferenceValidationContext = {
  nodeMap: Map<string, RAGFlowNodeType>;
  edges: Edge[];
  beginInputKeys: Set<string>;
  variables?: Record<string, any>;
  reachableCache: Map<string, Set<string>>;
};

function buildReachableNodeIds(
  node: RAGFlowNodeType,
  nodeMap: Map<string, RAGFlowNodeType>,
  edges: Edge[],
) {
  // Mirrors the variable picker (useBuildVariableOptions): a node may reference
  // its own upstream, its parent's upstream, and — for Loop parents only — the
  // parent's own outputs.
  const reachable = new Set(filterAllUpstreamNodeIds(edges, [node.id]));
  const parentId = node.parentId;
  if (parentId) {
    for (const id of filterAllUpstreamNodeIds(edges, [parentId])) {
      reachable.add(id);
    }
    if (nodeMap.get(parentId)?.data?.label === Operator.Loop) {
      reachable.add(parentId);
    }
  }
  return reachable;
}

function isReferenceValid(
  reference: string,
  relaxedUpstream: boolean,
  referencingNode: RAGFlowNodeType,
  ctx: ReferenceValidationContext,
): boolean {
  const atIndex = reference.indexOf('@');

  if (atIndex === -1) {
    // sys.* globals are backend built-ins and always resolvable; only
    // conversation variables (env.*) can dangle when the variable is deleted.
    if (reference.startsWith(ConversationVariablePrefix)) {
      return ctx.variables
        ? reference.slice(ConversationVariablePrefix.length) in ctx.variables
        : true;
    }
    return true;
  }

  const nodeId = reference.slice(0, atIndex);
  const rootOutputKey = reference.slice(atIndex + 1).split('.')[0];

  if (nodeId === BeginId) {
    return ctx.beginInputKeys.has(rootOutputKey);
  }

  const targetNode = ctx.nodeMap.get(nodeId);
  if (!targetNode) {
    return false; // the referenced node was deleted
  }

  if (!relaxedUpstream) {
    let reachable = ctx.reachableCache.get(referencingNode.id);
    if (!reachable) {
      reachable = buildReachableNodeIds(
        referencingNode,
        ctx.nodeMap,
        ctx.edges,
      );
      ctx.reachableCache.set(referencingNode.id, reachable);
    }
    if (!reachable.has(nodeId)) {
      return false; // still on the canvas, but no longer connected upstream
    }
  }

  // Dotted sub-paths resolve dynamically on the backend — only the root output
  // key is checked, and only when the node declares outputs at all (e.g.
  // Message exposes outputs on the backend without declaring them here).
  const outputs = getNodeOutputs(targetNode);
  if (!isEmpty(outputs) && !(rootOutputKey in outputs)) {
    return false;
  }

  return true;
}

/**
 * References inside an Agent node's embedded tools/mcp params run in the
 * agent's own context, so they skip the upstream-reachability check. Tool
 * canvas nodes mirror those params and are not scanned at all.
 */
function collectNodeReferenceIssues(
  node: RAGFlowNodeType,
  target: IssueTarget,
  ctx: ReferenceValidationContext,
): CanvasIssue[] {
  const label = node.data?.label;
  const form = node.data?.form;
  if (!isPlainObject(form) || label === Operator.Tool) {
    return [];
  }

  const skippedKeys = SkippedFormKeys;

  let strictTexts: string[];
  let relaxedTexts: string[] = [];
  if (label === Operator.Agent) {
    const { tools, mcp, ...ownForm } = form;
    strictTexts = collectFormStrings(ownForm, skippedKeys);
    relaxedTexts = collectStrings({ tools, mcp });
  } else {
    strictTexts = collectFormStrings(form, skippedKeys);
  }

  const issues: CanvasIssue[] = [];
  const reported = new Set<string>();
  const pushIfInvalid = (reference: string, relaxedUpstream: boolean) => {
    if (
      !reported.has(reference) &&
      !isReferenceValid(reference, relaxedUpstream, node, ctx)
    ) {
      reported.add(reference);
      issues.push({
        ...target,
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'flow.issueVariableInvalid',
        messageParams: { variable: reference },
      });
    }
  };

  for (const text of strictTexts) {
    for (const reference of extractReferencesFromText(text)) {
      pushIfInvalid(reference, false);
    }
  }
  for (const text of relaxedTexts) {
    for (const reference of extractReferencesFromText(text)) {
      pushIfInvalid(reference, true);
    }
  }
  return issues;
}

/**
 * Shared by Retrieval nodes and Agent-embedded Retrieval tools. Emptiness is
 * asserted directly instead of re-parsing the form schema so legacy DSLs keyed
 * on `kb_ids` are never flagged (ported from find-invalid-retrieval.ts).
 */
function collectRetrievalBindingIssues(
  params: Record<string, any> | undefined,
  target: IssueTarget,
  inputs: Pick<CanvasChecklistInputs, 'memoryIds' | 'staleDatasetIds'>,
): CanvasIssue[] {
  if (!params || !isPlainObject(params)) {
    return [];
  }
  const { memoryIds, staleDatasetIds } = inputs;
  const issues: CanvasIssue[] = [];

  if (
    params.retrieval_from === RetrievalFrom.Dataset &&
    Array.isArray(params.dataset_ids) &&
    params.dataset_ids.length === 0
  ) {
    issues.push({
      ...target,
      type: CanvasIssueType.MissingRequired,
      messageKey: 'flow.retrievalDatasetMissing',
    });
  }
  if (
    params.retrieval_from === RetrievalFrom.Memory &&
    Array.isArray(params.memory_ids) &&
    params.memory_ids.length === 0
  ) {
    issues.push({
      ...target,
      type: CanvasIssueType.MissingRequired,
      messageKey: 'flow.retrievalMemoryMissing',
    });
  }
  if (
    staleDatasetIds &&
    params.retrieval_from !== RetrievalFrom.Memory &&
    Array.isArray(params.dataset_ids) &&
    params.dataset_ids.some((id: string) => staleDatasetIds.has(id))
  ) {
    issues.push({
      ...target,
      type: CanvasIssueType.InvalidVariable,
      messageKey: 'chat.datasetUnavailable',
    });
  }
  if (
    memoryIds &&
    params.retrieval_from === RetrievalFrom.Memory &&
    Array.isArray(params.memory_ids) &&
    params.memory_ids.some((id: string) => !memoryIds.has(id))
  ) {
    issues.push({
      ...target,
      type: CanvasIssueType.InvalidVariable,
      messageKey: 'flow.memoryUnavailable',
    });
  }
  return issues;
}

function findToolCanvasNode(
  edges: Edge[],
  nodeMap: Map<string, RAGFlowNodeType>,
  agentNodeId: string,
) {
  const toolNodeId = edges.find(
    (edge) =>
      edge.source === agentNodeId && edge.sourceHandle === NodeHandleId.Tool,
  )?.target;
  return toolNodeId ? nodeMap.get(toolNodeId) : undefined;
}

export function collectCanvasIssues({
  nodes,
  edges,
  editedNodeFormIds,
  memoryIds,
  modelValidIds,
  staleDatasetIds,
  compilationTemplateGroupIds,
  variables,
}: CanvasChecklistInputs): CanvasIssue[] {
  const nodeMap = new Map(nodes.map((x) => [x.id, x]));
  // References always use `begin@key` even when the Begin node's id is a
  // legacy one (`begin:0`), so look the node up by label. The pipeline
  // (dataflow) canvas names its entry node `File` instead of `Begin` — same
  // structural anchor, different label.
  const beginNode = nodes.find(
    (x) => x.data?.label === Operator.Begin || x.data?.label === Operator.File,
  );
  const beginInputKeys = new Set(
    Object.keys(beginNode?.data?.form?.inputs ?? {}),
  );
  const connectedNodeIds = new Set<string>();
  for (const edge of edges) {
    connectedNodeIds.add(edge.source);
    connectedNodeIds.add(edge.target);
  }

  // Orphan = outside the Begin node's connected component. Edges alone are not
  // enough: a group of nodes wired to each other (e.g. Agent -> Agent_1) has
  // incident edges yet is unreachable from Begin. Iteration/Loop children are
  // only linked to their container by `parentId` — outer edges terminate at the
  // container — so containment counts as connectivity too. The graph is walked
  // undirected: any component detached from Begin, single node or a group, is
  // orphan regardless of edge direction. The anchor itself is always part of
  // its own component, so it keeps the edges-only rule: an entry node with no
  // edges at all (an empty pipeline) is still flagged.
  const neighbors = new Map<string, string[]>();
  const link = (a?: string, b?: string) => {
    if (!a || !b || a === b) {
      return;
    }
    if (!neighbors.has(a)) {
      neighbors.set(a, []);
    }
    neighbors.get(a)!.push(b);
    if (!neighbors.has(b)) {
      neighbors.set(b, []);
    }
    neighbors.get(b)!.push(a);
  };
  for (const edge of edges) {
    link(edge.source, edge.target);
  }
  for (const node of nodes) {
    link(node.id, node.parentId);
  }
  let beginComponentIds: Set<string> | undefined;
  if (beginNode) {
    beginComponentIds = new Set([beginNode.id]);
    const stack = [beginNode.id];
    while (stack.length) {
      const current = stack.pop()!;
      for (const next of neighbors.get(current) ?? []) {
        if (!beginComponentIds.has(next)) {
          beginComponentIds.add(next);
          stack.push(next);
        }
      }
    }
  }

  const ctx: ReferenceValidationContext = {
    nodeMap,
    edges,
    beginInputKeys,
    variables,
    reachableCache: new Map(),
  };

  const issues: CanvasIssue[] = [];

  for (const node of nodes) {
    const label = node.data?.label;
    const form = node.data?.form;
    const target: IssueTarget = {
      nodeId: node.id,
      nodeName: node.data?.name ?? node.id,
      operatorLabel: label ?? '',
    };

    const orphan =
      beginComponentIds && node.id !== beginNode?.id
        ? !beginComponentIds.has(node.id)
        : !connectedNodeIds.has(node.id);
    if (label && !OrphanExemptOperators.includes(label) && orphan) {
      issues.push({
        ...target,
        type: CanvasIssueType.Orphan,
        messageKey: 'flow.issueNotConnected',
      });
    }

    if (label === Operator.Message && isEmptyMessageContent(form?.content)) {
      issues.push({
        ...target,
        type: CanvasIssueType.MissingRequired,
        messageKey: 'flow.messageMsg',
      });
    }

    if (label === Operator.Retrieval) {
      issues.push(
        ...collectRetrievalBindingIssues(form, target, {
          memoryIds,
          staleDatasetIds,
        }),
      );
    }

    if (label === Operator.Agent) {
      // Not gated on editedNodeFormIds: a canvas loaded from storage with an
      // empty model must flag on the very next render, not only after edits.
      if (!form?.llm_id) {
        issues.push({
          ...target,
          type: CanvasIssueType.MissingRequired,
          messageKey: 'flow.agentModelMissing',
        });
      }
      if (Array.isArray(form?.tools)) {
        const toolNode = findToolCanvasNode(edges, nodeMap, node.id);
        for (const tool of form.tools) {
          if (tool?.component_name !== Operator.Retrieval) {
            continue;
          }
          // Point at the concrete tool under the concrete agent — the Tool
          // canvas node's own name is just the operator label (legacy canvases
          // even carry the raw i18n key), which says nothing about which tool
          // is broken.
          const toolTarget: IssueTarget = {
            nodeId: toolNode?.id ?? node.id,
            toolId: toolNode ? tool.id || tool.component_name : undefined,
            nodeName: `${node.data?.name ?? node.id} / ${
              tool.name || tool.component_name
            }`,
            operatorLabel: tool.component_name ?? Operator.Tool,
          };
          issues.push(
            ...collectRetrievalBindingIssues(tool?.params, toolTarget, {
              memoryIds,
              staleDatasetIds,
            }),
          );
        }
      }
    }

    if (label === Operator.Extractor) {
      // Same always-on policy as the Agent model check: a template-created
      // pipeline ships an empty llm_id and must flag on load, not only after
      // edits.
      if (!form?.llm_id) {
        issues.push({
          ...target,
          type: CanvasIssueType.MissingRequired,
          messageKey: 'flow.extractorModelMissing',
        });
      }
    }

    if (label === Operator.Compiler) {
      // Same always-on policy as the Agent model check: an empty operator
      // must flag on load, not only after edits.
      const groupId = form?.compilation_template_group_id;
      if (!groupId) {
        issues.push({
          ...target,
          type: CanvasIssueType.MissingRequired,
          messageKey: 'knowledgeConfiguration.compilationTemplateRequired',
        });
      } else if (
        compilationTemplateGroupIds &&
        !compilationTemplateGroupIds.has(groupId)
      ) {
        issues.push({
          ...target,
          type: CanvasIssueType.InvalidVariable,
          messageKey: 'knowledgeConfiguration.compilationTemplateUnavailable',
        });
      }
    }

    if (
      label === Operator.Parser &&
      editedNodeFormIds.includes(node.id) &&
      !ParserFormSchema.safeParse(form).success
    ) {
      issues.push({
        ...target,
        type: CanvasIssueType.MissingRequired,
        messageKey: 'flow.nodeFormInvalid',
      });
    }

    if (
      label === Operator.Message &&
      memoryIds &&
      Array.isArray(form?.memory_ids) &&
      form.memory_ids.some((id: string) => !memoryIds.has(id))
    ) {
      issues.push({
        ...target,
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'flow.memoryUnavailable',
      });
    }

    if (
      modelValidIds &&
      typeof form?.llm_id === 'string' &&
      form.llm_id &&
      !modelValidIds.has(form.llm_id)
    ) {
      issues.push({
        ...target,
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'common.modelUnavailable',
      });
    }

    issues.push(...collectNodeReferenceIssues(node, target, ctx));
  }

  return issues;
}
