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

// Synthetic anchor used only when the source data is a forest (multiple
// natural roots). antv/g6's ``compact-box`` requires a single rooted
// tree — without an anchor the layout either stack-overflows or
// produces nodes with undefined positions, which crashes the renderer.
// Single-natural-root data skips this entirely so no synthetic node
// shows up in the canvas. When the anchor IS added, tree-graph.tsx
// renders it invisibly via the ``isSynthetic`` flag.
const FOREST_ANCHOR_ID = '__forest_anchor__';

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
 *   2. Edges that reference unknown endpoints are dropped (antv/g6's
 *      compact-box layout chokes on dangling references).
 *
 * Multiple top-level entities (those with no incoming edge) render as a
 * forest under the layout's implicit virtual root — no synthetic
 * "Document" node is prepended any more, since it just added noise to
 * the canvas. TreeGraph's BFS-for-depth auto-seeds from every node
 * with no incoming edges, so depth shading still works on forests.
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

  // De-duplicate by node id (entity.name). RAPTOR-produced trees can
  // surface two clusters whose first-line summary collides, producing
  // duplicate ids; compact-box reacts to a duplicate by building a
  // corrupted child-map and either rendering one of the dupes with
  // an undefined position (→ "Cannot read 'draw'") or recursing
  // forever (→ stack overflow). First-seen wins.
  const seenIds = new Set<string>();
  const nodes: any[] = [];
  for (const entity of entities) {
    const id = (entity.name ?? '').trim();
    if (!id || seenIds.has(id)) continue;
    seenIds.add(id);
    nodes.push({
      id,
      entity_type: entity.type || 'title',
      description:
        entity.discription || entity.description || entity.aliases?.join(', '),
    });
  }
  const known = seenIds;

  // Drop dangling endpoints + self-loops up front. Self-loops would
  // also stack-overflow compact-box.
  const rawEdges = relations
    .filter((r) => r.from !== r.to && known.has(r.from) && known.has(r.to))
    .map((r) => ({
      source: r.from,
      target: r.to,
      description: r.type,
    }));

  // Cycle prune: walk forward edges in insertion order and drop any
  // edge whose target can already reach its source — that means
  // adding it would close a cycle. ``compact-box`` assumes a DAG with
  // single parents; cycles or DAG diamonds both break it.
  const reachable = new Map<string, Set<string>>(); // source → reachable nodes
  const childMap = new Map<string, string[]>(); // source → direct children
  const edges: typeof rawEdges = [];
  const hasParent = new Set<string>();
  for (const e of rawEdges) {
    // Each node can have at most one parent in a tree — drop a second
    // parent edge rather than corrupt the layout.
    if (hasParent.has(e.target)) continue;
    // Does target already reach source? If yes, adding would cycle.
    const sourceReachable = reachable.get(e.source) ?? new Set<string>();
    if (sourceReachable.has(e.target) || e.target === e.source) continue;
    // Accept the edge.
    edges.push(e);
    hasParent.add(e.target);
    const targetChildren = childMap.get(e.target);
    // Mark target + everything it transitively reaches as reachable
    // from source (and from every ancestor of source).
    const newlyReachable = new Set<string>([e.target]);
    if (targetChildren) {
      const queue = [...targetChildren];
      while (queue.length) {
        const n = queue.shift()!;
        if (newlyReachable.has(n)) continue;
        newlyReachable.add(n);
        const grand = childMap.get(n);
        if (grand) queue.push(...grand);
      }
    }
    if (!childMap.has(e.source)) childMap.set(e.source, []);
    childMap.get(e.source)!.push(e.target);
    // Propagate the reachable set up the ancestor chain. Capped O(N²)
    // worst case; node counts here are small (≤ a few hundred), fine.
    for (const [ancestor, set] of reachable) {
      if (set.has(e.source) || ancestor === e.source) {
        for (const n of newlyReachable) set.add(n);
      }
    }
    if (!reachable.has(e.source)) reachable.set(e.source, new Set());
    const sr = reachable.get(e.source)!;
    for (const n of newlyReachable) sr.add(n);
  }

  // Single-natural-root case: pass through unchanged. The natural root
  // is the real top-level entity — no synthetic noise.
  const hasIncoming = new Set<string>();
  for (const e of edges) hasIncoming.add(e.target);
  const naturalRoots = nodes.filter((n) => !hasIncoming.has(n.id));
  if (naturalRoots.length <= 1) {
    return { nodes, edges };
  }

  // Forest case: inject an invisible anchor so compact-box has a
  // single rooted tree to lay out. ``isSynthetic`` is the renderer's
  // signal to draw the node with no fill/stroke/label and size 1×1 —
  // effectively a non-event on the canvas.
  return {
    nodes: [
      {
        id: FOREST_ANCHOR_ID,
        entity_type: 'root',
        isRoot: true,
        isSynthetic: true,
      },
      ...nodes,
    ],
    edges: [
      ...naturalRoots.map((n) => ({
        source: FOREST_ANCHOR_ID,
        target: n.id,
      })),
      ...edges,
    ],
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
  // ``tree`` kind ships as RAPTOR's hierarchical output; treat it the
  // same as the synthetic ``raptor`` kind / classic page_index — they
  // all read as a vertical hierarchy under one synthetic root, so they
  // share the LR tree renderer.

  const renderer: Renderer = useMemo(() => {
    const k = normalizeKind(activeTemplate?.kind);
    if (k === 'page_index' || k === 'raptor' || k === 'tree') return 'tree';
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
            <TreeGraph data={treeGraphData} show />
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
