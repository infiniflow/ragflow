import { Graph } from '@antv/g6';
import { useSize } from 'ahooks';
import { useEffect, useRef } from 'react';
import { graphData } from './constant';

import styles from './index.less';
import { Converter } from './util';

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
  let graph: Graph;

  const render = () => {
    graph = new Graph({
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
      //   data: graphData,
    });

    graph.setData(finalData);

    graph.render();
  };

  useEffect(() => {
    console.info('rendered');
    render();
  }, []);

  return <div ref={containerRef} className={styles.container} />;
};

export default ForceGraph;
