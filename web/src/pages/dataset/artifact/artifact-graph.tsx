import { Button } from '@/components/ui/button';
import { useFetchDatasetArtifactGraph } from '@/hooks/use-dataset-artifact-request';
import {
  ArtifactGraphEntity,
  ArtifactGraphRelation,
} from '@/interfaces/database/dataset-artifact';
import { Undo2, X } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
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
  entities: ArtifactGraphEntity[] | undefined,
  relations: ArtifactGraphRelation[] | undefined,
): { nodes: any[]; edges: any[] } {
  // Defensive: useQuery's data can briefly be undefined on first paint
  // even with ``initialData`` (StrictMode, hot reload), and an upstream
  // backend hiccup can return a partial payload. Treat anything that
  // isn't an array as the empty list rather than crashing later inside
  // ForceGraph.
  const entitiesSafe = Array.isArray(entities) ? entities : [];
  const relationsSafe = Array.isArray(relations) ? relations : [];

  /**
   * Node id is the page **name** (title). The shared antv/g6
   * ``ForceGraph`` renderer uses ``d.id`` as the visible label, so the
   * id has to be the human-readable string. Slugs (which are unique by
   * construction) are still used for relation endpoints upstream — we
   * resolve them to names via a slug→name lookup below and drop any
   * edge whose endpoint can't be matched.
   *
   * The tooltip description prefers the page summary (``e.description``);
   * aliases are appended on a second line when present.
   */
  const slugToName = new Map<string, string>();
  for (const e of entitiesSafe) {
    if (e.slug && e.name) {
      slugToName.set(e.slug, e.name);
    }
  }

  const nodes = entitiesSafe.map((e) => {
    const tooltipLines: string[] = [];
    if (e.description) tooltipLines.push(e.description);
    if (e.aliases && e.aliases.length > 0) {
      tooltipLines.push(`aliases: ${e.aliases.join(', ')}`);
    }
    // ``weight`` (outlink count, set by the backend) is the canvas
    // signal of importance. The KG ForceGraph drives node size from
    // ``rank``, so feed weight into rank too. Legacy payloads that
    // only carry ``mention_count`` still degrade gracefully via the
    // nullish chain.
    const importance = e.weight ?? e.mention_count ?? 1;
    return {
      id: e.name || e.slug,
      entity_type: e.type,
      rank: importance,
      weight: importance,
      description:
        tooltipLines.length > 0 ? tooltipLines.join('\n') : undefined,
    };
  });

  const known = new Set(nodes.map((n) => n.id));
  const edges = relationsSafe
    .map((r) => {
      // ``r.from`` / ``r.to`` are slugs in the backend payload; map
      // each to its display name. Fall back to the raw value so a
      // legacy name-keyed payload still resolves.
      const src = slugToName.get(r.from) ?? r.from;
      const tgt = slugToName.get(r.to) ?? r.to;
      if (!known.has(src) || !known.has(tgt)) return null;
      return {
        source: src,
        target: tgt,
        description: r.type,
        weight: 1,
      };
    })
    .filter(
      (
        e,
      ): e is {
        source: string;
        target: string;
        description?: string;
        weight: number;
      } => e !== null,
    );

  return { nodes, edges };
}

export function ArtifactGraph({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  // ``centerSlug`` drives the incremental loader: null = overview
  // (top-weighted entities), a slug = subgraph centred on that node.
  const [centerSlug, setCenterSlug] = useState<string | null>(null);
  const { data: graph, loading } = useFetchDatasetArtifactGraph(
    true,
    centerSlug,
  );

  const graphData = useMemo(
    () => toForceGraphShape(graph?.entities, graph?.relations),
    [graph],
  );

  /**
   * Reverse the node.id → entity.slug mapping for the click handler.
   * ``toForceGraphShape`` uses ``e.name || e.slug`` as ``node.id`` so the
   * canvas shows human-readable labels; the API expects slugs back.
   */
  const nameToSlug = useMemo(() => {
    const m = new Map<string, string>();
    const entities = Array.isArray(graph?.entities) ? graph.entities : [];
    for (const e of entities) {
      const key = e.name || e.slug;
      if (key && e.slug) m.set(key, e.slug);
    }
    return m;
  }, [graph]);

  const onNodeClick = useCallback(
    (id: string) => {
      const slug = nameToSlug.get(id);
      if (slug && slug !== centerSlug) setCenterSlug(slug);
    },
    [nameToSlug, centerSlug],
  );

  const hasNodes = graphData.nodes.length > 0;
  const entityCount = Array.isArray(graph?.entities)
    ? graph.entities.length
    : 0;
  const relationCount = Array.isArray(graph?.relations)
    ? graph.relations.length
    : 0;

  return (
    <div className="flex-1 flex flex-col relative bg-bg-base">
      <header className="px-4 py-2 border-b border-border-button flex items-center justify-between">
        <h3 className="text-sm font-medium text-text-primary">
          {t('artifact.graphTitle')}
          <span className="ml-2 text-xs text-text-secondary">
            {t('artifact.graphCounts', {
              entities: entityCount,
              relations: relationCount,
            })}
          </span>
        </h3>
        <div className="flex items-center gap-2">
          {centerSlug ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setCenterSlug(null)}
            >
              <Undo2 className="size-4" />
              {t('artifact.graphResetView')}
            </Button>
          ) : null}
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            aria-label="Close"
          >
            <X className="size-4" />
            {t('common.close')}
          </Button>
        </div>
      </header>

      {!hasNodes ? (
        // Guard ForceGraph against the empty-payload case explicitly.
        // antv/g6's combo-combined layout chokes on a 0-node input, and
        // the existing util.buildNodesAndCombos receives no nodes to
        // group, so just short-circuit before mounting it at all.
        <div className="flex-1 flex items-center justify-center text-text-secondary">
          {loading ? t('common.loading') : t('artifact.graphEmpty')}
        </div>
      ) : (
        <div className="flex-1 min-h-0 relative">
          <ForceGraph data={graphData} show onNodeClick={onNodeClick} />
        </div>
      )}
    </div>
  );
}
