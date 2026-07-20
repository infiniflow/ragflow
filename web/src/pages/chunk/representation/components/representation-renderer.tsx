import ArtifactForceGraph from '@/components/artifact-force-graph';
import { TreeView, type TreeDataItem } from '@/components/ui/tree-view';
import { CompilationTemplateKind } from '@/constants/compilation';
import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import {
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
  filterTreeDataByKeyword,
} from '../utils/adapters';
import MindMapG6Graph from './mindmap-g6-graph';
import TimelineX6Graph from './timeline-x6-graph';

export interface ClickableNode {
  id: string;
  name?: string;
  source_chunk_ids?: string[];
}

interface RepresentationRendererProps {
  template?: IStructureGraphTemplate;
  searchKeyword?: string;
  onNodeClick?: (node: ClickableNode) => void;
}

function UnsupportedPlaceholder({ kind }: { kind: StructureTemplateKind }) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center justify-center h-full text-text-secondary">
      {t('chunk.representationUnsupported', {
        kind,
        defaultValue: 'This representation type is not supported yet.',
      })}
    </div>
  );
}

export function RepresentationRenderer({
  template,
  searchKeyword = '',
  onNodeClick,
}: RepresentationRendererProps) {
  const handleTreeItemClick = useCallback(
    (item: TreeDataItem | undefined) => {
      if (item?.source_chunk_ids?.length) {
        onNodeClick?.({
          id: item.id,
          name: item.name,
          source_chunk_ids: item.source_chunk_ids,
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

  const filteredTreeData = useMemo<TreeDataItem[]>(() => {
    if (!template) return [];
    if (template.kind === CompilationTemplateKind.PageIndex) {
      const data = adaptPageIndexToTreeData(template);
      return searchKeyword.trim()
        ? filterTreeDataByKeyword(data, searchKeyword)
        : data;
    }
    if (template.kind === CompilationTemplateKind.Tree) {
      const data = adaptTreeToTreeData(template);
      return searchKeyword.trim()
        ? filterTreeDataByKeyword(data, searchKeyword)
        : data;
    }
    return [];
  }, [template, searchKeyword]);

  if (!template) {
    return null;
  }

  switch (template.kind) {
    case CompilationTemplateKind.PageIndex:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView
            data={filteredTreeData}
            expandAll
            onSelectChange={handleTreeItemClick}
          />
        </div>
      );
    case CompilationTemplateKind.Tree:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView
            data={filteredTreeData}
            expandAll
            onSelectChange={handleTreeItemClick}
          />
        </div>
      );
    case CompilationTemplateKind.KnowledgeGraph:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <ArtifactForceGraph
            data={adaptKnowledgeGraphToForceGraph(template)}
            show
            getNodeId={getArtifactNodeName}
            onNodeClick={handleArtifactNodeClick}
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
            data={adaptKnowledgeGraphToForceGraph(template)}
            show
            getNodeId={getArtifactNodeName}
            onNodeClick={handleArtifactNodeClick}
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
            data={adaptKnowledgeGraphToForceGraph(template)}
            show
            getNodeId={getArtifactNodeName}
            onNodeClick={handleArtifactNodeClick}
          />
        </div>
      );
    default:
      return <UnsupportedPlaceholder kind={template.kind} />;
  }
}
