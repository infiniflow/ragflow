import { useCallback } from 'react';
import { NodeHandleId } from '../../constant';
import useGraphStore from '../../store';

export function useDeleteToolNode() {
  const { edges, deleteEdgeById, deleteNodeById } = useGraphStore(
    (state) => state,
  );
  const deleteToolNode = useCallback(
    (agentNodeId: string) => {
      const edge = edges.find(
        (x) => x.source === agentNodeId && x.sourceHandle === NodeHandleId.Tool,
      );

      if (edge) {
        deleteEdgeById(edge.id);
        deleteNodeById(edge.target);
      }
    },
    [deleteEdgeById, deleteNodeById, edges],
  );

  return { deleteToolNode };
}
