import {
  BaseEdge,
  Edge,
  EdgeLabelRenderer,
  EdgeProps,
  getBezierPath,
  useStore,
} from '@xyflow/react';
import {
  bezierDetourOffset,
  describeDetourBezier,
} from '../../utils/canvas-edge-curve';
import { memo, useMemo } from 'react';
import useGraphStore from '../../store';

import { useFetchAgent } from '@/hooks/use-agent-request';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { PointerEvent as ReactPointerEvent } from 'react';
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

  const obstacles = useStore(
    (state) => {
      const [originX, originY] = state.nodeOrigin;
      return [...state.nodeLookup.values()].flatMap((node) => {
        if (node.id === source || node.id === target) return [];
        if (
          node.type === 'noteNode' ||
          node.type === 'iterationNode' ||
          node.type === 'group'
        ) {
          return [];
        }
        const width = node.measured?.width ?? node.width ?? 0;
        const height = node.measured?.height ?? node.height ?? 0;
        const left = node.internals.positionAbsolute.x - width * originX;
        const top = node.internals.positionAbsolute.y - height * originY;
        const coversEnds =
          sourceX >= left &&
          sourceX <= left + width &&
          sourceY >= top &&
          sourceY <= top + height &&
          targetX >= left &&
          targetX <= left + width &&
          targetY >= top &&
          targetY <= top + height;
        if (coversEnds || width < 8 || height < 8) return [];
        return [{ x: left, y: top, width, height }];
      });
    },
    (left, right) =>
      left.length === right.length &&
      left.every(
        (box, index) =>
          box.x === right[index]?.x &&
          box.y === right[index]?.y &&
          box.width === right[index]?.width &&
          box.height === right[index]?.height,
      ),
  );
  const detour = bezierDetourOffset(
    { x: sourceX, y: sourceY },
    { x: targetX, y: targetY },
    obstacles,
  );
  const plain = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });
  const curved =
    detour === 0
      ? null
      : describeDetourBezier(
          { x: sourceX, y: sourceY },
          { x: targetX, y: targetY },
          detour,
        );
  const edgePath = curved?.path ?? plain[0];
  const labelX = curved?.labelX ?? plain[1];
  const labelY = curved?.labelY ?? plain[2];
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

  const onEdgeClick = (event: ReactPointerEvent<HTMLButtonElement>) => {
    // pointerdown: React Flow may swallow click inside group nodes.
    event.stopPropagation();
    event.preventDefault();
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

  const showDelete = useMemo(() => {
    return (
      (selected || data?.isHovered) &&
      sourceHandleId !== NodeHandleId.Tool &&
      sourceHandleId !== NodeHandleId.AgentBottom &&
      !target.startsWith(Operator.Tool) &&
      !isTargetPlaceholder
    );
  }, [data?.isHovered, isTargetPlaceholder, selected, sourceHandleId, target]);

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

      {showDelete ? (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              fontSize: 12,
              pointerEvents: 'auto',
              zIndex: 1002,
            }}
            className="nodrag nopan"
          >
            <button
              className="size-5 border border-state-error text-state-error rounded-full leading-none bg-bg-canvas outline outline-bg-canvas"
              type="button"
              onPointerDown={onEdgeClick}
            >
              ×
            </button>
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}

export const ButtonEdge = memo(InnerButtonEdge);
