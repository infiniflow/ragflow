import ArtifactForceGraph from '@/components/artifact-force-graph';
import { IndentedTree } from '@/components/indented-tree/indented-tree';
import { TreeView } from '@/components/ui/tree-view';
import { CompilationTemplateKind } from '@/constants/compilation';
import {
  type IStructureGraphTemplate,
  type StructureTemplateKind,
} from '@/interfaces/database/document-structure';
import { useTranslation } from 'react-i18next';
import {
  adaptKnowledgeGraphToForceGraph,
  adaptMindMapToIndentedTree,
  adaptPageIndexToTreeData,
  adaptTreeToTreeData,
} from '../utils/adapters';

interface RepresentationRendererProps {
  template?: IStructureGraphTemplate;
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
}: RepresentationRendererProps) {
  if (!template) {
    return null;
  }

  switch (template.kind) {
    case CompilationTemplateKind.PageIndex:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView data={adaptPageIndexToTreeData(template)} expandAll />
        </div>
      );
    case CompilationTemplateKind.Tree:
      return (
        <div className="mt-6 overflow-auto scrollbar-auto">
          <TreeView data={adaptTreeToTreeData(template)} expandAll />
        </div>
      );
    case CompilationTemplateKind.KnowledgeGraph:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <ArtifactForceGraph
            data={adaptKnowledgeGraphToForceGraph(template)}
            show
            getNodeId={(node) => node.name}
          />
        </div>
      );
    case CompilationTemplateKind.MindMap:
      return (
        <div className="mt-6 flex-1 min-h-0">
          <IndentedTree data={adaptMindMapToIndentedTree(template)} />
        </div>
      );
    default:
      return <UnsupportedPlaceholder kind={template.kind} />;
  }
}
