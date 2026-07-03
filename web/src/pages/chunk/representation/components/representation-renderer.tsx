import ArtifactForceGraph from '@/components/artifact-force-graph';
import { IndentedTree } from '@/components/indented-tree/indented-tree';
import { TreeView, type TreeDataItem } from '@/components/ui/tree-view';
import { CompilationTemplateKind } from '@/constants/compilation';
import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import {
  type IStructureGraphTemplate,
  type StructureTemplateKind,
} from '@/interfaces/database/document-structure';
import { Graph as G6Graph } from '@antv/g6';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  adaptKnowledgeGraphToForceGraph,
  adaptMindMapToIndentedTree,
  adaptPageIndexToTreeData,
  adaptTreeToTreeData,
} from '../utils/adapters';

export interface ClickableNode {
  id: string;
  name?: string;
  source_chunk_ids?: string[];
}

interface RepresentationRendererProps {
  template?: IStructureGraphTemplate;
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

  const handleMindMapNodeClick = useCallback(
    (evt: any) => {
      const model = evt.target?.getModel?.() || evt.item?.getModel?.();
      const chunkIds = model?.source_chunk_ids;
      if (chunkIds?.length) {
        onNodeClick?.({
          id: model.id,
          name: model.id,
          source_chunk_ids: chunkIds,
        });
      }
    },
    [onNodeClick],
  );

  const handleMindMapRender = useCallback(
    (graph: G6Graph) => {
      graph.on('node:click', handleMindMapNodeClick);
    },
    [handleMindMapNodeClick],
  );

  const getArtifactNodeName = useCallback(
    (node: IArtifactGraphEntity) => node.name,
    [],
  );

  if (!template) {
    return null;
  }

  switch (template.kind) {
    case CompilationTemplateKind.PageIndex:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView
            data={adaptPageIndexToTreeData(template)}
            expandAll
            onSelectChange={handleTreeItemClick}
          />
        </div>
      );
    case CompilationTemplateKind.Tree:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView
            data={adaptTreeToTreeData(template)}
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
    case CompilationTemplateKind.MindMap:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <IndentedTree
            data={adaptMindMapToIndentedTree(template)}
            onRender={handleMindMapRender}
          />
        </div>
      );
    default:
      return <UnsupportedPlaceholder kind={template.kind} />;
  }
}
