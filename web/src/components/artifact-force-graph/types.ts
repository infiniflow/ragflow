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
import { type LinkObject, type NodeObject } from 'react-force-graph-2d';

export type ArtifactGraphNode = NodeObject<IArtifactGraphEntity> & {
  id: string;
  __color: string;
  __radius: number;
  __neighbors?: ArtifactGraphNode[];
  __links?: ArtifactGraphLink[];
};

export type ArtifactGraphLink = LinkObject<
  ArtifactGraphNode,
  { source: string; target: string; type?: string }
>;

export interface ArtifactForceGraphProps<TNodeValue = IArtifactGraphEntity> {
  data?: IArtifactGraph;
  show?: boolean;
  onNodeClick?: (node: TNodeValue) => void;
  mapNodeToValue?: (node: IArtifactGraphEntity) => TNodeValue;
  getNodeId?: (node: IArtifactGraphEntity) => string;
  getNodeColor?: (node: IArtifactGraphEntity) => string;
  getNodeRadius?: (
    node: IArtifactGraphEntity,
    minWeight: number,
    maxWeight: number,
  ) => number;
  /** Controlled highlighted node id (same id space as getNodeId output, slug by default); real hover takes precedence over this value */
  highlightNodeId?: string | null;
  /** Total entity count in the dataset; rendered together with returnedEntities as an overlay badge */
  totalEntities?: number;
  /** Entity count present in the current graph data (may be a sampled subset of totalEntities) */
  returnedEntities?: number;
}
