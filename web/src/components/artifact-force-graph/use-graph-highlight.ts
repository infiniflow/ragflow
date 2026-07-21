import { useCallback, useMemo, useState, type ComponentProps } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import { renderNodeLabel } from './node-label';
import {
  DefaultLinkWidth,
  DimmedAlpha,
  HighlightLinkWidth,
} from './node-style';
import { type ArtifactGraphLink, type ArtifactGraphNode } from './types';
import { withAlpha } from './utils';

type PaintNodeFn = NonNullable<
  ComponentProps<typeof ForceGraph2D>['nodeCanvasObject']
>;

export const useGraphHighlight = (getBaseLinkColor: () => string) => {
  const [hoverNode, setHoverNode] = useState<ArtifactGraphNode | null>(null);

  const highlightNodes = useMemo(() => {
    const nodes = new Set<ArtifactGraphNode>();
    if (hoverNode) {
      nodes.add(hoverNode);
      hoverNode.__neighbors?.forEach((neighbor) => nodes.add(neighbor));
    }
    return nodes;
  }, [hoverNode]);

  const highlightLinks = useMemo(
    () => new Set<ArtifactGraphLink>(hoverNode?.__links ?? []),
    [hoverNode],
  );

  const handleNodeHover = useCallback((node: ArtifactGraphNode | null) => {
    setHoverNode(node ?? null);
  }, []);

  const getNodeColor = useCallback(
    (node: ArtifactGraphNode) =>
      hoverNode && !highlightNodes.has(node)
        ? withAlpha(node.__color, DimmedAlpha)
        : node.__color,
    [hoverNode, highlightNodes],
  );

  const getLinkColor = useCallback(
    (link: ArtifactGraphLink) => {
      const baseColor = getBaseLinkColor();
      return hoverNode && !highlightLinks.has(link)
        ? withAlpha(baseColor, DimmedAlpha)
        : baseColor;
    },
    [getBaseLinkColor, hoverNode, highlightLinks],
  );

  const getLinkWidth = useCallback(
    (link: ArtifactGraphLink) =>
      highlightLinks.has(link) ? HighlightLinkWidth : DefaultLinkWidth,
    [highlightLinks],
  );

  const paintNode = useCallback<PaintNodeFn>(
    (node, ctx, globalScale) => {
      const dimmed =
        hoverNode !== null && !highlightNodes.has(node as ArtifactGraphNode);
      if (dimmed) {
        ctx.globalAlpha = DimmedAlpha;
      }
      renderNodeLabel(node, ctx, globalScale);
      if (dimmed) {
        ctx.globalAlpha = 1;
      }
    },
    [highlightNodes, hoverNode],
  );

  return {
    handleNodeHover,
    getNodeColor,
    getLinkColor,
    getLinkWidth,
    paintNode,
  };
};
