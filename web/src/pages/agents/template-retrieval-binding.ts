import { cloneDeep } from 'lodash';
import { Operator } from '@/constants/agent';
import { DSL } from '@/interfaces/database/agent';
import { RetrievalFrom } from '@/pages/agent/constant';

/**
 * Retrieval params live in two views of a DSL: the canvas graph
 * (`graph.nodes[].data.form` / `...tools[].params`) and the components the
 * runtime executes (`components[].obj.params` / `...tools[].params`). Both are
 * written on save, so binding helpers must keep the two views in sync.
 */
interface RetrievalParams {
  retrieval_from?: string;
  dataset_ids?: string[];
  memory_ids?: string[];
  kb_ids?: string[];
}

function walkRetrievalParams(
  dsl: DSL | Record<string, any>,
): Array<{ params: RetrievalParams }> {
  const locations: Array<{ params: RetrievalParams }> = [];
  const pushParams = (params: unknown) => {
    if (
      params &&
      typeof params === 'object' &&
      typeof (params as RetrievalParams).retrieval_from === 'string'
    ) {
      locations.push({ params: params as RetrievalParams });
    }
  };

  const graph = (dsl as Record<string, any>)?.graph;
  if (Array.isArray(graph?.nodes)) {
    for (const node of graph.nodes) {
      const label = node?.data?.label;
      const form = node?.data?.form;
      if (label === Operator.Retrieval) {
        pushParams(form);
      } else if (label === Operator.Agent) {
        for (const tool of form?.tools ?? []) {
          if (tool?.component_name === Operator.Retrieval) {
            pushParams(tool?.params);
          }
        }
      }
    }
  }

  const components = (dsl as Record<string, any>)?.components;
  if (components && typeof components === 'object') {
    for (const component of Object.values(components)) {
      const obj = (component as Record<string, any>)?.obj;
      if (obj?.component_name === Operator.Retrieval) {
        pushParams(obj?.params);
      } else if (obj?.component_name === Operator.Agent) {
        for (const tool of obj?.params?.tools ?? []) {
          if (tool?.component_name === Operator.Retrieval) {
            pushParams(tool?.params);
          }
        }
      }
    }
  }

  return locations;
}

export interface RetrievalBindingCount {
  datasetCount: number;
  memoryCount: number;
}

/**
 * Counts retrieval steps that explicitly source from a dataset/memory but do
 * not carry any binding yet. Such steps fail at runtime with a
 * `dataset_ids`/`memory_ids is required` error.
 */
export function countUnboundRetrieval(
  dsl: DSL | Record<string, any> | undefined,
): RetrievalBindingCount {
  let datasetCount = 0;
  let memoryCount = 0;
  if (!dsl) {
    return { datasetCount, memoryCount };
  }
  for (const { params } of walkRetrievalParams(dsl)) {
    if (params.retrieval_from === RetrievalFrom.Dataset) {
      const ids = params.dataset_ids ?? params.kb_ids;
      if (!Array.isArray(ids) || ids.length === 0) {
        datasetCount++;
      }
    } else if (params.retrieval_from === RetrievalFrom.Memory) {
      const ids = params.memory_ids;
      if (!Array.isArray(ids) || ids.length === 0) {
        memoryCount++;
      }
    }
  }
  return { datasetCount, memoryCount };
}

/**
 * Applies the selected dataset/memory ids to every unbound retrieval step in
 * the template DSL (both the graph and the components views). Steps that
 * already carry ids are left untouched so users can fine-tune each retrieval
 * in the canvas later.
 */
export function bindUnboundRetrieval(
  dsl: DSL | Record<string, any> | undefined,
  datasetIds: string[],
  memoryIds: string[],
): DSL | undefined {
  if (!dsl) {
    return undefined;
  }
  const next = cloneDeep(dsl) as Record<string, any>;
  for (const { params } of walkRetrievalParams(next)) {
    if (
      params.retrieval_from === RetrievalFrom.Dataset &&
      datasetIds.length > 0
    ) {
      const ids = params.dataset_ids ?? params.kb_ids;
      if (!Array.isArray(ids) || ids.length === 0) {
        params.dataset_ids = datasetIds;
        delete params.kb_ids;
      }
    }
    if (
      params.retrieval_from === RetrievalFrom.Memory &&
      memoryIds.length > 0
    ) {
      const ids = params.memory_ids;
      if (!Array.isArray(ids) || ids.length === 0) {
        params.memory_ids = memoryIds;
      }
    }
  }
  return next as DSL;
}
