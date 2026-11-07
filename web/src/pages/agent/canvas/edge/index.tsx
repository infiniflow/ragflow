import {
  BaseEdge,
  Edge,
  EdgeLabelRenderer,
  EdgeProps,
  getBezierPath,
  useStore,
} from '@xyflow/react';
import { memo, useMemo } from 'react';
import useGraphStore from '../../store';

import { useFetchAgent } from '@/hooks/use-agent-request';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { PointerEvent as ReactPointerEvent } from 'react';
import { NodeHandleId, Operator } from '../../constant';
import {
  describeOrthogonalEdge,
  orthogonalLanes,
  useCanvasEdgeRoute,
} from '../../utils/canvas-edge-route';

type HandleBox = {
  id?: string | null;
  x: number;
  y: number;
  width: number;
  height: number;
  position?: string;
};

function handleCenter(
  node: {
    width?: number | null;
    height?: number | null;
    measured?: { width?: number; height?: number };
    internals: {
      positionAbsolute: { x: number; y: number };
      handleBounds?: {
        source?: HandleBox[] | null;
        target?: HandleBox[] | null;
      } | null;
    };
  },
  handleId: string | null | undefined,
  role: 'source' | 'target',
) {
  const bounds = node.internals.handleBounds?.[role] ?? [];
  const handle =
    (handleId ? bounds.find((item) => item.id === handleId) : undefined) ??
    bounds[0];
  const origin = node.internals.positionAbsolute;
  if (!handle) {
    const width = node.measured?.width ?? node.width ?? 0;
    const height = node.measured?.height ?? node.height ?? 0;
    return {
      x: origin.x + (role === 'source' ? width / 2 : -width / 2),
      y: origin.y + height / 2,
    };
  }
  const position = handle.position ?? (role === 'source' ? 'right' : 'left');
  const x = handle.x + origin.x;
  const y = handle.y + origin.y;
  if (position === 'right') {
    return { x: x + handle.width, y: y + handle.height / 2 };
  }
  if (position === 'left') return { x, y: y + handle.height / 2 };
  if (position === 'top') return { x: x + handle.width / 2, y };
  return { x: x + handle.width / 2, y: y + handle.height };
}

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

  const route = useCanvasEdgeRoute((state) => state.route);
  const lane = useStore(
    (state) => {
      const samples = state.edges.flatMap((edge) => {
        const sourceNode = state.nodeLookup.get(edge.source);
        const targetNode = state.nodeLookup.get(edge.target);
        if (!sourceNode || !targetNode) return [];
        const sourcePoint = handleCenter(
          sourceNode,
          edge.sourceHandle,
          'source',
        );
        const targetPoint = handleCenter(
          targetNode,
          edge.targetHandle,
          'target',
        );
        return [
          {
            id: edge.id,
            sourceX: sourcePoint.x,
            sourceY: sourcePoint.y,
            targetX: targetPoint.x,
            targetY: targetPoint.y,
          },
        ];
      });
      return (
        orthogonalLanes(samples).get(id) ?? {
          step: 0.62,
          lane: 0,
          laneCount: 1,
        }
      );
    },
    (left, right) =>
      left.step === right.step &&
      left.lane === right.lane &&
      left.laneCount === right.laneCount,
  );
  const curved =
    route === 'orthogonal'
      ? describeOrthogonalEdge(
          { x: sourceX, y: sourceY },
          { x: targetX, y: targetY },
          lane.step,
          lane.lane,
          lane.laneCount,
        )
      : null;
  const plain = curved
    ? null
    : getBezierPath({
        sourceX,
        sourceY,
        sourcePosition,
        targetX,
        targetY,
        targetPosition,
      });
  const edgePath = curved?.path ?? plain?.[0] ?? '';
  const labelX = curved?.labelX ?? plain?.[1] ?? 0;
  const labelY = curved?.labelY ?? plain?.[2] ?? 0;
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
