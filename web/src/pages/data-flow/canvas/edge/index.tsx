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
  const deleteEdgeById = useGraphStore((state) => state.deleteEdgeById);
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });
  const selectedStyle = useMemo(() => {
    return selected ? { strokeWidth: 1, stroke: 'rgba(76, 164, 231, 1)' } : {};
  }, [selected]);

  const onEdgeClick = () => {
    deleteEdgeById(id);
  };

  // highlight the nodes that the workflow passes through
  const { data: flowDetail } = useFetchAgent();

  const graphPath = useMemo(() => {
    // TODO: this will be called multiple times
    const path = flowDetail?.dsl?.path ?? [];
    // The second to last
    const previousGraphPath: string[] = path.at(-2) ?? [];
    let graphPath: string[] = path.at(-1) ?? [];
    // The last of the second to last article
    const previousLatestElement = previousGraphPath.at(-1);
    if (previousGraphPath.length > 0 && previousLatestElement) {
      graphPath = [previousLatestElement, ...graphPath];
    }
    return Array.isArray(graphPath) ? graphPath : [];
  }, [flowDetail.dsl?.path]);

  const highlightStyle = useMemo(() => {
    const idx = graphPath.findIndex((x) => x === source);
    if (idx !== -1) {
      // The set of elements following source
      const slicedGraphPath = graphPath.slice(idx + 1);
      if (slicedGraphPath.some((x) => x === target)) {
        return { strokeWidth: 1, stroke: 'red' };
      }
    }
    return {};
  }, [source, target, graphPath]);

  const visible = useMemo(() => {
    return (
      data?.isHovered &&
      sourceHandleId !== NodeHandleId.Tool &&
      sourceHandleId !== NodeHandleId.AgentBottom && // The connection between the agent node and the tool node does not need to display the delete button
      !target.startsWith(Operator.Tool)
    );
  }, [data?.isHovered, sourceHandleId, target]);

  return (
    <>
      <BaseEdge
        path={edgePath}
        markerEnd={markerEnd}
        style={{ ...style, ...selectedStyle, ...highlightStyle }}
        className="text-text-secondary"
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
              'size-3.5 border border-state-error text-state-error rounded-full leading-none',
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
