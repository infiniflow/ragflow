import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { DefaultAgentToolValuesMap } from '../../constant';
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

  const values = useMemo(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    const tools = getAgentNodeTools(agentNode);

    const formData = tools.find(
      (x) => x.component_name === clickedToolId,
    )?.params;

    if (isEmpty(formData)) {
      const defaultValues =
        DefaultAgentToolValuesMap[
          clickedToolId as keyof typeof DefaultAgentToolValuesMap
        ];

      return defaultValues;
    }

    return {
      ...formData,
    };
  }, [clickedNodeId, clickedToolId, findUpstreamNodeById]);

  return values;
}
