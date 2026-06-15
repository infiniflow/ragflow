import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Spin } from '@/components/ui/spin';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  IDocumentStructureGraph,
  IDocumentStructureTemplate,
} from '@/hooks/use-chunk-request';
import ForceGraph from '@/pages/dataset/knowledge-graph/force-graph';
import TreeGraph from '@/pages/dataset/knowledge-graph/tree-graph';
import { X } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';

/**
 * Normalize a template's ``kind`` string. The backend stamps a few
 * variants over time ("page_index", "pageindex", "PageIndex") — collapse
 * them so the dispatcher can match on a single form.
 */
function normalizeKind(kind: string | undefined): string {
  if (!kind) return '';
  return kind.trim().toLowerCase().replace(/-/g, '_');
}

const ARTIFACT_TREE_ROOT_ID = '__document_structure_root__';

/**
 * Project one template's payload onto the (id-keyed) shape ForceGraph
 * expects. ``description`` is built from the first non-empty of the
 * optional fields the structure pipeline persists.
 */
function toForceGraphShape(template: IDocumentStructureTemplate | undefined): {
  nodes: any[];
  edges: any[];
} {
  if (!template) return { nodes: [], edges: [] };
  const entities = Array.isArray(template.entities) ? template.entities : [];
  const relations = Array.isArray(template.relations) ? template.relations : [];

  const nodes = entities.map((entity) => ({
    id: entity.name,
    entity_type: entity.type || 'other',
    rank: entity.mention_count ?? 1,
    weight: entity.mention_count ?? 1,
    description:
      entity.discription || entity.description || entity.aliases?.join(', '),
  }));

  const known = new Set(nodes.map((node) => node.id));
  const edges = relations
    .filter((relation) => known.has(relation.from) && known.has(relation.to))
    .map((relation) => ({
      source: relation.from,
      target: relation.to,
      description: relation.type,
      weight: 1,
    }));

  return { nodes, edges };
}

/**
 * Project a ``page_index``-kind template onto a tree.
 *
 *   1. Adopt entity.name as the node id (same as the force-graph adapter).
 *   2. A synthetic root node is prepended: ``__document_structure_root__``.
 *   3. Every entity that is not the ``to`` of any relation becomes a
 *      first-level child of the synthetic root.
 *   4. The remaining relations are kept as-is — they form the hierarchy.
 *
 * If a chunk's structure happens to be cyclic (the LLM occasionally
 * emits a cycle), antv/g6's ``compact-box`` still renders something
 * sensible; we don't try to fix that here.
 */
