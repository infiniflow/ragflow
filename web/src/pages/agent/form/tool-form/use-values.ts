import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { Operator } from '../../constant';
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
      ? undefined
      : Array.isArray(formData?.kb_ids)
        ? formData.kb_ids
        : undefined;

    return legacyDatasetIds
      ? { ...formData, dataset_ids: legacyDatasetIds }
      : { ...formData };
  }, [
    clickedNodeId,
    clickedToolId,
    findUpstreamNodeById,
    getAgentToolById,
    initializeAgentToolValues,
  ]);

  return values;
}
