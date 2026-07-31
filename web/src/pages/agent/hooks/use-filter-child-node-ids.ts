import { filterChildNodeIds } from '@/utils/canvas-util';
import useGraphStore from '../store';

export function useFilterChildNodeIds(nodeId?: string) {
  const nodes = useGraphStore((state) => state.nodes);

  const childNodeIds = filterChildNodeIds(nodes, nodeId);

  return childNodeIds ?? [];
}