function toTreeShape(template: IDocumentStructureTemplate | undefined): {
  nodes: any[];
  edges: any[];
} {
  if (!template) return { nodes: [], edges: [] };
  const entities = Array.isArray(template.entities) ? template.entities : [];
  const relations = Array.isArray(template.relations) ? template.relations : [];

  const nodes = entities.map((entity) => ({
    id: entity.name,
    entity_type: entity.type || 'title',
    description:
      entity.discription || entity.description || entity.aliases?.join(', '),
  }));
  const known = new Set(nodes.map((n) => n.id));

  const edges = relations
    .filter((r) => known.has(r.from) && known.has(r.to))
    .map((r) => ({
      source: r.from,
      target: r.to,
      description: r.type,
    }));

  // First-level nodes = entities with no incoming edge. These get
  // attached to the synthetic root so the whole thing reads as a single
  // tree rather than a forest of disconnected components.
  const hasIncoming = new Set<string>();
  for (const e of edges) hasIncoming.add(e.target);
  const topLevel = nodes
    .filter((n) => !hasIncoming.has(n.id))
    .map((n) => ({ source: ARTIFACT_TREE_ROOT_ID, target: n.id }));

  const rootNode = {
    id: ARTIFACT_TREE_ROOT_ID,
    entity_type: 'root',
    isRoot: true,
    description: 'Document',
  };

  return {
    nodes: [rootNode, ...nodes],
    edges: [...topLevel, ...edges],
  };
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
  /**
   * Backend already filters out empty templates (zero entities AND
   * zero relations). Belt-and-braces here for hot-reload windows where
   * the cached payload still has old data.
   */
  const nonEmptyTemplates = useMemo(
    () =>
      (data.templates ?? []).filter(
        (t) =>
          (Array.isArray(t.entities) && t.entities.length > 0) ||
          (Array.isArray(t.relations) && t.relations.length > 0),
      ),
    [data.templates],
  );

  const [activeId, setActiveId] = useState<string>(
    nonEmptyTemplates[0]?.template_id ?? '',
  );

  // If the payload changes (e.g. compile finishes mid-view) and the
  // previously-active tab disappears, snap to the first available.
  useEffect(() => {
    if (
      !activeId ||
      !nonEmptyTemplates.some((t) => t.template_id === activeId)
    ) {
      setActiveId(nonEmptyTemplates[0]?.template_id ?? '');
    }
  }, [nonEmptyTemplates, activeId]);

  const activeTemplate = useMemo(
    () => nonEmptyTemplates.find((t) => t.template_id === activeId),
    [activeId, nonEmptyTemplates],
  );

  /**
   * Both ``page_index`` (case/dash variants accepted) and the
   * synthetic ``raptor`` kind are strict hierarchies — render them as
   * left-to-right trees under a synthetic root. Every other kind keeps
   * the force-directed layout.
   */
  const useTreeLayout = useMemo(() => {
    const k = normalizeKind(activeTemplate?.kind);
    return k === 'page_index' || k === 'raptor';
  }, [activeTemplate?.kind]);

  const forceGraphData = useMemo(
    () =>
      useTreeLayout
        ? { nodes: [], edges: [] }
        : toForceGraphShape(activeTemplate),
    [activeTemplate, useTreeLayout],
  );
  const treeGraphData = useMemo(
    () =>
      useTreeLayout ? toTreeShape(activeTemplate) : { nodes: [], edges: [] },
    [activeTemplate, useTreeLayout],
  );

  const hasAny = nonEmptyTemplates.length > 0;
  const isEmpty = !loading && !hasAny;
  const activeNodeCount = useTreeLayout
    ? treeGraphData.nodes.length
    : forceGraphData.nodes.length;

  return (
    <div className="absolute inset-0 z-10 flex flex-col bg-bg-base">
      <header className="flex items-center justify-between gap-3 border-b border-border-button px-4 py-2">
        <div className="flex items-center gap-3 min-w-0">
          <div className="text-sm font-medium text-text-primary shrink-0">
            Structure graph
          </div>
          {hasAny ? (
            <Tabs
              value={activeId}
              onValueChange={setActiveId}
              className="min-w-0"
            >
              <TabsList>
                {nonEmptyTemplates.map((t) => (
                  <TabsTrigger
                    key={t.template_id}
                    value={t.template_id}
                    className="flex items-center gap-2"
                  >
                    <span className="truncate max-w-[14rem]">
                      {t.template_name}
                    </span>
                    {t.kind ? (
                      <Badge variant="secondary" className="text-[10px]">
                        {t.kind}
                      </Badge>
                    ) : null}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          ) : null}
          {activeTemplate ? (
            <span className="text-xs text-text-secondary shrink-0">
              {activeTemplate.entities.length} entities /{' '}
              {activeTemplate.relations.length} relations
            </span>
          ) : null}
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
        ) : activeTemplate && activeNodeCount > 0 ? (
          useTreeLayout ? (
            <TreeGraph
              data={treeGraphData}
              show
              rootId={ARTIFACT_TREE_ROOT_ID}
            />
          ) : (
            <ForceGraph data={forceGraphData} show />
          )
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-text-secondary">
            {loading ? 'Loading…' : 'No data for this template.'}
          </div>
        )}
      </Spin>
    </div>
  );
}
