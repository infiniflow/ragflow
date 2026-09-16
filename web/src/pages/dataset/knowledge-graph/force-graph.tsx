import { ElementDatum, Graph, IElementEvent } from '@antv/g6';
import isEmpty from 'lodash/isEmpty';
import { useCallback, useEffect, useId, useMemo, useRef } from 'react';
import { buildNodesAndCombos, defaultComboLabel } from './util';

import { useIsDarkTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';
import styles from './index.module.less';

const TooltipColorMap = {
  combo: 'text-red-600',
  node: 'text-black',
  edge: 'text-blue-600',
};

const getMaxSize = (node: any) => {
  if (!node?.size) return 32;
  const size = Array.isArray(node.size) ? node.size : [node.size, node.size];
  return Math.max(size[0] || 32, size[1] || 32);
};

interface IProps {
  data: any;
  show: boolean;
}

const ForceGraph = ({ data, show }: IProps) => {
  const tooltipId = useId();
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);
  const isDark = useIsDarkTheme();
  const nextData = useMemo(() => {
    if (!isEmpty(data)) {
      const graphData = data;
      const mi = buildNodesAndCombos(graphData.nodes);
      return { edges: graphData.edges, ...mi };
    }
    return { nodes: [], edges: [], combos: [] };
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
        'collapse-expand',
        {
          type: 'hover-activate',
          degree: 1, // 👈🏻 Activate relations.
        },
      ],
      plugins: [
        {
          type: 'tooltip',
          enterable: true,
          getContent: (e: IElementEvent, items: ElementDatum) => {
            if (Array.isArray(items)) {
              if (items.some((x) => x?.isCombo)) {
                return `<p class="font-semibold text-red-600">${items?.[0]?.data?.label}</p>`;
              }

              return items
                .flatMap((item) => {
                  return [
                    '<div ',
                    `id="${tooltipId}"`,
                    `aria-label="${item?.id}"`,
                    `role="tooltip"`,
                    `class="${TooltipColorMap[e['targetType'] as keyof typeof TooltipColorMap]}"`,
                    '>',
                    `<h3>${item?.id}</h3>`,
                    '<dl class="mb-1 empty:hidden">',
                    ...(item?.entity_type
                      ? [
                          '<div class="flex items-center gap-[.5ch]">',
                          '<dt><b>Entity type: </b></dt>',
                          `<dd>${item.entity_type}</dd>`,
                          '</div>',
                        ]
                      : []),
                    ...(item?.weight
                      ? [
                          '<div class="flex items-center gap-[.5ch]">',
                          '<dt><b>Weight: </b></dt>',
                          `<dd>${item.weight}</dd>`,
                          '</div>',
                        ]
                      : []),
                    '</dl>',
                    item.description
                      ? `<p class="text-xs">${item.description}</p>`
                      : '',
                    '</div>',
                  ];
                })
                .join('');
            }

            return undefined;
          },
        },
      ],
      layout: {
        type: 'combo-combined',
        comboPadding: 10,
        nodeSpacing: 100,
        comboSpacing: 100,
        layout: (comboId: string | null) =>
          !comboId
            ? {
                type: 'force',
                preventOverlap: true,
                gravity: 1,
                factor: 4,
                linkDistance: (_edge: any, source: any, target: any) => {
                  const sourceSize = getMaxSize(source);
                  const targetSize = getMaxSize(target);
                  return sourceSize / 2 + targetSize / 2 + 200;
                },
              }
            : { type: 'concentric', preventOverlap: true },
      },
      node: {
        style: {
          size: (d) => {
            const size = 100 + ((d.rank as number) || 0) * 5;
            return Math.min(size, 300);
          },

          labelText: (d) => d.id,
          labelFill: isDark ? 'rgba(255,255,255,1)' : 'rgba(0,0,0,1)',
          // labelPadding: 30,
          labelFontSize: 40,
          // labelOffsetX: 20,
          labelOffsetY: 20,
          labelPlacement: 'center',
          labelWordWrap: true,
        },
        palette: {
          type: 'group',
          field: (d) => d?.entity_type as string,
        },
      },
      edge: {
        style: (model) => {
          const weight: number = Number(model?.weight) || 2;

          return {
            stroke: isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.5)',
            lineDash: [10, 10],
            lineWidth: Math.min(weight * 4, 8),
          };
        },
      },
      combo: {
        style: (e) => {
          if (e.label === defaultComboLabel) {
            return {
              stroke: 'rgba(0,0,0,0)',
              fill: 'rgba(0,0,0,0)',
            };
          } else {
            return {
              stroke: isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.5)',
            };
          }
        },
      },
    });

    if (graphRef.current) {
      graphRef.current.destroy();
    }

    graphRef.current = graph;

    graph.setData(nextData);

    graph.render();
  }, [isDark, nextData, tooltipId]);

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

export default ForceGraph;
