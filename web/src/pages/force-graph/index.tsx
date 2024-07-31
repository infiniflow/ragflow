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

  const render = () => {
    const graph = new Graph({
      container: containerRef.current!,
      autoFit: 'view',
      behaviors: ['drag-element', 'drag-canvas', 'zoom-canvas'],
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
        comboPadding: 2,
      },
      node: {
        style: {
          size: 20,
          labelText: (d) => d.id,
          labelPadding: 20,
          //   labelOffsetX: 20,
          labelOffsetY: 5,
        },
        palette: {
          type: 'group',
          field: (d) => d.combo,
        },
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
    render();
  }, []);

  return <div ref={containerRef} className={styles.container} />;
};

export default ForceGraph;
