import { Button } from '@/components/ui/button';
import { Spin } from '@/components/ui/spin';
import { IDocumentStructureGraph } from '@/hooks/use-chunk-request';
import ForceGraph from '@/pages/dataset/knowledge-graph/force-graph';
import { X } from 'lucide-react';
import { useMemo } from 'react';

function toForceGraphShape(graph: IDocumentStructureGraph): {
  nodes: any[];
  edges: any[];
} {
  const nodes = graph.entities.map((entity) => ({
    id: entity.name,
    entity_type: entity.type || 'other',
    rank: entity.mention_count ?? 1,
    weight: entity.mention_count ?? 1,
    description:
      entity.discription || entity.description || entity.aliases?.join(', '),
  }));

  const known = new Set(nodes.map((node) => node.id));
  const edges = graph.relations
    .filter((relation) => known.has(relation.from) && known.has(relation.to))
    .map((relation) => ({
      source: relation.from,
      target: relation.to,
      description: relation.type,
      weight: 1,
    }));

  return { nodes, edges };
}

export function DocumentStructureGraph({
  data,
  loading,
  onClose,
}: {
  data: IDocumentStructureGraph;
  loading: boolean;
  onClose: () => void;
}) {
  const graphData = useMemo(() => toForceGraphShape(data), [data]);
  const isEmpty = !loading && graphData.nodes.length === 0;

  return (
    <div className="absolute inset-0 z-10 flex flex-col bg-bg-base">
      <header className="flex items-center justify-between border-b border-border-button px-4 py-2">
        <div className="text-sm font-medium text-text-primary">
          Structure graph
          <span className="ml-2 text-xs text-text-secondary">
            {data.entities.length} entities / {data.relations.length} relations
          </span>
        </div>
        <Button variant="ghost" size="sm" onClick={onClose}>
          <X className="size-4" />
          Close
        </Button>
      </header>
      <Spin spinning={loading} className="flex-1 h-0">
        {isEmpty ? (
          <div className="flex h-full items-center justify-center text-sm text-text-secondary">
            No generated structure graph.
          </div>
        ) : (
          <ForceGraph data={graphData} show />
        )}
      </Spin>
    </div>
  );
}
