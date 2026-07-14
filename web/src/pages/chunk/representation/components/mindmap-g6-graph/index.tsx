import { cn } from '@/lib/utils';
import { Graph, IElementEvent, NodeEvent, treeToGraphData } from '@antv/g6';
import { memo, useEffect, useRef } from 'react';

import { adaptMindMapToIndentedTree } from '../../utils/adapters';
import { type MindMapG6GraphProps, type MindMapNodeValue } from './types';

interface MindMapNodeData {
  name?: string;
  source_chunk_ids?: string[];
}

function MindMapG6Graph({
  template,
  show = true,
  onNodeClick,
}: MindMapG6GraphProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const styles = window.getComputedStyle(container);
    const textPrimary = styles.getPropertyValue('--text-primary').trim();
    const textSecondary = styles.getPropertyValue('--text-secondary').trim();
    const accentPrimary = styles.getPropertyValue('--accent-primary').trim();

    let graph: Graph | null = null;
    let observer: ResizeObserver | null = null;

    const createGraph = () => {
      if (graph) return;

      const { width, height } = container.getBoundingClientRect();
      if (width === 0 || height === 0) return;

      graph = new Graph({
        container,
        width,
        height,
        autoFit: 'view',
        data: treeToGraphData(adaptMindMapToIndentedTree(template)),
        node: {
          style: {
            labelText: (datum) =>
              (datum.data as MindMapNodeData | undefined)?.name ?? datum.id,
            labelFill: textPrimary ? `rgb(${textPrimary})` : '#262626',
            fill: accentPrimary ? `rgb(${accentPrimary})` : '#00beb4',
            labelBackground: true,
            labelBackgroundFill: 'transparent',
            labelPlacement: 'top',
          },
        },
        edge: {
          type: 'cubic-horizontal',
          style: {
            stroke: textSecondary ? `rgb(${textSecondary})` : '#75787a',
          },
        },
        layout: {
          type: 'mindmap',
          direction: 'H',
          preLayout: false,
          getHeight: () => 32,
          getWidth: (node: { id: string; data?: MindMapNodeData }) =>
            Math.max(120, String(node.data?.name ?? node.id).length * 7),
          getVGap: () => 10,
          getHGap: () => 80,
        },
        behaviors: ['collapse-expand', 'drag-canvas', 'zoom-canvas'],
      });

      const handleNodeClick = (evt: IElementEvent) => {
        const nodeId = evt.target.id as string;
        const nodeData = graph!.getNodeData(nodeId);
        const data = nodeData.data as MindMapNodeData | undefined;
        if (data?.source_chunk_ids?.length) {
          const payload: MindMapNodeValue = {
            id: nodeId,
            name: data.name || nodeId,
            source_chunk_ids: data.source_chunk_ids,
          };
          onNodeClick?.(payload);
        }
      };

      graph.on<IElementEvent>(NodeEvent.CLICK, handleNodeClick);
      graphRef.current = graph;

      void graph.render();
    };

    observer = new ResizeObserver(() => {
      if (!graph) {
        createGraph();
        return;
      }

      const { width, height } = container.getBoundingClientRect();
      if (width > 0 && height > 0) {
        graph.resize(width, height);
      }
    });

    observer.observe(container);
    createGraph();

    return () => {
      observer?.disconnect();
      graph?.destroy();
      graphRef.current = null;
    };
  }, [template, onNodeClick]);

  return (
    <div
      ref={containerRef}
      className={cn('flex-1 min-h-0 h-full', !show && 'hidden')}
    />
  );
}

const MemoizedMindMapG6Graph = memo(MindMapG6Graph) as typeof MindMapG6Graph;

export { MemoizedMindMapG6Graph as MindMapG6Graph };
export default MemoizedMindMapG6Graph;
