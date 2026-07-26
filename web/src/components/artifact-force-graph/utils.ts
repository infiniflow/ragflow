import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';

export const defaultMapNodeToValue = <TNode extends IArtifactGraphEntity>(
  node: TNode,
): TNode => node;

export const withAlpha = (color: string, alpha: number): string => {
  if (color.length === 7 && color.startsWith('#')) {
    return (
      color +
      Math.round(alpha * 255)
        .toString(16)
        .padStart(2, '0')
    );
  }
  if (color.startsWith('rgb(')) {
    return `rgba(${color.slice(4, -1)}, ${alpha})`;
  }
  return color;
};
