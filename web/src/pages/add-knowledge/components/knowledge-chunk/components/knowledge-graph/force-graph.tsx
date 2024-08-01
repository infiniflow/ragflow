import { Graph } from '@antv/g6';
import { useSize } from 'ahooks';
import { useCallback, useEffect, useMemo, useRef } from 'react';
import { graphData } from './constant';
import { Converter, buildNodesAndCombos, isDataExist } from './util';

import { useFetchKnowledgeGraph } from '@/hooks/chunk-hooks';
import styles from './index.less';

const converter = new Converter();

const nextData = converter.buildNodesAndCombos(
  graphData.nodes,
  graphData.edges,
);
console.log('🚀 ~ nextData:', nextData);

const finalData = { ...graphData, ...nextData };

const ForceGraph = () => {
  const containerRef = useRef<HTMLDivElement>(null);
  const size = useSize(containerRef);
  const { data } = useFetchKnowledgeGraph();

  const nextData = useMemo(() => {
    if (isDataExist(data)) {
      const graphData = data.data;
      const mi = buildNodesAndCombos(graphData.nodes);
      return { edges: graphData.links, ...mi };
    }
    return { nodes: [], edges: [] };
  }, [data]);
  console.log('🚀 ~ nextData ~ nextData:', nextData);

  const render = useCallback(() => {
    const graph = new Graph({
      container: containerRef.current!,
      autoFit: 'view',
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
          getContent: (e, items) => {
            if (items.every((x) => x?.description)) {
              let result = ``;
              items.forEach((item) => {
                result += `<h3>${item?.id}</h3>`;
                if (item?.description) {
                  result += `<p>${item?.description}</p>`;
                }
              });
              return result;
            }
            return undefined;
          },
        },
      ],
      layout: {
        type: 'combo-combined',
        preventOverlap: true,
        comboPadding: 1,
        spacing: 20,
      },
      node: {
        style: {
          size: 20,
          labelText: (d) => d.id,
          labelPadding: 30,
          //   labelOffsetX: 20,
          // labelOffsetY: 5,
          labelPlacement: 'center',
          labelWordWrap: true,
        },
        palette: {
          type: 'group',
          field: (d) => d.combo,
        },
        // state: {
        //   highlight: {
        //     fill: '#D580FF',
        //     halo: true,
        //     lineWidth: 0,
        //   },
        //   dim: {
        //     fill: '#99ADD1',
        //   },
        // },
      },
      edge: {
        style: (model) => {
          const { size, color } = model.data;
          return {
            stroke: color || '#99ADD1',
            lineWidth: size || 1,
          };
        },
      },
    });

    graph.setData(nextData);

    graph.render();
  }, [nextData]);

  useEffect(() => {
    if (isDataExist(data)) {
      render();
    }
  }, [data, render]);

  return <div ref={containerRef} className={styles.forceContainer} />;
};

export default ForceGraph;
