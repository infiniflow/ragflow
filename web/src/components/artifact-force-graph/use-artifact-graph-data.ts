/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
import { type ArtifactGraphLink, type ArtifactGraphNode } from './types';

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

    const nodes: ArtifactGraphNode[] = entities.map((entity) => ({
      ...entity,
      id: getNodeId(entity),
      __color: getNodeColor(entity),
      __radius: getNodeRadius(entity, minWeight, maxWeight),
    }));

    const nodesBySlug = new Map(nodes.map((node) => [node.slug, node]));
    // ForceLink throws when a relation references an entity omitted from the
    // current graph slice, so only pass relations with two loaded endpoints.
    const links: ArtifactGraphLink[] = (data.relations || []).flatMap(
      (relation) => {
        const source = nodesBySlug.get(relation.from);
        const target = nodesBySlug.get(relation.to);
        if (!source || !target) return [];
        return [
          {
            source: source.id,
            target: target.id,
            type: relation.type,
          },
        ];
      },
    );

    // cross-link node objects for hover highlighting
    const nodesById = new Map(nodes.map((node) => [node.id, node]));
    links.forEach((link) => {
      const a = nodesById.get(link.source);
      const b = nodesById.get(link.target);
      if (!a || !b) return;
      (a.__neighbors ??= []).push(b);
      (b.__neighbors ??= []).push(a);
      (a.__links ??= []).push(link);
      (b.__links ??= []).push(link);
    });

    return { nodes, links };
  }, [data, getNodeColor, getNodeId, getNodeRadius]);
};
