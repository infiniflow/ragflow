import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { Operator } from '../../constant';
import { useAgentToolInitialValues } from '../../hooks/use-agent-tool-initial-values';
import useGraphStore from '../../store';
import { getAgentNodeTools } from '../../utils';

export enum SearchDepth {
  Basic = 'basic',
  Advanced = 'advanced',
}

export enum Topic {
  News = 'news',
  General = 'general',
}

export function useValues() {
  const { clickedToolId, clickedNodeId, findUpstreamNodeById } = useGraphStore(
    (state) => state,
  );
  const { initializeAgentToolValues } = useAgentToolInitialValues();

  const values = useMemo(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    const tools = getAgentNodeTools(agentNode);

    const formData = tools.find(
      (x) => x.component_name === clickedToolId,
    )?.params;

    if (isEmpty(formData)) {
      const defaultValues = initializeAgentToolValues(
        clickedNodeId as Operator,
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
    initializeAgentToolValues,
  ]);

  return values;
}
