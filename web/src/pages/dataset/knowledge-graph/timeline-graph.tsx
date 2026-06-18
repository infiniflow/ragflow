import { ElementDatum, Graph, IElementEvent } from '@antv/g6';
import isEmpty from 'lodash/isEmpty';
import { useCallback, useEffect, useId, useMemo, useRef } from 'react';

import { useIsDarkTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';
import styles from './index.module.less';

/**
 * Vertical-waterfall renderer for timeline-kind structure graphs.
 *
 * Same data contract as ``force-graph.tsx`` and ``tree-graph.tsx`` —
 * ``{nodes, edges}`` keyed by ``id``. Layout is antv/g6's ``dag``
 * (top-to-bottom DAG, polyline edges) so the result reads as a
 * flowchart even when the relations branch. ``mention_count`` drives
 * node size on a log-clamp curve; ``entity_type`` drives node colour
 * via g6's ``palette: {type: 'group'}`` strategy so timeline + force
 * views agree on which colour means which type.
 *
 * Read-only — no node-click behaviour. Drag / zoom / hover-tooltip are
 * inherited from the shared g6 behaviours, same as the peer renderers.
 */

interface TimelineNode {
  id: string;
  entity_type?: string;
  /** From backend ``mention_count`` — drives node size. */
  rank?: number;
  description?: string;
  aliases?: string[];
}

interface TimelineEdge {
  source: string;
  target: string;
  description?: string;
}

interface IProps {
  data: { nodes: TimelineNode[]; edges: TimelineEdge[] };
  show: boolean;
}

// Node-size tuning. Width auto-scales with label length AND
// log-clamped ``mention_count`` so importance is visible without one
// hot node blowing up the canvas. Height is fixed so each rank reads
// as a neat row.
const MIN_NODE_W = 140;
const MAX_NODE_W = 280;
const NODE_H = 48;

// Layout breathing room. dagre's ``nodesep`` / ``ranksep`` are gaps
// between bounding boxes (not centres), so values clearly larger than
// a glance keep the waterfall legible at typical zoom.
const LAYOUT_NODE_SEP = 56;
const LAYOUT_RANK_SEP = 88;

/**
 * Compute the rendered box for one node. Width grows with both the
 * importance signal (log-clamped ``mention_count``) and label length;
 * height is fixed.
 */
function sizeForNode(node: { id?: string; rank?: number }): [number, number] {
  const rank = Math.max(0, node.rank ?? 0);
  const labelLen = (node.id ?? '').length;
  const importance = Math.log(1 + rank) * 14;
  const labelW = labelLen * 9 + 32;
  const w = Math.min(MAX_NODE_W, Math.max(MIN_NODE_W, labelW + importance));
  return [w, NODE_H];
}

const TimelineGraph = ({ data, show }: IProps) => {
  const tooltipId = useId();
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);
  const isDark = useIsDarkTheme();

  /**
   * Decorate nodes with their pre-computed ``size`` so the layout
   * (``nodeSize``) and the renderer (``style.size``) read the same
   * value. Without this dagre treats nodes as points → pile-up.
   */
  const safeData = useMemo(() => {
    const rawNodes = Array.isArray(data?.nodes) ? data.nodes : [];
    const edges = Array.isArray(data?.edges) ? data.edges : [];
    const nodes = rawNodes.map((n) => ({
      ...n,
      size: sizeForNode(n),
    }));
    return { nodes, edges };
  }, [data]);

  const render = useCallback(() => {
    const graph = new Graph({
      container: containerRef.current!,
      autoFit: 'view',
      autoResize: true,
      behaviors: [
        'drag-element',
        'drag-canvas',
        'zoom-canvas',
        { type: 'hover-activate', degree: 1 },
      ],
      plugins: [
        {
          type: 'tooltip',
          enterable: true,
          getContent: (_e: IElementEvent, items: ElementDatum) => {
            if (!Array.isArray(items)) return '';
            return items
              .flatMap((item) => {
                const lines: string[] = [];
                lines.push(
                  '<div ',
                  `id="${tooltipId}"`,
                  `aria-label="${item?.id}"`,
                  `role="tooltip"`,
                  '>',
                  `<h3>${item?.id}</h3>`,
                  '<dl class="mb-1 empty:hidden">',
                );
                if ((item as any)?.entity_type) {
                  lines.push(
                    '<div class="flex items-center gap-[.5ch]">',
                    '<dt><b>Type: </b></dt>',
                    `<dd>${(item as any).entity_type}</dd>`,
                    '</div>',
                  );
                }
                if ((item as any)?.rank != null) {
                  lines.push(
                    '<div class="flex items-center gap-[.5ch]">',
                    '<dt><b>Mentions: </b></dt>',
                    `<dd>${(item as any).rank}</dd>`,
                    '</div>',
                  );
                }
                if ((item as any)?.description) {
                  lines.push(
                    '<div><dt><b>Description: </b></dt>',
                    `<dd>${(item as any).description}</dd></div>`,
                  );
                }
                if (
                  Array.isArray((item as any)?.aliases) &&
                  (item as any).aliases.length
                ) {
                  lines.push(
                    '<div><dt><b>Aliases: </b></dt>',
                    `<dd>${(item as any).aliases.join(', ')}</dd></div>`,
                  );
                }
                lines.push('</dl>', '</div>');
                return lines;
              })
              .join('');
          },
        },
      ],
      // ``antv-dagre`` is the dagre-based hierarchical layout shipped
      // with g6 v5. We feed it ``nodeSize`` so it spaces nodes by
      // their actual bounding box, not as points — without this every
      // node lands on the same coordinate and rectangles pile up.
      layout: {
        type: 'antv-dagre',
        rankdir: 'TB',
        align: 'UL',
        nodesep: LAYOUT_NODE_SEP,
        ranksep: LAYOUT_RANK_SEP,
        nodeSize: (node: any) => {
          const sz = node?.size;
          if (Array.isArray(sz) && sz.length === 2) return sz;
          return [MIN_NODE_W, NODE_H];
        },
      } as any,
      node: {
        type: 'rect',
        style: {
          // Same size that was fed to the layout — the rendered box
          // matches the slot dagre reserved.
          size: (d: any) =>
            Array.isArray(d.size) ? d.size : [MIN_NODE_W, NODE_H],
          radius: 8,
          stroke: isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.3)',
          lineWidth: 1,
          labelText: (d: any) => (d.id as string) ?? '',
          labelFill: isDark ? 'rgba(255,255,255,0.95)' : 'rgba(15,23,42,0.95)',
          labelFontSize: 13,
          labelTextAlign: 'center',
          labelTextBaseline: 'middle',
          labelPlacement: 'center',
          labelWordWrap: true,
          labelMaxLines: 2,
          labelMaxWidth: (d: any) =>
            Array.isArray(d.size) ? Math.max(80, d.size[0] - 24) : 240,
        },
        // Match force-graph's palette strategy so the colour assigned
        // to a given ``entity_type`` is consistent across views.
        palette: {
          type: 'group',
          field: (d: any) => (d?.entity_type as string) ?? 'other',
        },
      },
      edge: {
        // Polyline / orthogonal segments → classic flowchart look.
        // dagre supplies control points; polyline routes through them.
        type: 'polyline',
        style: {
          stroke: isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.4)',
          lineWidth: 1.4,
          endArrow: true,
          endArrowType: 'triangle',
          endArrowSize: 8,
          router: { type: 'orth' },
        },
      },
    });

    if (graphRef.current) {
      graphRef.current.destroy();
    }
    graphRef.current = graph;

    graph.setData({
      nodes: safeData.nodes as any,
      edges: safeData.edges as any,
    });
    graph.render();
  }, [safeData, isDark, tooltipId]);

  useEffect(() => {
    if (!isEmpty(safeData.nodes)) {
      render();
    }
  }, [safeData, render]);

  return (
    <div
      ref={containerRef}
      className={cn(styles.forceContainer, 'size-full', !show && 'hidden')}
      aria-haspopup="true"
      aria-describedby={tooltipId}
    />
  );
};

export default TimelineGraph;
