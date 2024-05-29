import { useCallback, useLayoutEffect } from 'react';
import { getLayoutedElements } from './elk-utils';

export const elkOptions = {
  'elk.algorithm': 'layered',
  'elk.layered.spacing.nodeNodeBetweenLayers': '100',
  'elk.spacing.nodeNode': '80',
};

export const useLayoutGraph = (
  initialNodes,
  initialEdges,
  setNodes,
  setEdges,
) => {
  const onLayout = useCallback(({ direction, useInitialNodes = false }) => {
    const opts = { 'elk.direction': direction, ...elkOptions };
    const ns = initialNodes;
    const es = initialEdges;

    getLayoutedElements(ns, es, opts).then(
      ({ nodes: layoutedNodes, edges: layoutedEdges }) => {
        setNodes(layoutedNodes);
        setEdges(layoutedEdges);

        // window.requestAnimationFrame(() => fitView());
      },
    );
  }, []);

  // Calculate the initial layout on mount.
  useLayoutEffect(() => {
    onLayout({ direction: 'RIGHT', useInitialNodes: true });
  }, [onLayout]);
};
