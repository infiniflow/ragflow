import {
  type IArtifactGraph,
  type IArtifactGraphEntity,
} from '@/interfaces/database/dataset';
import isEmpty from 'lodash/isEmpty';
import { useMemo } from 'react';
import { type GraphData } from 'react-force-graph-2d';
import {
  getNodeColor as defaultGetNodeColor,
  getNodeRadius as defaultGetNodeRadius,
} from './node-style';
import { type ArtifactGraphNode } from './types';

export interface UseArtifactGraphDataOptions {
  data?: IArtifactGraph;
  getNodeId?: (node: IArtifactGraphEntity) => string;
  getNodeColor?: (node: IArtifactGraphEntity) => string;
  getNodeRadius?: (
    node: IArtifactGraphEntity,
    minWeight: number,
    maxWeight: number,
  ) => number;
}

export const useArtifactGraphData = ({
  data,
  getNodeId = (node) => node.slug,
  getNodeColor = defaultGetNodeColor,
  getNodeRadius = defaultGetNodeRadius,
}: UseArtifactGraphDataOptions) => {
  return useMemo<
    GraphData<ArtifactGraphNode, { source: string; target: string }>
  >(() => {
    if (isEmpty(data) || !data) {
      return { nodes: [], links: [] };
    }

    const entities = data.entities || [];
    const weights = entities.map((entity) => entity.weight ?? 0);
    const minWeight = Math.min(0, ...weights);
    const maxWeight = Math.max(0, ...weights);

    const nodes = entities.map((entity) => ({
      ...entity,
      id: getNodeId(entity),
      __color: getNodeColor(entity),
      __radius: getNodeRadius(entity, minWeight, maxWeight),
    }));

    const links = (data.relations || []).map((relation) => ({
      source: relation.from,
      target: relation.to,
    }));

    return { nodes, links };
  }, [data, getNodeColor, getNodeId, getNodeRadius]);
};
