import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Spin } from '@/components/ui/spin';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useShowDeleteConfirm } from '@/hooks/common-hooks';
import {
  IDocumentStructureGraph,
  IDocumentStructureTemplate,
  useDeleteDocumentStructureGraph,
} from '@/hooks/use-chunk-request';
import ForceGraph from '@/pages/dataset/knowledge-graph/force-graph';
import TimelineGraph from '@/pages/dataset/knowledge-graph/timeline-graph';
import TreeGraph from '@/pages/dataset/knowledge-graph/tree-graph';
import { Trash2, X } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';

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

/**
 * Project a ``timeline``-kind template onto the vertical-waterfall
 * shape. Same id-by-name convention as the peer adapters; ``rank`` is
 * fed from ``mention_count`` so the renderer can scale node size.
 * Dangling-endpoint edges are dropped.
 */
function toTimelineShape(template: IDocumentStructureTemplate | undefined): {
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
    description: entity.discription || entity.description || undefined,
    aliases: entity.aliases ?? [],
  }));
  const known = new Set(nodes.map((n) => n.id));
  const edges = relations
    .filter((r) => known.has(r.from) && known.has(r.to))
    .map((r) => ({
      source: r.from,
      target: r.to,
      description: r.type,
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
   * Pick the right renderer for the active template's kind:
   *   ``page_index`` / ``raptor`` → tree (left-to-right hierarchy)
   *   ``timeline`` / ``list``      → vertical-waterfall flowchart. Both
   *                                  kinds are strict linear chains
   *                                  (enforced post-extract by
   *                                  validate_and_correct_chain), so
   *                                  they read the same way visually.
   *   anything else                → force-directed
   */
  type Renderer = 'tree' | 'timeline' | 'force';
  const renderer: Renderer = useMemo(() => {
    const k = normalizeKind(activeTemplate?.kind);
    if (k === 'page_index' || k === 'raptor') return 'tree';
    if (k === 'timeline' || k === 'list') return 'timeline';
    return 'force';
  }, [activeTemplate?.kind]);

  const forceGraphData = useMemo(
    () =>
      renderer === 'force'
        ? toForceGraphShape(activeTemplate)
        : { nodes: [], edges: [] },
    [activeTemplate, renderer],
  );
  const treeGraphData = useMemo(
    () =>
      renderer === 'tree'
        ? toTreeShape(activeTemplate)
        : { nodes: [], edges: [] },
    [activeTemplate, renderer],
  );
  const timelineGraphData = useMemo(
    () =>
      renderer === 'timeline'
        ? toTimelineShape(activeTemplate)
        : { nodes: [], edges: [] },
    [activeTemplate, renderer],
  );

  const hasAny = nonEmptyTemplates.length > 0;
  const isEmpty = !loading && !hasAny;
  const activeNodeCount =
    renderer === 'tree'
      ? treeGraphData.nodes.length
      : renderer === 'timeline'
        ? timelineGraphData.nodes.length
        : forceGraphData.nodes.length;
  const { deleteStructureGraph, loading: deleting } =
    useDeleteDocumentStructureGraph();
  const showDeleteConfirm = useShowDeleteConfirm();

  const handleDelete = useCallback(() => {
    if (!activeTemplate?.template_id) return;
    void showDeleteConfirm({
      title: `Delete ${activeTemplate.template_name}?`,
      content: 'This structure graph tab will be removed from this document.',
      onOk: async () => {
        const code = await deleteStructureGraph({
          template_id: activeTemplate.template_id,
        });
        if (code === 0) {
          const next = nonEmptyTemplates.find(
            (t) => t.template_id !== activeTemplate.template_id,
          );
          setActiveId(next?.template_id ?? '');
        }
        return code;
      },
    });
  }, [
    activeTemplate,
    deleteStructureGraph,
    nonEmptyTemplates,
    showDeleteConfirm,
  ]);

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
        <div className="flex items-center gap-1 shrink-0">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleDelete}
            disabled={!activeTemplate || deleting}
            loading={deleting}
            title="Delete structure graph"
          >
            <Trash2 className="size-4" />
            Delete
          </Button>
          <Button variant="ghost" size="sm" onClick={onClose}>
            <X className="size-4" />
            Close
          </Button>
        </div>
      </header>
      <Spin spinning={loading} className="flex-1 h-0">
        {isEmpty ? (
          <div className="flex h-full items-center justify-center text-sm text-text-secondary">
            No generated structure graph.
          </div>
        ) : activeTemplate && activeNodeCount > 0 ? (
          renderer === 'tree' ? (
            <TreeGraph
              data={treeGraphData}
              show
              rootId={ARTIFACT_TREE_ROOT_ID}
            />
          ) : renderer === 'timeline' ? (
            <TimelineGraph data={timelineGraphData} show />
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
