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

  const values = useMemo(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    const tool = getAgentToolById(clickedToolId, agentNode!);
    const formData = tool?.params;

    if (isEmpty(formData)) {
      const defaultValues = initializeAgentToolValues(
        (tool?.component_name || clickedNodeId) as Operator,
      );

      return defaultValues;
    }

    return {
      ...formData,
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
