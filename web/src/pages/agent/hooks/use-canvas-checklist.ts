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

import { ModelTypeMap } from '@/components/model-tree-select';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { useFetchAllCompilationTemplateGroups } from '@/hooks/use-compilation-template-group-request';
import { useStaleDatasetIds } from '@/hooks/use-knowledge-request';
import { useModelValidIds } from '@/hooks/use-llm-request';
import { useFetchAllMemoryList } from '@/hooks/use-memory-request';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { useDebounce } from 'ahooks';
import { useMemo } from 'react';
import { Operator } from '../constant';
import useGraphStore from '../store';
import {
  CanvasChecklistInputs,
  collectCanvasIssues,
} from '../utils/canvas-checklist';

// Every dataset binding on the canvas, including Agent-embedded Retrieval
// tools, so a single lookup settles staleness for all of them. Variable
// references mixed into the arrays are dropped inside useStaleDatasetIds.
function collectAllDatasetIds(nodes: RAGFlowNodeType[]): string[] {
  const ids: string[] = [];
  for (const node of nodes) {
    const form = node.data?.form;
    if (
      node.data?.label === Operator.Retrieval &&
      Array.isArray(form?.dataset_ids)
    ) {
      ids.push(...form.dataset_ids);
    }
    if (node.data?.label === Operator.Agent && Array.isArray(form?.tools)) {
      for (const tool of form.tools) {
        if (
          tool?.component_name === Operator.Retrieval &&
          Array.isArray(tool?.params?.dataset_ids)
        ) {
          ids.push(...tool.params.dataset_ids);
        }
      }
    }
  }
  return ids;
}

/**
 * Real-time canvas issues for the checklist panel, debounced so dragging a
 * node does not recompute every frame. The checklist only reports issues to
 * the user. Run checks blocking rules synchronously, without this debounce.
 */
export function useCanvasChecklist({
  resourceTenantId,
  ownerUserId,
  canvasID,
}: {
  resourceTenantId?: string;
  ownerUserId?: string | null;
  canvasID?: string;
} = {}) {
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  const editedNodeFormIds = useGraphStore((state) => state.editedNodeFormIds);

  const { data: agentDetail } = useFetchAgent();
  const {
    data: memoryList,
    isLoading: memoryLoading,
    isError: memoryError,
  } = useFetchAllMemoryList(ownerUserId);
  // Model, template-group, and memory references use the canvas owner's
  // access scope. Dataset references are scoped separately by canvasID.
  const { validIds: modelValidIds, isFetched: modelsFetched } =
    useModelValidIds(ModelTypeMap.llm_id, resourceTenantId, ownerUserId);
  const {
    validIds: categorizeModelValidIds,
    isFetched: categorizeModelsFetched,
  } = useModelValidIds(
    ['chat', 'vlm', 'decision'],
    resourceTenantId,
    ownerUserId,
  );
  const {
    groups: templateGroups,
    isFetched: templateGroupsFetched,
    isError: templateGroupsError,
  } = useFetchAllCompilationTemplateGroups(ownerUserId);
  const datasetIds = useMemo(() => collectAllDatasetIds(nodes), [nodes]);
  const { staleDatasetIds, settled: datasetsSettled } = useStaleDatasetIds(
    datasetIds,
    canvasID,
    ownerUserId,
  );

  const inputs: CanvasChecklistInputs = useMemo(
    () => ({
      nodes,
      edges,
      editedNodeFormIds,
      // useFetchAllMemoryList only fetches the first page (page_size: 100) —
      // a full page means some memories live beyond it, so hold off instead of
      // misreporting them as deleted.
      memoryIds:
        !memoryLoading && !memoryError && (memoryList?.length ?? 0) < 100
          ? new Set(memoryList?.map((x) => x.id))
          : undefined,
      modelValidIds: modelsFetched ? modelValidIds : undefined,
      categorizeModelValidIds: categorizeModelsFetched
        ? categorizeModelValidIds
        : undefined,
      staleDatasetIds: datasetsSettled ? staleDatasetIds : undefined,
      // The groups query fetches only the first page (page_size: 100) — a
      // full page means some groups live beyond it, so hold off instead of
      // misreporting them as inaccessible. Gate on isFetched: initialData
      // keeps isLoading from ever firing.
      compilationTemplateGroupIds:
        templateGroupsFetched &&
        !templateGroupsError &&
        templateGroups.length < 100
          ? new Set(templateGroups.map((x) => x.id))
          : undefined,
      variables: agentDetail?.dsl?.variables,
    }),
    [
      nodes,
      edges,
      editedNodeFormIds,
      memoryLoading,
      memoryError,
      memoryList,
      modelsFetched,
      modelValidIds,
      categorizeModelsFetched,
      categorizeModelValidIds,
      datasetsSettled,
      staleDatasetIds,
      templateGroups,
      templateGroupsFetched,
      templateGroupsError,
      agentDetail?.dsl?.variables,
    ],
  );

  const issues = useMemo(() => collectCanvasIssues(inputs), [inputs]);
  const debouncedIssues = useDebounce(issues, { wait: 300 });

  return { issues: debouncedIssues };
}
