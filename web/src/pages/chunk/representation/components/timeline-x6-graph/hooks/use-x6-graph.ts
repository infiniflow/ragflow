import { DagreLayout } from '@antv/layout';
import { Graph, type EdgeMetadata, type NodeMetadata } from '@antv/x6';
import { useEffect, useRef } from 'react';

import { type TimelineNodeValue, type TimelineX6GraphProps } from '../types';

export function useX6Graph(
  containerRef: React.RefObject<HTMLDivElement | null>,
  data: TimelineX6GraphProps['data'],
  onNodeClick?: (node: TimelineNodeValue) => void,
) {
  const graphRef = useRef<Graph | null>(null);

  // Initialize the X6 graph instance once.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const graph = new Graph({
      container,
      autoResize: true,
      background: { color: 'transparent' },
      grid: false,
      panning: { enabled: true },
      mousewheel: { enabled: true },
      scaling: { min: 0.1, max: 3 },
    });

    graph.on('node:click', ({ node }) => {
      const entity = node.getData<Record<string, unknown> | undefined>();
      const chunkIds = entity?.source_chunk_ids as string[] | undefined;
      if (chunkIds?.length) {
        onNodeClick?.({
          id: node.id,
          name: (entity?.name as string) || node.id,
          source_chunk_ids: chunkIds,
        });
      }
    });

    graphRef.current = graph;

    return () => {
      graph.dispose();
      graphRef.current = null;
    };
  }, [containerRef, onNodeClick]);

  // Re-layout and render whenever the graph data changes.
  useEffect(() => {
    const graph = graphRef.current;
    if (!graph || !data.nodes.length) return;

    const dagreLayout = new DagreLayout({
      type: 'dagre',
      rankdir: 'LR',
      align: 'UL',
      ranksep: 240,
      nodesep: 200,
      edgeMinLen: 2,
      controlPoints: false,
    });

    const layoutData = {
      nodes: data.nodes.map((node) => ({ ...node })) as NodeMetadata[],
      edges: data.edges.map((edge) => ({ ...edge })) as EdgeMetadata[],
    };

    dagreLayout.execute(layoutData).then(() => {
      const positionedNodes = new Map<string, NodeMetadata>();

      dagreLayout.forEachNode((node) => {
        positionedNodes.set(String(node.id), {
          ...(node._original as NodeMetadata),
          x: node.x,
          y: node.y,
        });
      });

      const nodes = layoutData.nodes
        .map((node) => positionedNodes.get(node.id as string))
        .filter(Boolean) as NodeMetadata[];

      graph.fromJSON({ nodes, edges: layoutData.edges });
      graph.zoomToFit({ padding: 20 });
    });
  }, [data]);
}
