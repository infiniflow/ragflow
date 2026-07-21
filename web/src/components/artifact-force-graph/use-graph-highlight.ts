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

export const useGraphHighlight = (
  getBaseLinkColor: () => string,
  pinnedNode?: ArtifactGraphNode | null,
) => {
  const [hoverNode, setHoverNode] = useState<ArtifactGraphNode | null>(null);

  // Real hover takes precedence; falls back to the externally controlled pinned node when not hovering
  const activeNode = hoverNode ?? pinnedNode ?? null;

  const highlightNodes = useMemo(() => {
    const nodes = new Set<ArtifactGraphNode>();
    if (activeNode) {
      nodes.add(activeNode);
      activeNode.__neighbors?.forEach((neighbor) => nodes.add(neighbor));
    }
    return nodes;
  }, [activeNode]);

  const highlightLinks = useMemo(
    () => new Set<ArtifactGraphLink>(activeNode?.__links ?? []),
    [activeNode],
  );

  const handleNodeHover = useCallback((node: ArtifactGraphNode | null) => {
    setHoverNode(node ?? null);
  }, []);

  const getNodeColor = useCallback(
    (node: ArtifactGraphNode) =>
      activeNode && !highlightNodes.has(node)
        ? withAlpha(node.__color, DimmedAlpha)
        : node.__color,
    [activeNode, highlightNodes],
  );

  const getLinkColor = useCallback(
    (link: ArtifactGraphLink) => {
      const baseColor = getBaseLinkColor();
      return activeNode && !highlightLinks.has(link)
        ? withAlpha(baseColor, DimmedAlpha)
        : baseColor;
    },
    [getBaseLinkColor, activeNode, highlightLinks],
  );

  const getLinkWidth = useCallback(
    (link: ArtifactGraphLink) =>
      highlightLinks.has(link) ? HighlightLinkWidth : DefaultLinkWidth,
    [highlightLinks],
  );

  const paintNode = useCallback<PaintNodeFn>(
    (node, ctx, globalScale) => {
      const dimmed =
        activeNode !== null && !highlightNodes.has(node as ArtifactGraphNode);
      if (dimmed) {
        ctx.globalAlpha = DimmedAlpha;
      }
      renderNodeLabel(node, ctx, globalScale);
      if (dimmed) {
        ctx.globalAlpha = 1;
      }
    },
    [highlightNodes, activeNode],
  );

  return {
    handleNodeHover,
    getNodeColor,
    getLinkColor,
    getLinkWidth,
    paintNode,
  };
};
