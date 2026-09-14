import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { Operator, RetrievalFrom } from '../constant';

/**
 * A Retrieval node sourcing from datasets must name at least one dataset,
 * and one sourcing from memories must name at least one memory; the backend
 * otherwise rejects the run with a `dataset_ids`/`memory_ids is required`
 * error that only surfaces at runtime. The same applies to Retrieval tools
 * embedded in an Agent node, which is how template-built agents usually
 * bind retrieval. Only forms/tools carrying the canonical `dataset_ids`/
 * `memory_ids` field are checked, so legacy DSLs still keyed on `kb_ids`
 * cannot wedge saving. Binding emptiness is asserted directly instead of
 * re-parsing the whole form schema: template-era params can miss unrelated
 * newer fields (e.g. `rerank_candidates_count`), and zod skips refinements
 * when the base shape fails — such a stale field must never be reported as
 * a missing dataset after the user picked one at creation.
 */
const toInvalidRetrieval = (
  params: Record<string, any>,
): { memoryMissing: boolean } | undefined => {
  if (
    params?.retrieval_from === RetrievalFrom.Dataset &&
    Array.isArray(params?.dataset_ids) &&
    params.dataset_ids.length === 0
  ) {
    return { memoryMissing: false };
  }
  if (
    params?.retrieval_from === RetrievalFrom.Memory &&
    Array.isArray(params?.memory_ids) &&
    params.memory_ids.length === 0
  ) {
    return { memoryMissing: true };
  }
  return undefined;
};

export function findInvalidRetrievalBinding(nodes: RAGFlowNodeType[]):
  | {
      node: RAGFlowNodeType;
      messageKey: string;
    }
  | undefined {
  for (const node of nodes) {
    if (node.data?.label === Operator.Retrieval) {
      const invalid = toInvalidRetrieval(node.data?.form);
      if (invalid) {
        return {
          node,
          messageKey: invalid.memoryMissing
            ? 'flow.retrievalMemoryMissing'
            : 'flow.retrievalDatasetMissing',
        };
      }
    }
    if (node.data?.label === Operator.Agent) {
      const tools = node.data?.form?.tools;
      if (Array.isArray(tools)) {
        for (const tool of tools) {
          if (tool?.component_name !== Operator.Retrieval) {
            continue;
          }
          const invalid = toInvalidRetrieval(tool?.params);
          if (invalid) {
            return {
              node,
              messageKey: invalid.memoryMissing
                ? 'flow.retrievalMemoryMissing'
                : 'flow.retrievalDatasetMissing',
            };
          }
        }
      }
    }
  }
  return undefined;
}
