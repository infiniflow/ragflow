import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';

export const defaultMapNodeToValue = <TNode extends IArtifactGraphEntity>(
  node: TNode,
): TNode => node;
