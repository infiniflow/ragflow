import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { Operator, RetrievalFrom } from '../../constant';
import { useAgentToolInitialValues } from '../../hooks/use-agent-tool-initial-values';
import useGraphStore from '../../store';

export enum SearchDepth {
  Basic = 'basic',
  Advanced = 'advanced',
}

export enum Topic {
  News = 'news',
  General = 'general',
}

export function useValues() {
  const {
    clickedToolId,
    clickedNodeId,
    findUpstreamNodeById,
    getAgentToolById,
  } = useGraphStore();

  const { initializeAgentToolValues } = useAgentToolInitialValues();

  const values = useMemo<Record<string, any>>(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    const tool = getAgentToolById(clickedToolId, agentNode!);
    const formData = tool?.params;

    if (isEmpty(formData)) {
      const defaultValues = initializeAgentToolValues(
        (tool?.component_name || clickedNodeId) as Operator,
      );

      return defaultValues;
    }

    // DSLs predating the canonical `dataset_ids` field key retrieval
    // bindings under the legacy `kb_ids` key. Fold it into the canonical
    // field so the edited tool persists `dataset_ids` and save/run
    // validation can see the binding.
    const legacyDatasetIds = Array.isArray(formData?.dataset_ids)
      ? formData.dataset_ids
      : Array.isArray(formData?.kb_ids)
        ? formData.kb_ids
        : undefined;

    // Both bindings stay concrete arrays and `retrieval_from` always resolves,
    // so an unbound tool is written back with the keys the canvas checklist
    // and the form schema inspect instead of dropping them.
    return {
      ...formData,
      dataset_ids: legacyDatasetIds ?? [],
      memory_ids: formData?.memory_ids ?? [],
      retrieval_from: formData?.retrieval_from ?? RetrievalFrom.Dataset,
    };
  }, [
    clickedNodeId,
    clickedToolId,
    findUpstreamNodeById,
    getAgentToolById,
    initializeAgentToolValues,
  ]);

  return values;
}
