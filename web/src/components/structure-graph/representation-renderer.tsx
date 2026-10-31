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

import ArtifactForceGraph from '@/components/artifact-force-graph';
import { TreeView, type TreeDataItem } from '@/components/ui/tree-view';
import { CompilationTemplateKind } from '@/constants/compilation';
import {
  type IArtifactGraph,
  type IArtifactGraphEntity,
} from '@/interfaces/database/dataset';
import {
  type IClaimEvidence,
  type IStructureGraphTemplate,
  type StructureTemplateKind,
} from '@/interfaces/database/document-structure';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  adaptKnowledgeGraphToForceGraph,
  adaptPageIndexToTreeData,
  adaptTimelineToX6Data,
  adaptTreeToTreeData,
} from './adapters';
import MindMapG6Graph from './mindmap-g6-graph';
import OntologyModelGraph from './ontology-model-graph';
import type { OntologyDetailPanelState } from './ontology-model-graph/detail-panel';
import TimelineX6Graph from './timeline-x6-graph';

export interface ClickableNode {
  id: string;
  name?: string;
  source_chunk_ids?: string[];
  /** Tree leaves only: whether this node has child clusters. */
  hasChildren?: boolean;
  /** Leaf claim badge (tree slot). 0/undefined ⇒ nothing for the claims panel. */
  badge?: React.ReactNode;
  /** page_index fact/conclusion: gate-verified quotes for the detail panel. */
  description?: string;
  evidence?: IClaimEvidence[];
}

const EmptyForceGraphData: IArtifactGraph = { entities: [], relations: [] };

interface RepresentationRendererProps {
  template?: IStructureGraphTemplate;
  onNodeClick?: (node: ClickableNode) => void;
  highlightNodeId?: string | null;
  totalEntities?: number;
  returnedEntities?: number;
  /**
   * Scope for the ontology model graph's drill-down (only that representation
   * queries the index directly). Without it the graph still renders, but its
   * class and property panels do not offer the drill-down.
   */
  datasetId?: string;
  /**
   * The document the page is showing, when it is a document page. Passed through
   * so the ontology drill-down counts and lists the SAME scope the graph's
   * numbers came from.
   */
  documentId?: string;
  templateId?: string;
  /**
   * Publishes the class the ontology graph has selected, for the host to render
   * in its own column — the graph must not draw the panel itself, because it
   * sits in a resizable column whose divider does not know about a panel nested
   * inside it.
   */
  onOntologyDetailChange?: (state: OntologyDetailPanelState | null) => void;
}

function UnsupportedPlaceholder({ kind }: { kind: StructureTemplateKind }) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center justify-center h-full text-text-secondary">
      {t('knowledgeCompilation.representationUnsupported', {
        kind,
        defaultValue: 'This representation type is not supported yet.',
      })}
    </div>
  );
}

