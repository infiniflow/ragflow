import { Graph as G6Graph, treeToGraphData } from '@antv/g6';
import { useEffect, useMemo, useRef } from 'react';

const assignIds = (node: any, parentId: string = '', index = 0) => {
  if (!node.id) node.id = parentId ? `${parentId}-${index}` : 'root';
  if (node.children) {
    node.children.forEach((child: any, idx: number) =>
      assignIds(child, node.id, idx),
    );
  }
};

const getNodeSize = (d: any): [number, number] => {
  const text = d.id || '';
  const lines = text.split('\n');
  const maxChars = Math.max(...lines.map((l: string) => l.length), 0);
  const width = Math.min(maxChars * 6 + 20, 400);
  const height = Math.max(lines.length * 20 + 20, 40);
  return [width, height];
};

export interface GraphProps {
  onRender?: (graph: G6Graph) => void;
  onDestroy?: () => void;
  data: any;
}

export const IndentedTree = (props: GraphProps) => {
  const { onRender, onDestroy, data } = props;

  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<G6Graph>();

  const options = useMemo(
    () => ({
      data: treeToGraphData(data),
      autoFit: 'view',
      node: {
        style: (d: any) => ({
          labelText: d.id,
          labelPlacement: 'right',
          labelTextBaseline: 'top',
          labelBackground: true,
          fill: 'transparent',
          stroke: 'transparent',
          size: [0.1, 0.1],
        }),
        animation: {
          enter: false,
        },
      },
      edge: {
        type: 'polyline',
        style: {
          radius: 4,
          router: {
            type: 'orth',
          },
        },
        animation: {
          enter: false,
        },
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
    }),
    [data],
  );

  useEffect(() => {
    const graph = new G6Graph({ container: containerRef.current! });
    graphRef.current = graph;

    return () => {
      const graph = graphRef.current;
      if (graph) {
        graph.destroy();
        onDestroy?.();
        graphRef.current = undefined;
      }
    };
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    const graph = graphRef.current;

    if (!options || !container || !graph || graph.destroyed || !data) return;

    graph.setOptions(options as any);
    assignIds(data);
    graph.setData(treeToGraphData(data));
    graph
      .render()
      .then(() => onRender?.(graph))
      .catch((error) => console.debug(error));
  }, [options, data, onRender]);

  return <div ref={containerRef} style={{ width: '100%', height: '100%' }} />;
};
