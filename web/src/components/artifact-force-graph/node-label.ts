import { type ComponentProps } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import { MinNodeRadius } from './node-style';
import { type ArtifactGraphNode } from './types';

export const renderNodeLabel: NonNullable<
  ComponentProps<typeof ForceGraph2D>['nodeCanvasObject']
> = (node, ctx) => {
  const graphNode = node as ArtifactGraphNode;
  const label = graphNode.name;
  const radius = graphNode.__radius ?? MinNodeRadius;
  const fontSize = 2;
  ctx.font = `${fontSize}px Sans-Serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';

  const textSecondary = getComputedStyle(document.documentElement)
    .getPropertyValue('--text-secondary')
    .trim();
  ctx.fillStyle = `rgb(${textSecondary})`;

  if (typeof graphNode.x === 'number' && typeof graphNode.y === 'number') {
    ctx.fillText(label, graphNode.x, graphNode.y + radius - 2);
  }
};
