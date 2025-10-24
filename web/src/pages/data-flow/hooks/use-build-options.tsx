import { buildNodeOutputOptions } from '@/utils/canvas-util';
import { useMemo } from 'react';
import { Operator } from '../constant';
import OperatorIcon from '../operator-icon';
import useGraphStore from '../store';

export function useBuildNodeOutputOptions(nodeId?: string) {
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);

  return useMemo(() => {
    return buildNodeOutputOptions({
      nodes,
      edges,
      nodeId,
      Icon: ({ name }) => <OperatorIcon name={name as Operator}></OperatorIcon>,
    });
  }, [edges, nodeId, nodes]);
}
