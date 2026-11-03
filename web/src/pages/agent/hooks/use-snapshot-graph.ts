import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { Edge } from '@xyflow/react';
import { useCallback } from 'react';
import { useCanvasSnapshot } from '../context';
import useGraphStore from '../store';

// Snapshot-aware graph accessors. Inside a read-only snapshot preview (e.g.
// the version history dialog) they resolve nodes/edges from the provided
// snapshot instead of the live canvas store, which always holds the current
// draft; outside a preview they fall back to the store unchanged.

export function useSnapshotAwareNodes(): RAGFlowNodeType[] {
  const snapshot = useCanvasSnapshot();
  const storeNodes = useGraphStore((state) => state.nodes);

  return snapshot?.nodes ?? storeNodes;
}

export function useSnapshotAwareEdges(): Edge[] {
  const snapshot = useCanvasSnapshot();
  const storeEdges = useGraphStore((state) => state.edges);

  return snapshot?.edges ?? storeEdges;
}

export function useSnapshotAwareGetNode() {
  const snapshot = useCanvasSnapshot();
  const getNode = useGraphStore((state) => state.getNode);

  return useCallback(
    (id?: string | null) =>
      snapshot ? snapshot.nodes.find((x) => x.id === id) : getNode(id),
    [snapshot, getNode],
  );
}

export function useSnapshotAwareGetOperatorTypeFromId() {
  const getNode = useSnapshotAwareGetNode();

  return useCallback(
    (id?: string | null) => getNode(id)?.data?.label,
    [getNode],
  );
}
