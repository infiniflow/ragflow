import { SkeletonCard } from '@/components/skeleton-card';
import { cn } from '@/lib/utils';
import { Graph, IElementEvent, NodeEvent, treeToGraphData } from '@antv/g6';
import { memo, useEffect, useRef, useState } from 'react';

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
  const [loading, setLoading] = useState(true);
  const [graphData, setGraphData] = useState<ReturnType<
    typeof treeToGraphData
  > | null>(null);

  // Defer heavy data transformation to avoid blocking the render phase.
  useEffect(() => {
    setLoading(true);
    setGraphData(null);

    const rafId = requestAnimationFrame(() => {
      setGraphData(treeToGraphData(adaptMindMapToIndentedTree(template)));
    });

    return () => {
      cancelAnimationFrame(rafId);
    };
  }, [template]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || !graphData) return;

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
        animation: false,
        data: graphData,
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
          getWidth: () => 120,
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

      void graph.render().then(() => {
        setLoading(false);
      });
    };

    // Defer graph creation so the browser paints the skeleton before the
    // heavy G6 layout + render starts.
    const rafId = requestAnimationFrame(() => {
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
    });

    return () => {
      cancelAnimationFrame(rafId);
      observer?.disconnect();
      graph?.destroy();
      graphRef.current = null;
    };
  }, [graphData, onNodeClick]);

  return (
    <div className="relative flex-1 min-h-0 h-full">
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <SkeletonCard className="w-80" />
        </div>
      )}
      <div
        ref={containerRef}
        className={cn('h-full', !show && 'hidden', loading && 'invisible')}
      />
    </div>
  );
}

const MemoizedMindMapG6Graph = memo(MindMapG6Graph) as typeof MindMapG6Graph;

export { MemoizedMindMapG6Graph as MindMapG6Graph };
export default MemoizedMindMapG6Graph;
