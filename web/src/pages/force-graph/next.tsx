import { useEffect, useRef } from 'react';
import { ForceGraph2D } from 'react-force-graph';
import { graphData } from './constant';

const Next = () => {
  const graphRef = useRef<ForceGraph2D>();

  useEffect(() => {
    graphRef.current.d3Force('cluster');
  }, []);

  return (
    <div>
      <ForceGraph2D
        ref={graphRef}
        graphData={graphData}
        nodeAutoColorBy={'type'}
        nodeLabel={(node) => {
          return node.id;
        }}
        // nodeVal={(node) => {
        //   return <div>xxx</div>;
        // }}
        // nodeVal={(node) => 100 / (node.level + 1)}
        linkAutoColorBy={'type'}
        nodeCanvasObjectMode={() => 'after'}
        nodeCanvasObject={(node, ctx) => {
          console.info(ctx);
          return ctx.canvas;
        }}
        // nodeVal={'id'}
      />
    </div>
  );
};

export default Next;
