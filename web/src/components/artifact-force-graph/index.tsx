import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { memo, useCallback, useEffect, useRef } from 'react';
import ForceGraph2D, { type ForceGraphMethods } from 'react-force-graph-2d';
import { renderNodeLabel } from './node-label';
import {
  getNodeColor as defaultGetNodeColor,
  getNodeRadius as defaultGetNodeRadius,
  MinNodeRadius,
} from './node-style';
import { type ArtifactForceGraphProps, type ArtifactGraphNode } from './types';
import { useArtifactGraphData } from './use-artifact-graph-data';
import { useContainerDimensions } from './use-container-dimensions';
import { defaultMapNodeToValue } from './utils';

function ArtifactForceGraph<TNodeValue = IArtifactGraphEntity>({
  data,
  show = true,
  onNodeClick,
  mapNodeToValue = defaultMapNodeToValue as (
    node: IArtifactGraphEntity,
  ) => TNodeValue,
  getNodeId = (node) => node.slug,
  getNodeColor = defaultGetNodeColor,
  getNodeRadius = defaultGetNodeRadius,
}: ArtifactForceGraphProps<TNodeValue>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<ArtifactGraphNode> | undefined>(
    undefined,
  );
  const hasFittedRef = useRef(false);
  const dimensions = useContainerDimensions(containerRef, show);

  const graphData = useArtifactGraphData({
    data,
    getNodeId,
    getNodeColor,
    getNodeRadius,
  });

  useEffect(() => {
    hasFittedRef.current = false;
  }, [graphData]);

  const handleEngineStop = useCallback(() => {
    if (!hasFittedRef.current && fgRef.current) {
      fgRef.current.zoomToFit(400);
      hasFittedRef.current = true;
    }
  }, []);

  const handleNodeClick = useCallback(
    (node: IArtifactGraphEntity) => {
      onNodeClick?.(mapNodeToValue(node));
    },
    [onNodeClick, mapNodeToValue],
  );

  const getLinkColor = useCallback(() => {
    if (typeof window === 'undefined' || !containerRef.current) {
      return '#b2b5b7';
    }
    return window
      .getComputedStyle(containerRef.current)
      .getPropertyValue('--text-disabled')
      .trim();
  }, []);

  const nodeColor = useCallback((node: ArtifactGraphNode) => node.__color, []);

  const nodeVal = useCallback(
    (node: ArtifactGraphNode) => node.__radius ?? MinNodeRadius,
    [],
  );

  return (
    <div
      ref={containerRef}
      className={cn('flex-1 min-h-0 h-full', !show && 'hidden')}
    >
      {dimensions.width > 0 && dimensions.height > 0 && (
        <ForceGraph2D
          ref={fgRef}
          width={dimensions.width}
          height={dimensions.height}
          graphData={graphData}
          nodeRelSize={1}
          nodeColor={nodeColor}
          nodeVal={nodeVal}
          cooldownTicks={100}
          nodeLabel={''}
          onEngineStop={handleEngineStop}
          onNodeClick={handleNodeClick}
          nodeCanvasObject={renderNodeLabel}
          nodeCanvasObjectMode={() => 'after'}
          linkColor={getLinkColor}
        />
      )}
    </div>
  );
}

const MemoizedArtifactForceGraph = memo(ArtifactForceGraph) as <
  TNodeValue = IArtifactGraphEntity,
>(
  props: ArtifactForceGraphProps<TNodeValue>,
) => React.ReactElement;

export { MemoizedArtifactForceGraph as ArtifactForceGraph };
export default MemoizedArtifactForceGraph;
