import {
  BaseEdge,
  Edge,
  EdgeLabelRenderer,
  EdgeProps,
  getBezierPath,
} from '@xyflow/react';
import { memo } from 'react';
import useGraphStore from '../../store';

import { useFetchAgent } from '@/hooks/use-agent-request';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { NodeHandleId, Operator } from '../../constant';

function InnerButtonEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  source,
  target,
  style = {},
  markerEnd,
  selected,
  data,
  sourceHandleId,
}: EdgeProps<Edge<{ isHovered: boolean }>>) {
  const { deleteEdgeById, getOperatorTypeFromId } = useGraphStore(
    (state) => state,
  );

  const [edgePath, labelX, labelY] = getBezierPath({
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

  const isTargetPlaceholder = useMemo(() => {
    return getOperatorTypeFromId(target) === Operator.Placeholder;
  }, [getOperatorTypeFromId, target]);

  const placeholderHighlightStyle = useMemo(() => {
    const isHighlighted = isTargetPlaceholder;
    return isHighlighted
      ? { strokeWidth: 2, stroke: 'rgb(var(--accent-primary))' }
      : {};
  }, [isTargetPlaceholder]);

  const onEdgeClick = () => {
    deleteEdgeById(id);
  };

  // highlight the nodes that the workflow passes through
  const { data: flowDetail } = useFetchAgent();

  const showHighlight = useMemo(() => {
    const path = flowDetail?.dsl?.path ?? [];
    const idx = path.findIndex((x) => x === target);
    if (idx !== -1) {
      let index = idx - 1;
      while (index >= 0) {
        if (path[index] === source) {
          return { strokeWidth: 1, stroke: 'rgb(var(--accent-primary))' };
        }
        index--;
      }
      return {};
    }
    return {};
  }, [flowDetail?.dsl?.path, source, target]);

  const visible = useMemo(() => {
    return (
      data?.isHovered &&
      sourceHandleId !== NodeHandleId.Tool &&
      sourceHandleId !== NodeHandleId.AgentBottom && // The connection between the agent node and the tool node does not need to display the delete button
      !target.startsWith(Operator.Tool) &&
      !isTargetPlaceholder
    );
  }, [data?.isHovered, isTargetPlaceholder, sourceHandleId, target]);

  const activeMarkerEnd =
    selected || !isEmpty(showHighlight) || isTargetPlaceholder
      ? 'url(#selected-marker)'
      : markerEnd;

  return (
    <>
      <BaseEdge
        path={edgePath}
        markerEnd={activeMarkerEnd}
        style={{
          ...style,
          ...selectedStyle,
          ...showHighlight,
          ...placeholderHighlightStyle,
        }}
        className={cn('text-text-disabled')}
      />

      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            fontSize: 12,
            // everything inside EdgeLabelRenderer has no pointer events by default
            // if you have an interactive element, set pointer-events: all
            pointerEvents: 'all',
            zIndex: 1001, // https://github.com/xyflow/xyflow/discussions/3498
          }}
          className="nodrag nopan"
        >
          <button
            className={cn(
              'size-3.5 border border-state-error text-state-error rounded-full leading-none bg-bg-canvas outline outline-bg-canvas',
              'invisible',
              { visible },
            )}
            type="button"
            onClick={onEdgeClick}
          >
            ×
          </button>
        </div>
      </EdgeLabelRenderer>
    </>
  );
}

export const ButtonEdge = memo(InnerButtonEdge);
