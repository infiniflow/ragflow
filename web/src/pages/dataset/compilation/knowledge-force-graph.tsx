import { useIsDarkTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';
import isEmpty from 'lodash/isEmpty';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ForceGraph2D, { LinkObject, NodeObject } from 'react-force-graph-2d';

import { buildNodesAndCombos } from '../knowledge-graph/util';

interface IProps {
  data: any;
  show: boolean;
}

interface NodeData {
  rank?: number;
  entity_type?: string;
  description?: string;
  weight?: number;
  communities?: string[];
  combo?: string;
  __bckgDimensions?: [number, number];
}

interface LinkData {
  weight?: number;
}

type GNode = NodeObject<NodeData>;
type GLink = LinkObject<NodeData, LinkData>;

const NodeColorPalette = [
  '#5B8FF9',
  '#5AD8A6',
  '#F6BD16',
  '#E8684A',
  '#6DC8EC',
  '#9270CA',
  '#FF9D4D',
  '#269A99',
  '#FF99C3',
  '#A9A9A9',
];

const getNodeSize = (node: GNode) => {
  const size = 180 + ((node.rank as number) || 0) * 5;
  return Math.min(size, 300) / 2;
};

const getLinkWidth = (link: GLink) => {
  const weight = Number(link.weight) || 2;
  return Math.min(weight * 4, 8);
};

export function KnowledgeForceGraph({ data, show }: IProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<any>(null);
  const isDark = useIsDarkTheme();
  const [hoverNode, setHoverNode] = useState<GNode | null>(null);
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });
  const forceConfiguredRef = useRef(false);

  const graphData = useMemo(() => {
    if (isEmpty(data)) {
      return { nodes: [], links: [] };
    }

    const rawNodes = data.nodes || [];
    const rawEdges = data.edges || [];
    const { nodes: comboNodes } = buildNodesAndCombos(rawNodes);

    const entityTypes = Array.from(
      new Set(rawNodes.map((n: any) => n.entity_type).filter(Boolean)),
    );

    const typeColorMap = entityTypes.reduce<Record<string, string>>(
      (pre, cur, idx) => {
        pre[cur as string] = NodeColorPalette[idx % NodeColorPalette.length];
        return pre;
      },
      {},
    );

    const nodes = comboNodes.map((n: any) => ({
      ...n,
      color: typeColorMap[n.entity_type as string] || NodeColorPalette[0],
    }));

    const links = rawEdges.map((e: any) => ({
      source: e.source,
      target: e.target,
      weight: e.weight,
    }));

    return { nodes, links };
  }, [data]);

  // Compute the set of node IDs directly connected to the hovered node (degree: 1)
  const connectedNodeIds = useMemo(() => {
    if (!hoverNode) return new Set<string>();
    const ids = new Set<string>();
    ids.add(hoverNode.id as string);
    graphData.links.forEach((link: any) => {
      const sourceId =
        typeof link.source === 'object' ? link.source.id : link.source;
      const targetId =
        typeof link.target === 'object' ? link.target.id : link.target;
      if (sourceId === hoverNode.id) ids.add(targetId as string);
      if (targetId === hoverNode.id) ids.add(sourceId as string);
    });
    return ids;
  }, [hoverNode, graphData.links]);

  useEffect(() => {
    if (!containerRef.current || !show) return;

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) {
        const { width, height } = entry.contentRect;
        setDimensions({ width, height });
      }
    });

    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, [show]);

  // Reset the flag when graphData changes, so forces are reconfigured for new data
  useEffect(() => {
    forceConfiguredRef.current = false;
  }, [graphData]);

  useEffect(() => {
    if (
      fgRef.current &&
      !isEmpty(graphData.nodes) &&
      !forceConfiguredRef.current
    ) {
      forceConfiguredRef.current = true;
      // D3 force configuration
      const linkForce = fgRef.current.d3Force('link');
      if (linkForce) {
        linkForce.distance((link: GLink) => {
          const source =
            typeof link.source === 'object'
              ? (link.source as GNode)
              : (graphData.nodes.find(
                  (n: GNode) => n.id === link.source,
                ) as GNode);
          const target =
            typeof link.target === 'object'
              ? (link.target as GNode)
              : (graphData.nodes.find(
                  (n: GNode) => n.id === link.target,
                ) as GNode);
          const sourceSize = source ? getNodeSize(source) : 16;
          const targetSize = target ? getNodeSize(target) : 16;
          return sourceSize + targetSize + 200;
        });
      }

      const chargeForce = fgRef.current.d3Force('charge');
      if (chargeForce) {
        chargeForce.strength(-1000);
      }

      const timer = setTimeout(() => {
        fgRef.current?.zoomToFit(400, 50);
      }, 600);
      return () => clearTimeout(timer);
    }
  }, [graphData, dimensions]);

  const getNodeLabel = useCallback((node: GNode) => {
    const parts = [`<h3 class="font-semibold">${node.id}</h3>`];

    if (node.entity_type) {
      parts.push(
        `<div class="flex items-center gap-[.5ch]"><dt><b>Entity type: </b></dt><dd>${node.entity_type}</dd></div>`,
      );
    }

    if (node.weight) {
      parts.push(
        `<div class="flex items-center gap-[.5ch]"><dt><b>Weight: </b></dt><dd>${node.weight}</dd></div>`,
      );
    }

    if (node.description) {
      parts.push(
        `<p class="text-xs mt-1 max-w-[240px]">${node.description}</p>`,
      );
    }

    return `<dl class="mb-1 empty:hidden">${parts.join('')}</dl>`;
  }, []);

  const getLinkLabel = useCallback((link: GLink) => {
    const sourceId =
      typeof link.source === 'object' ? link.source.id : link.source;
    const targetId =
      typeof link.target === 'object' ? link.target.id : link.target;
    return `${sourceId} → ${targetId}`;
  }, []);

  const nodeCanvasObject = useCallback(
    (node: GNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const size = getNodeSize(node);
      const label = (node.id as string) || '';
      const fontSize = Math.max(12 / globalScale, Math.min(size / 2, 40));

      // Dim non-connected nodes when hovering
      const isDimmed = hoverNode && !connectedNodeIds.has(node.id as string);
      ctx.globalAlpha = isDimmed ? 0.1 : 1;

      // Node circle (solid fill by entity_type color)
      const nodeColor = (node as any).color || NodeColorPalette[0];
      ctx.beginPath();
      ctx.arc(node.x!, node.y!, size, 0, 2 * Math.PI);
      ctx.fillStyle = nodeColor;
      ctx.fill();
      ctx.strokeStyle = isDark ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.2)';
      ctx.lineWidth = Math.max(2 / globalScale, 3);
      ctx.stroke();

      // Only show label on hover
      if (hoverNode?.id === node.id) {
        ctx.font = `${fontSize}px Sans-Serif`;
        const textWidth = ctx.measureText(label).width;
        const padding = fontSize * 0.4;
        const bckgDimensions: [number, number] = [
          textWidth + padding * 2,
          fontSize + padding * 2,
        ];

        // Label background
        ctx.fillStyle = isDark
          ? 'rgba(0, 0, 0, 0.1)'
          : 'rgba(255, 255, 255, 0.1)';
        ctx.fillRect(
          node.x! - bckgDimensions[0] / 2,
          node.y! + size + fontSize / 2,
          bckgDimensions[0],
          bckgDimensions[1],
        );

        // Label text
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = isDark ? '#fff' : '#000';
        ctx.fillText(label, node.x!, node.y! + size + fontSize);

        (node as any).__bckgDimensions = bckgDimensions;
      }

      ctx.globalAlpha = 1;
    },
    [isDark, hoverNode, connectedNodeIds],
  );

  const nodePointerAreaPaint = useCallback(
    (node: GNode, color: string, ctx: CanvasRenderingContext2D) => {
      const size = getNodeSize(node);
      ctx.fillStyle = color;
      ctx.beginPath();
      ctx.arc(node.x!, node.y!, size, 0, 2 * Math.PI);
      ctx.fill();
    },
    [],
  );

  const linkCanvasObject = useCallback(
    (link: GLink, ctx: CanvasRenderingContext2D) => {
      const source =
        typeof link.source === 'object'
          ? (link.source as GNode)
          : (graphData.nodes.find((n: GNode) => n.id === link.source) as GNode);
      const target =
        typeof link.target === 'object'
          ? (link.target as GNode)
          : (graphData.nodes.find((n: GNode) => n.id === link.target) as GNode);

      if (!source || !target) return;

      const isDimmed =
        hoverNode &&
        !(
          connectedNodeIds.has(source.id as string) &&
          connectedNodeIds.has(target.id as string)
        );
      ctx.globalAlpha = isDimmed ? 0.1 : 1;

      const lineWidth = getLinkWidth(link);
      ctx.lineWidth = lineWidth;
      ctx.strokeStyle = isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.5)';

      ctx.beginPath();
      ctx.moveTo(source.x!, source.y!);
      ctx.lineTo(target.x!, target.y!);
      ctx.stroke();
      ctx.globalAlpha = 1;
    },
    [graphData.nodes, hoverNode, connectedNodeIds, isDark],
  );

  const handleNodeHover = useCallback((node: GNode | null) => {
    setHoverNode(node);
  }, []);

  const backgroundColor = isDark ? 'rgba(0,0,0,0)' : 'rgba(255,255,255,0)';

  return (
    <div
      ref={containerRef}
      className={cn('flex-1 min-h-0', !show && 'hidden')}
      aria-haspopup="true"
    >
      {dimensions.width > 0 && dimensions.height > 0 && (
        <ForceGraph2D
          ref={fgRef}
          width={dimensions.width}
          height={dimensions.height}
          graphData={graphData}
          backgroundColor={backgroundColor}
          nodeLabel={getNodeLabel}
          linkLabel={getLinkLabel}
          nodeRelSize={1}
          nodeCanvasObject={nodeCanvasObject}
          nodePointerAreaPaint={nodePointerAreaPaint}
          linkCanvasObject={linkCanvasObject}
          linkWidth={getLinkWidth}
          linkColor={() =>
            isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.5)'
          }
          onNodeHover={handleNodeHover}
          enableNodeDrag
          enableZoomInteraction
          enablePanInteraction
          warmupTicks={50}
          cooldownTicks={100}
        />
      )}
    </div>
  );
}

export default KnowledgeForceGraph;
