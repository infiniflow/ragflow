import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import isEmpty from 'lodash/isEmpty';
import { memo, useCallback, useEffect, useMemo, useRef } from 'react';
import ForceGraph2D, { type ForceGraphMethods } from 'react-force-graph-2d';
import { type ArtifactForceGraphProps } from './types';
import { useContainerDimensions } from './use-container-dimensions';
import { defaultMapNodeToValue, renderNodeLabel } from './utils';

function ArtifactForceGraph<TNodeValue = IArtifactGraphEntity>({
  data,
  show = true,
  onNodeClick,
  mapNodeToValue = defaultMapNodeToValue as (
    node: IArtifactGraphEntity,
  ) => TNodeValue,
  getNodeId = (node) => node.slug,
}: ArtifactForceGraphProps<TNodeValue>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<IArtifactGraphEntity> | undefined>(
    undefined,
  );
  const hasFittedRef = useRef(false);
  const dimensions = useContainerDimensions(containerRef, show);

  const graphData = useMemo(() => {
    if (isEmpty(data) || !data) {
      return { nodes: [], links: [] };
    }

    const nodes = (data.entities || []).map((entity) => ({
      ...entity,
      id: getNodeId(entity),
    }));

    const links = (data.relations || []).map((relation) => ({
      source: relation.from,
      target: relation.to,
    }));

    return { nodes, links };
  }, [data, getNodeId]);

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
          nodeAutoColorBy="type"
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
