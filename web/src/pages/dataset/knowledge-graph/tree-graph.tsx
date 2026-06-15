import { ElementDatum, Graph, IElementEvent } from '@antv/g6';
import isEmpty from 'lodash/isEmpty';
import { useCallback, useEffect, useId, useMemo, useRef } from 'react';

import { useIsDarkTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';
import styles from './index.module.less';

/**
 * Tree renderer for hierarchical structures (page_index, table-of-contents-like).
 *
 * Same data contract as ``force-graph.tsx`` — ``{nodes, edges}`` keyed by
 * ``id``. Switches the antv/g6 layout to ``compact-box`` so the result is
 * a top-down tree instead of a force-directed cluster. The caller is
 * responsible for ensuring edges form a tree (one parent per node) and
 * for injecting a synthetic root if the source data is a forest.
 *
 * Why a sibling component instead of a mode-flag on ForceGraph: the
 * tree layout swap also pulls in collapse/expand interactions and a
 * different palette strategy (depth-based instead of entity-type), so
 * a clean sibling keeps both paths readable.
 */

const ROOT_FALLBACK_COLOR = '#7C3AED'; // amethyst — distinct from KG palette

interface TreeNode {
  id: string;
  depth?: number;
  entity_type?: string;
  description?: string;
  weight?: number;
  rank?: number;
  isRoot?: boolean;
}

interface TreeEdge {
  source: string;
  target: string;
  description?: string;
  weight?: number;
}

interface IProps {
  data: { nodes: TreeNode[]; edges: TreeEdge[] };
  show: boolean;
  rootId?: string;
}

const TreeGraph = ({ data, show, rootId }: IProps) => {
  const tooltipId = useId();
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);
  const isDark = useIsDarkTheme();

  /**
   * Compute a depth map by BFS from ``rootId`` so the renderer can color
   * nodes by tree depth (root → leaves). Falls back to depth 0 for any
   * node not reachable from the root — they still render, just outside
   * the main hierarchy.
   */
  const annotated = useMemo(() => {
    if (isEmpty(data?.nodes)) return { nodes: [], edges: [] };

    const adj = new Map<string, string[]>();
    for (const e of data.edges || []) {
      if (!adj.has(e.source)) adj.set(e.source, []);
      adj.get(e.source)!.push(e.target);
    }

    const depths = new Map<string, number>();
    if (rootId) {
      const queue: string[] = [rootId];
      depths.set(rootId, 0);
      while (queue.length > 0) {
        const cur = queue.shift()!;
        const d = depths.get(cur)!;
        for (const child of adj.get(cur) || []) {
          if (depths.has(child)) continue;
          depths.set(child, d + 1);
          queue.push(child);
        }
      }
    }

    const nodes = data.nodes.map((n) => ({
      ...n,
      depth: depths.get(n.id) ?? 0,
    }));
    return { nodes, edges: data.edges };
  }, [data, rootId]);

  const render = useCallback(() => {
    const graph = new Graph({
      container: containerRef.current!,
      autoFit: 'view',
      autoResize: true,
      behaviors: [
        'drag-element',
        'drag-canvas',
        'zoom-canvas',
        'collapse-expand',
        { type: 'hover-activate', degree: 1 },
      ],
      plugins: [
        {
          type: 'tooltip',
          enterable: true,
          getContent: (_e: IElementEvent, items: ElementDatum) => {
            if (!Array.isArray(items)) return undefined;
            return items
              .flatMap((item) => [
                `<div id="${tooltipId}" role="tooltip" aria-label="${item?.id}">`,
                `<h3 class="font-medium">${item?.id}</h3>`,
                item?.entity_type
                  ? `<div class="text-xs"><b>Type:</b> ${item.entity_type}</div>`
                  : '',
                item?.description
                  ? `<p class="text-xs whitespace-pre-wrap">${item.description}</p>`
                  : '',
                '</div>',
              ])
              .join('');
          },
        },
      ],
      layout: {
        type: 'compact-box',
        // Root on the left, depth grows rightward, siblings stack
        // vertically — the standard outline / file-tree orientation.
        direction: 'LR',
        getId: (d: any) => d.id,
        getHeight: () => 36,
        getWidth: (d: any) => Math.max(80, (d.id?.length ?? 0) * 9 + 24),
        // ``compact-box`` reads V/H gaps relative to the layout
        // direction. With LR, getVGap is between siblings (vertical
        // stacking) and getHGap is between parent and child (level
        // spacing).
        getVGap: () => 14,
        getHGap: () => 48,
      },
      node: {
        type: 'rect',
        style: {
          // Rectangles with rounded corners read more naturally as a
          // table-of-contents than circles do.
          size: (d: any) => {
            const labelLen = (d.id as string)?.length ?? 0;
            const width = Math.max(80, Math.min(labelLen * 9 + 24, 280));
            return [width, 36];
          },
          radius: 6,
          fill: (d: any) => {
            if (d.isRoot) return ROOT_FALLBACK_COLOR;
            const depth = (d.depth as number) ?? 0;
            // Lighter shades for deeper nodes (root is the boldest).
            const alpha = Math.max(0.2, 1 - depth * 0.18);
            return isDark
              ? `rgba(56, 189, 248, ${alpha})`
              : `rgba(14, 165, 233, ${alpha})`;
          },
          stroke: isDark ? 'rgba(255,255,255,0.4)' : 'rgba(0,0,0,0.3)',
          lineWidth: 1,
          labelText: (d: any) => (d.id as string) ?? '',
          labelFill: (d: any) =>
            d.isRoot
              ? '#ffffff'
              : isDark
                ? 'rgba(255,255,255,0.95)'
                : 'rgba(15,23,42,0.95)',
          labelFontSize: 13,
          labelTextAlign: 'center',
          labelTextBaseline: 'middle',
          labelPlacement: 'center',
          labelWordWrap: true,
          labelMaxWidth: 260,
          labelMaxLines: 2,
        },
      },
      edge: {
        // ``cubic-horizontal`` draws a smooth Bézier whose tangents are
        // horizontal — matches the LR layout direction. (``cubic-vertical``
        // looks awkward when siblings are stacked.)
        type: 'cubic-horizontal',
        style: {
          stroke: isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.4)',
          lineWidth: 1.2,
          endArrow: false,
        },
      },
    });

    if (graphRef.current) {
      graphRef.current.destroy();
    }
    graphRef.current = graph;

    graph.setData({ nodes: annotated.nodes, edges: annotated.edges } as any);
    graph.render();
  }, [annotated, isDark, tooltipId]);

  useEffect(() => {
    if (!isEmpty(data)) {
      render();
    }
  }, [data, render]);

  return (
    <div
      ref={containerRef}
      className={cn(styles.forceContainer, 'size-full', !show && 'hidden')}
      aria-haspopup="true"
      aria-describedby={tooltipId}
    />
  );
};

export default TreeGraph;
