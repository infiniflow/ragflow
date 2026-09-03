/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { memo, useCallback, useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph2D, { type ForceGraphMethods } from 'react-force-graph-2d';
import {
  getNodeColor as defaultGetNodeColor,
  getNodeRadius as defaultGetNodeRadius,
  MinNodeRadius,
} from './node-style';
import {
  type ArtifactForceGraphProps,
  type ArtifactGraphLink,
  type ArtifactGraphNode,
} from './types';
import { useArtifactGraphData } from './use-artifact-graph-data';
import { useCenterGravity } from './use-center-gravity';
import { useContainerDimensions } from './use-container-dimensions';
import { useGraphHighlight } from './use-graph-highlight';
import { defaultMapNodeToValue } from './utils';

const defaultGetNodeId = (node: IArtifactGraphEntity) => node.slug;

const nodeCanvasObjectMode = () => 'after' as const;

function ArtifactForceGraph<TNodeValue = IArtifactGraphEntity>({
  data,
  show = true,
  onNodeClick,
  mapNodeToValue = defaultMapNodeToValue as (
    node: IArtifactGraphEntity,
  ) => TNodeValue,
  getNodeId = defaultGetNodeId,
  getNodeColor = defaultGetNodeColor,
  getNodeRadius = defaultGetNodeRadius,
  highlightNodeId,
  totalEntities,
  returnedEntities,
}: ArtifactForceGraphProps<TNodeValue>) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<ArtifactGraphNode> | undefined>(
    undefined,
  );
  const hasFittedRef = useRef(false);
  const dimensions = useContainerDimensions(containerRef, show);
  const hasDimensions = dimensions.width > 0 && dimensions.height > 0;

  const graphData = useArtifactGraphData({
    data,
    getNodeId,
    getNodeColor,
    getNodeRadius,
  });

  // Resolve the controlled id back to a node object reference (highlighting relies on the node's __neighbors/__links)
  const pinnedNode = useMemo(
    () =>
      highlightNodeId
        ? ((graphData.nodes.find((node) => node.id === highlightNodeId) ??
            null) as ArtifactGraphNode | null)
        : null,
    [graphData, highlightNodeId],
  );

  const {
    handleNodeHover,
    getNodeColor: nodeColor,
    getLinkColor,
    getLinkWidth,
    paintNode,
  } = useGraphHighlight(containerRef, pinnedNode);

  useEffect(() => {
    hasFittedRef.current = false;
  }, [graphData]);

  useCenterGravity(fgRef, hasDimensions);

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

  const nodeVal = useCallback(
    (node: ArtifactGraphNode) => node.__radius ?? MinNodeRadius,
    [],
  );

  // Hover tooltip shows the entity description; empty string hides it
  const getNodeLabel = useCallback(
    (node: ArtifactGraphNode) => node.description ?? '',
    [],
  );

  // Empty label hides the tooltip, so relations without a type show nothing
  const getLinkLabel = useCallback(
    (link: ArtifactGraphLink) => link.type ?? '',
    [],
  );

  return (
    <div
      ref={containerRef}
      className={cn('relative flex-1 min-h-0 h-full', !show && 'hidden')}
    >
      {hasDimensions && (
        <ForceGraph2D
          ref={fgRef}
          width={dimensions.width}
          height={dimensions.height}
          graphData={graphData}
          nodeRelSize={1}
          nodeColor={nodeColor}
          nodeVal={nodeVal}
          cooldownTicks={100}
          nodeLabel={getNodeLabel}
          autoPauseRedraw={false}
          onEngineStop={handleEngineStop}
          onNodeClick={handleNodeClick}
          onNodeHover={handleNodeHover}
          nodeCanvasObject={paintNode}
          nodeCanvasObjectMode={nodeCanvasObjectMode}
          linkColor={getLinkColor}
          linkWidth={getLinkWidth}
          linkLabel={getLinkLabel}
        />
      )}
      {totalEntities !== undefined && returnedEntities !== undefined && (
        <div className="absolute top-2 right-2 z-10 rounded-md border border-border-button bg-bg-card px-2 py-1 text-xs text-text-secondary">
          {t('knowledgeCompilation.graphEntityCount', {
            returned: returnedEntities,
            total: totalEntities,
          })}
        </div>
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
