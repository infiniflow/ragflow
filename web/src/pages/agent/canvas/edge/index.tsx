import {
  BaseEdge,
  EdgeLabelRenderer,
  EdgeProps,
  getBezierPath,
} from '@xyflow/react';
import useGraphStore from '../../store';

import { useTheme } from '@/components/theme-provider';
import { useFetchFlow } from '@/hooks/flow-hooks';
import { useMemo } from 'react';
import styles from './index.less';

export function ButtonEdge({
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
}: EdgeProps) {
  const deleteEdgeById = useGraphStore((state) => state.deleteEdgeById);
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });
  const { theme } = useTheme();
  const selectedStyle = useMemo(() => {
    return selected ? { strokeWidth: 2, stroke: '#1677ff' } : {};
  }, [selected]);

  const onEdgeClick = () => {
    deleteEdgeById(id);
  };

  // highlight the nodes that the workflow passes through
  const { data: flowDetail } = useFetchFlow();

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
    return graphPath;
  }, [flowDetail.dsl?.path]);

  const highlightStyle = useMemo(() => {
    const idx = graphPath.findIndex((x) => x === source);
    if (idx !== -1) {
      // The set of elements following source
      const slicedGraphPath = graphPath.slice(idx + 1);
      if (slicedGraphPath.some((x) => x === target)) {
        return { strokeWidth: 2, stroke: 'red' };
      }
    }
    return {};
  }, [source, target, graphPath]);

  return (
    <>
      <BaseEdge
        path={edgePath}
        markerEnd={markerEnd}
        style={{ ...style, ...selectedStyle, ...highlightStyle }}
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
            className={
              theme === 'dark' ? styles.edgeButtonDark : styles.edgeButton
            }
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
