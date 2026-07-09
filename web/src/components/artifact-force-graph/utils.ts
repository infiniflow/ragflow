import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { type ComponentProps } from 'react';
import ForceGraph2D from 'react-force-graph-2d';

export const renderNodeLabel: NonNullable<
  ComponentProps<typeof ForceGraph2D>['nodeCanvasObject']
> = (node, ctx) => {
  const label = node.name;
  const fontSize = 2;
  ctx.font = `${fontSize}px Sans-Serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';

  const textSecondary = getComputedStyle(document.documentElement)
    .getPropertyValue('--text-secondary')
    .trim();
  ctx.fillStyle = `rgb(${textSecondary})`;

  if (node.x && node.y) {
    ctx.fillText(label, node.x, node.y + 5);
  }
};

export const defaultMapNodeToValue = <TNode extends IArtifactGraphEntity>(
  node: TNode,
): TNode => node;
