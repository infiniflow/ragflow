import {
  type IArtifactGraph,
  type IArtifactGraphEntity,
} from '@/interfaces/database/dataset';

export interface ArtifactForceGraphProps<TNodeValue = IArtifactGraphEntity> {
  data?: IArtifactGraph;
  show?: boolean;
  onNodeClick?: (node: TNodeValue) => void;
  mapNodeToValue?: (node: IArtifactGraphEntity) => TNodeValue;
  getNodeId?: (node: IArtifactGraphEntity) => string;
}
