import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';

export const EntityNodeColor = '#00BEB4';
export const ConceptNodeColor = '#4CACFF';

export const MinNodeRadius = 4;
export const MaxNodeRadius = 14;

export const DefaultLinkWidth = 1;
export const HighlightLinkWidth = 2;

export const DimmedAlpha = 0.2;

export const getNodeColor = (node: IArtifactGraphEntity): string => {
  if (node.type === 'entity') return EntityNodeColor;
  if (node.type === 'concept') return ConceptNodeColor;
  return EntityNodeColor;
};

export const getNodeRadius = (
  node: IArtifactGraphEntity,
  minWeight: number,
  maxWeight: number,
): number => {
  const weight = node.weight ?? 0;
  if (maxWeight <= minWeight) return MinNodeRadius;
  const clamped = Math.max(minWeight, Math.min(maxWeight, weight));
  const t = (clamped - minWeight) / (maxWeight - minWeight);
  return MinNodeRadius + t * (MaxNodeRadius - MinNodeRadius);
};
