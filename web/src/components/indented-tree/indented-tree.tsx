import { Graph as G6Graph, treeToGraphData } from '@antv/g6';
import { useEffect, useRef } from 'react';

const assignIds = (node: any, parentId: string = '', index = 0) => {
  if (!node.id) node.id = parentId ? `${parentId}-${index}` : 'root';
  if (node.children) {
    node.children.forEach((child: any, idx: number) =>
      assignIds(child, node.id, idx),
    );
  }
};

const getNodeSize = (d: any): [number, number] => {
  const text = d?.id || '';
  const lines = text.split('\n');
  const maxChars = Math.max(...lines.map((l: string) => l.length), 0);
  const width = Math.min(maxChars * 6 + 20, 400);
  const height = Math.max(lines.length * 20 + 20, 40);
  return [width, height];
};

export interface GraphProps {
  data: any;
}

export const IndentedTree = (props: GraphProps) => {
  const { data } = props;

  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<G6Graph>();

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const rect = container.getBoundingClientRect();

    let graphData;
    if (data) {
      assignIds(data);
      graphData = treeToGraphData(data);
    }

    const graph = new G6Graph({
      container,
      width: rect.width || undefined,
      height: rect.height || undefined,
      autoFit: 'view',
      autoResize: true,
      data: graphData,
      node: {
        style: (d: any) => ({
          labelText: d.id,
          labelPlacement: 'right',
          labelTextBaseline: 'top',
          labelBackground: true,
          fill: 'transparent',
          stroke: 'transparent',
          size: getNodeSize(d),
        }),
        animation: { enter: false },
      },
      edge: {
        type: 'polyline',
        style: { radius: 4, router: { type: 'orth' } },
        animation: { enter: false },
      },
      layout: {
        type: 'indented',
        direction: 'LR',
        indent: 80,
        getHeight: (d: any) => getNodeSize(d)[1],
        getWidth: (d: any) => getNodeSize(d)[0],
        getVGap: () => 8,
      },
      behaviors: [
        'drag-canvas',
        'zoom-canvas',
        'drag-element',
        'collapse-expand',
      ],
    });
    graphRef.current = graph;

    if (graphData) {
      graph
        .render()
        .catch((error) =>
          console.error('[IndentedTree] initial render failed:', error),
        );
    }

    return () => {
      if (!graph.destroyed) {
        graph.destroy();
      }
      graphRef.current = undefined;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const graph = graphRef.current;
    if (!graph || graph.destroyed || !data) return;

    assignIds(data);
    graph.setData(treeToGraphData(data));
    graph
      .render()
      .catch((error) => console.error('[IndentedTree] render failed:', error));
  }, [data]);

  return <div ref={containerRef} style={{ width: '100%', height: '100%' }} />;
};
