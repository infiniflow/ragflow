import { isEmpty } from 'lodash';
import { useMemo } from 'react';
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

const defaultValues = {
  query: '',
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
    };
  }, [clickedNodeId, clickedToolId, findUpstreamNodeById]);

  return values;
}
