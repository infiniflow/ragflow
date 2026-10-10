import { cn } from '@/lib/utils';
import { BaseEdge, Edge, EdgeProps, getBezierPath } from '@xyflow/react';
import { memo, useMemo } from 'react';

// ReadonlyEdge renders an edge on the read-only canvas (e.g. the built-in
// pipeline preview). It keeps the path, arrow marker and selection highlight
// of ButtonEdge but drops the delete button and the run-path highlight —
// the latter would call useFetchAgent with a built-in pipeline id and 404.
function InnerReadonlyEdge({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  markerEnd,
  selected,
}: EdgeProps<Edge>) {
  const [edgePath] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  const selectedStyle = useMemo(() => {
    return selected
      ? { strokeWidth: 1, stroke: 'rgb(var(--accent-primary))' }
      : {};
  }, [selected]);

  return (
    <BaseEdge
      path={edgePath}
      markerEnd={selected ? 'url(#selected-marker)' : markerEnd}
      style={{ ...style, ...selectedStyle }}
      className={cn('text-text-disabled')}
    />
  );
}

export const ReadonlyEdge = memo(InnerReadonlyEdge);
