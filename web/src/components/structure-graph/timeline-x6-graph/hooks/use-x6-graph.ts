/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { DagreLayout } from '@antv/layout';
import { Graph, type EdgeMetadata, type NodeMetadata } from '@antv/x6';
import { useEffect, useRef } from 'react';

import { type TimelineX6NodeData } from '../../adapters';
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
        const original = node._original as TimelineX6NodeData;
        positionedNodes.set(String(node.id), {
          ...(node._original as NodeMetadata),
          x: (node.x ?? 0) - original.width / 2,
          y: (node.y ?? 0) - original.height / 2,
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
