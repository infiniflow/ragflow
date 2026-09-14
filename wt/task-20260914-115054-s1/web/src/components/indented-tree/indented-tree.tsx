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

import { Graph as G6Graph, treeToGraphData } from '@antv/g6';
import { useSize } from 'ahooks';
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
  const size = useSize(containerRef);
  const width = size?.width;
  const height = size?.height;

  const options = useMemo(
    () => ({
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
    [],
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
  }, [onDestroy]);

  useEffect(() => {
    const container = containerRef.current;
    const graph = graphRef.current;

    if (!container || !graph || graph.destroyed || !data) return;

    graph.setOptions(options as any);
    assignIds(data);
    graph.setData(treeToGraphData(data));
    graph
      .render()
      .then(() => onRender?.(graph))
      .catch((error) => console.debug(error));
  }, [options, data, onRender]);

  // G6 sizes its canvas at creation time, so the graph must be resized and
  // re-fitted whenever the container (window / sheet) changes size.
  useEffect(() => {
    const graph = graphRef.current;

    if (!graph || graph.destroyed || !width || !height) return;

    const [canvasWidth, canvasHeight] = graph.getSize();
    if (canvasWidth === width && canvasHeight === height) return;

    graph.resize(width, height);
    graph.fitView().catch((error: unknown) => console.debug(error));
  }, [width, height]);

  return <div ref={containerRef} style={{ width: '100%', height: '100%' }} />;
};