export function RepresentationRenderer({
  template,
  onNodeClick,
  highlightNodeId,
  totalEntities,
  returnedEntities,
  datasetId,
  documentId,
  templateId,
  onOntologyDetailChange,
}: RepresentationRendererProps) {
  const handleTreeItemClick = useCallback(
    (item: TreeDataItem | undefined) => {
      if (item?.source_chunk_ids?.length) {
        onNodeClick?.({
          id: item.id,
          name: item.name,
          source_chunk_ids: item.source_chunk_ids,
          // Tree leaves are where the claims UI attaches; branches are pure
          // structure and clicking them keeps the old behaviour.
          hasChildren: !!item.children?.length,
          badge: item.badge,
          description: item.description,
          evidence: item.evidence,
        });
      }
    },
    [onNodeClick],
  );

  const handleArtifactNodeClick = useCallback(
    (node: IArtifactGraphEntity) => {
      if (node.source_chunk_ids?.length) {
        onNodeClick?.({
          id: node.slug,
          name: node.name,
          source_chunk_ids: node.source_chunk_ids,
        });
      }
    },
    [onNodeClick],
  );

  const handleTimelineNodeClick = useCallback(
    (node: { id: string; name: string; source_chunk_ids?: string[] }) => {
      if (node.source_chunk_ids?.length) {
        onNodeClick?.(node);
      }
    },
    [onNodeClick],
  );

  const handleMindMapNodeClick = useCallback(
    (node: ClickableNode) => {
      if (node.source_chunk_ids?.length) {
        onNodeClick?.(node);
      }
    },
    [onNodeClick],
  );

  const getArtifactNodeName = useCallback(
    (node: IArtifactGraphEntity) => node.name,
    [],
  );

  const treeData = useMemo<TreeDataItem[]>(() => {
    if (!template) return [];
    if (template.kind === CompilationTemplateKind.PageIndex) {
      return adaptPageIndexToTreeData(template);
    }
    if (template.kind === CompilationTemplateKind.Tree) {
      return adaptTreeToTreeData(template);
    }
    return [];
  }, [template]);

  // Keep a stable reference across re-renders so the memoized ArtifactForceGraph
  // does not restart its force simulation when only highlightNodeId changes
  const forceGraphData = useMemo<IArtifactGraph>(
    () =>
      template
        ? adaptKnowledgeGraphToForceGraph(template)
        : EmptyForceGraphData,
    [template],
  );

  if (!template) {
    return null;
  }

  switch (template.kind) {
    case CompilationTemplateKind.PageIndex:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView
            data={treeData}
            expandAll
            onSelectChange={handleTreeItemClick}
          />
        </div>
      );
    case CompilationTemplateKind.Tree:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView
            data={treeData}
            expandAll
            onSelectChange={handleTreeItemClick}
          />
        </div>
      );
    case CompilationTemplateKind.KnowledgeGraph:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <ArtifactForceGraph
            data={forceGraphData}
            show
            getNodeId={getArtifactNodeName}
            onNodeClick={handleArtifactNodeClick}
            highlightNodeId={highlightNodeId}
            totalEntities={totalEntities}
            returnedEntities={returnedEntities}
          />
        </div>
      );
    case CompilationTemplateKind.Ontology:
      // Class-level model graph: classes are the nodes and properties are the
      // domain -> range edges, so it is deliberately not routed through
      // ArtifactForceGraph (which draws the instance graph).
      return (
        <div className="mt-6 flex-1 min-h-0">
          <OntologyModelGraph
            graph={template.ontology}
            highlightClass={highlightNodeId ?? null}
            datasetId={datasetId}
            documentId={documentId}
            templateId={templateId}
            onClassDetailChange={onOntologyDetailChange}
          />
        </div>
      );
    case CompilationTemplateKind.Timeline:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <TimelineX6Graph
            data={adaptTimelineToX6Data(template)}
            show
            onNodeClick={handleTimelineNodeClick}
          />
        </div>
      );
    case CompilationTemplateKind.MindMap:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <MindMapG6Graph
            template={template}
            show
            onNodeClick={handleMindMapNodeClick}
          />
        </div>
      );
    case CompilationTemplateKind.SessionGraph:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <ArtifactForceGraph
            data={forceGraphData}
            show
            getNodeId={getArtifactNodeName}
            onNodeClick={handleArtifactNodeClick}
            totalEntities={totalEntities}
            returnedEntities={returnedEntities}
          />
        </div>
      );
    case CompilationTemplateKind.SessionEssence:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <MindMapG6Graph
            template={template}
            show
            onNodeClick={handleMindMapNodeClick}
          />
        </div>
      );
    case CompilationTemplateKind.Empty:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <ArtifactForceGraph
            data={forceGraphData}
            show
            getNodeId={getArtifactNodeName}
            onNodeClick={handleArtifactNodeClick}
            totalEntities={totalEntities}
            returnedEntities={returnedEntities}
          />
        </div>
      );
    default:
      return <UnsupportedPlaceholder kind={template.kind} />;
  }
}
