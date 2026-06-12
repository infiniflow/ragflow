import { Button } from '@/components/ui/button';
import { useFetchDatasetArtifactGraph } from '@/hooks/use-dataset-artifact-request';
import {
  ArtifactGraphEntity,
  ArtifactGraphRelation,
} from '@/interfaces/database/dataset-artifact';
import { X } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph from '../knowledge-graph/force-graph';

/**
 * Project the artifact REDUCE payload onto the (id-keyed) shape the
 * existing knowledge-graph ``ForceGraph`` renderer (antv/g6) expects.
 *
 * Mapping:
 *   entity.name          -> node.id        (force-graph treats id as label)
 *   entity.type          -> node.entity_type  (palette grouping + tooltip)
 *   entity.mention_count -> node.rank, node.weight
 *                                          (rank drives size, weight shows in tooltip)
 *   entity.aliases       -> node.description (secondary line in the tooltip)
 *
 *   relation.from -> edge.source
 *   relation.to   -> edge.target
 *   relation.type -> edge.description       (surfaced in the edge tooltip)
 *
 * Edges that reference unknown entities are dropped — antv/g6 throws on
 * dangling endpoints.
 */
function toForceGraphShape(
  entities: ArtifactGraphEntity[],
  relations: ArtifactGraphRelation[],
): { nodes: any[]; edges: any[] } {
  const nodes = entities.map((e) => ({
    id: e.name,
    entity_type: e.type,
    rank: e.mention_count ?? 1,
    weight: e.mention_count ?? 1,
    description:
      e.aliases && e.aliases.length > 0
        ? `aliases: ${e.aliases.join(', ')}`
        : undefined,
  }));

  const known = new Set(nodes.map((n) => n.id));
  const edges = relations
    .filter((r) => known.has(r.from) && known.has(r.to))
    .map((r) => ({
      source: r.from,
      target: r.to,
      description: r.type,
      weight: 1,
    }));

  return { nodes, edges };
}

export function ArtifactGraph({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const { data: graph, loading } = useFetchDatasetArtifactGraph(true);

  const graphData = useMemo(
    () => toForceGraphShape(graph.entities, graph.relations),
    [graph],
  );

  const isEmpty = !loading && graphData.nodes.length === 0;

  return (
    <div className="flex-1 flex flex-col relative bg-bg-base">
      <header className="px-4 py-2 border-b border-border-button flex items-center justify-between">
        <h3 className="text-sm font-medium text-text-primary">
          {t('artifact.graphTitle')}
          <span className="ml-2 text-xs text-text-secondary">
            {t('artifact.graphCounts', {
              entities: graph.entities.length,
              relations: graph.relations.length,
            })}
          </span>
        </h3>
        <Button variant="ghost" size="sm" onClick={onClose} aria-label="Close">
          <X className="size-4" />
          {t('common.close')}
        </Button>
      </header>

      {isEmpty ? (
        <div className="flex-1 flex items-center justify-center text-text-secondary">
          {t('artifact.graphEmpty')}
        </div>
      ) : loading && graphData.nodes.length === 0 ? (
        <div className="flex-1 flex items-center justify-center text-text-secondary">
          {t('common.loading')}
        </div>
      ) : (
        <div className="flex-1 min-h-0 relative">
          <ForceGraph data={graphData} show />
        </div>
      )}
    </div>
  );
}
