import { IGraph } from '@/interfaces/database/agent';
import { useCallback } from 'react';
import useGraphStore from '../store';

export const useSetGraphInfo = () => {
  const { setEdges, setNodes } = useGraphStore((state) => state);
  const setGraphInfo = useCallback(
    ({ nodes = [], edges = [] }: IGraph) => {
      setNodes(nodes);
      setEdges(edges);
    },
    [setEdges, setNodes],
  );
  return setGraphInfo;
};
