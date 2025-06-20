import { AgentGlobals } from '@/constants/agent';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import useGraphStore from '../../store';
import { convertToObjectArray, getAgentNodeTools } from '../../utils';

export enum SearchDepth {
  Basic = 'basic',
  Advanced = 'advanced',
}

export enum Topic {
  News = 'news',
  General = 'general',
}

export const defaultValues = {
  api_key: '',
  query: AgentGlobals.SysQuery,
  search_depth: SearchDepth.Basic,
  topic: Topic.General,
  max_results: 5,
  days: 7,
  include_answer: false,
  include_raw_content: true,
  include_images: false,
  include_image_descriptions: false,
  include_domains: [],
  exclude_domains: [],
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: {},
      type: 'Object',
    },
  },
};

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
      return defaultValues;
    }

    return {
      ...formData,
      include_domains: convertToObjectArray(formData.include_domains),
      exclude_domains: convertToObjectArray(formData.exclude_domains),
    };
  }, [clickedNodeId, clickedToolId, findUpstreamNodeById]);

  return values;
}
