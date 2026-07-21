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
  { source: string; target: string }
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
}
