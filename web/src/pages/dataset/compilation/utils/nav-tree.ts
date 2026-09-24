import {
  adaptPageIndexToTreeData,
  adaptTreeToTreeData,
  getEntityDisplayName,
} from '@/components/structure-graph/adapters';
import { TreeDataItem } from '@/components/ui/tree-view';
import { CompilationTemplateKind } from '@/constants/compilation';
import { DatasetNavNode } from '@/interfaces/database/dataset-nav';
import {
  IStructureGraphEntity,
  IStructureGraphTemplate,
} from '@/interfaces/database/document-structure';
import trim from 'lodash/trim';
import { ReactNode } from 'react';

export type NavTreeActionsFactory = (
  node: DatasetNavNode,
  parentName: string | null,
) => ReactNode;

export type NavEntityClickHandler = (
  docNode: DatasetNavNode,
  name: string,
  description: string,
) => void;

type BuildNavTreeDataOptions = {
  childrenMap: Record<string, DatasetNavNode[]>;
  childrenErrorParents?: Record<string, boolean>;
  structureMap: Record<string, IStructureGraphTemplate[]>;
  /**
   * Search mode: the items are a pruned forest — the matched document leaves
   * plus the cluster path above each of them, every row carrying parent_kwd —
   * so branches come from the payload instead of a lazy /children request.
   */
  searchMode?: boolean;
  getActions?: NavTreeActionsFactory;
  onNodeClick: (node: DatasetNavNode, parentName: string | null) => void;
  onNodeExpand: (node: DatasetNavNode) => void;
  onEntityClick?: NavEntityClickHandler;
  loadingPlaceholder: string;
  errorPlaceholder: string;
};

function getEntityDescription(entity: IStructureGraphEntity): string {
  return entity.description ?? entity.discription ?? '';
}

// Kinds without a tree-shaped relation adapter fall back to a flat list.
function buildFlatEntityItems(
  entities: IStructureGraphEntity[],
): TreeDataItem[] {
  return entities
    .map((entity) => ({
      id: entity.id ?? entity.name ?? '',
      name: getEntityDisplayName(entity),
      entityType: entity.type,
    }))
    .filter((item) => item.id);
}

function buildTemplateChildren(
  template: IStructureGraphTemplate,
): TreeDataItem[] {
  switch (template.kind) {
    case CompilationTemplateKind.PageIndex:
      return adaptPageIndexToTreeData(template);
    case CompilationTemplateKind.Tree:
    case 'raptor':
      return adaptTreeToTreeData(template);
    default:
      return buildFlatEntityItems(template.entities);
  }
}

// Adapter output uses raw entity ids; re-id under the document node's tree id
// so ids stay unique across the whole nav tree, and wire entity clicks.
function decorateStructureItems(
  items: TreeDataItem[],
  entityById: Map<string, IStructureGraphEntity>,
  idPrefix: string,
  onEntityClick?: (entity: IStructureGraphEntity) => void,
): TreeDataItem[] {
  return items.map((item) => {
    const entity = entityById.get(item.id);
    return {
      ...item,
      id: `${idPrefix}/${item.id}`,
      onClick:
        entity && onEntityClick ? () => onEntityClick(entity) : undefined,
      children: item.children?.length
        ? decorateStructureItems(
            item.children,
            entityById,
            idPrefix,
            onEntityClick,
          )
        : undefined,
    };
  });
}

// Entity items mount directly under the document node — the template itself
// is not rendered as a tree node, its id only prefixes entity ids so they
// stay unique when a document has multiple templates.
function buildStructureTreeData(
  templates: IStructureGraphTemplate[],
  idPrefix: string,
  onEntityClick?: (entity: IStructureGraphEntity) => void,
): TreeDataItem[] {
  return templates.flatMap((template) => {
    const entityById = new Map<string, IStructureGraphEntity>(
      template.entities
        .map((entity) => [entity.id ?? entity.name ?? '', entity] as const)
        .filter(([id]) => !!id),
    );
    return decorateStructureItems(
      buildTemplateChildren(template),
      entityById,
      `${idPrefix}/${template.template_id}`,
      onEntityClick,
    );
  });
}

// navNodeIdentity is the forest's node key. Identity, not display name: two
// documents may share a name, and a cluster's display name is exactly the key
// its children reference through parent_kwd.
function navNodeIdentity(node: DatasetNavNode): string {
  return node.type === 'doc' && node.doc_id
    ? `doc:${node.doc_id}`
    : `cluster:${node.name}`;
}

type NavSearchForest = {
  roots: DatasetNavNode[];
  children: Record<string, DatasetNavNode[]>;
};

// nestNavSearchHits groups a search response into the tree its rows describe: a
// row whose parent edge points at another returned row becomes that row's child,
// everything else is a root. The backend returns every hit together with the
// cluster path above it, so this yields ONE tree per root cluster holding only
// the matched branches — instead of one top-level tree per hit, where a matching
// cluster's own matching subtrees showed up as trees beside it.
export function nestNavSearchHits(
  items: DatasetNavNode[] = [],
): NavSearchForest {
  const present = new Set(items.map(navNodeIdentity));
  const children: Record<string, DatasetNavNode[]> = {};
  const roots: DatasetNavNode[] = [];
  items.forEach((node) => {
    const parentKey = node.parent_kwd ? `cluster:${node.parent_kwd}` : '';
    if (parentKey && present.has(parentKey)) {
      const bucket = children[parentKey];
      if (bucket) {
        bucket.push(node);
      } else {
        children[parentKey] = [node];
      }
    } else {
      roots.push(node);
    }
  });
  return { roots, children };
}

// mountDocumentStructure attaches a document leaf's structure-graph children to
// its tree item, or a loading placeholder while the graph is being fetched.
//
// The search path passes pendingPlaceholder false: its branches mount already
// expanded (expandAll), so a pending row under every hit would sit there for
// good — the mount-time expansion never fires onExpand, which is what triggers
// the fetch.
function mountDocumentStructure(
  item: TreeDataItem,
  node: DatasetNavNode,
  id: string,
  options: BuildNavTreeDataOptions,
  { pendingPlaceholder = true } = {},
): void {
  const templates = options.structureMap[node.doc_id as string];
  if (!templates) {
    // Not fetched yet: the placeholder keeps the node rendered as an expandable
    // branch until the request resolves.
    item.hasChildren = true;
    if (pendingPlaceholder) {
      item.children = [
        { id: `${id}/__loading__`, name: options.loadingPlaceholder },
      ];
    }
    return;
  }
  const children = buildStructureTreeData(templates, id, (entity) =>
    options.onEntityClick?.(
      node,
      getEntityDisplayName(entity),
      getEntityDescription(entity),
    ),
  );
  if (children.length) {
    item.hasChildren = true;
    item.children = children;
  }
  // No entity nodes: leave hasChildren unset so the node stays a leaf.
}

// buildNavSearchTreeData renders the pruned search forest. Branches come from
// the response, so expanding one fetches nothing; a matched document leaf still
// expands into its structure graph.
function buildNavSearchTreeData(
  nodes: DatasetNavNode[],
  forest: NavSearchForest,
  options: BuildNavTreeDataOptions,
  parentName: string | null,
  idPrefix: string,
): TreeDataItem[] {
  const { getActions, onNodeClick, onNodeExpand } = options;
  return nodes.map((node) => {
    const identity = navNodeIdentity(node);
    const id = idPrefix ? `${idPrefix}/${identity}` : identity;
    const item: TreeDataItem = {
      id,
      name: trim(node.name),
      actions: getActions?.(node, parentName),
      onClick: () => onNodeClick(node, parentName),
    };

    const branches = forest.children[identity];
    if (branches?.length) {
      item.hasChildren = true;
      item.children = buildNavSearchTreeData(
        branches,
        forest,
        options,
        node.name,
        id,
      );
    } else if (node.doc_id) {
      item.onExpand = () => onNodeExpand(node);
      mountDocumentStructure(item, node, id, options, {
        pendingPlaceholder: false,
      });
    }

    return item;
  });
}

export function buildNavTreeData(
  items: DatasetNavNode[] = [],
  options: BuildNavTreeDataOptions,
  parentName: string | null = null,
  idPrefix = '',
): TreeDataItem[] {
  if (options.searchMode) {
    const forest = nestNavSearchHits(items);
    return buildNavSearchTreeData(
      forest.roots,
      forest,
      options,
      parentName,
      idPrefix,
    );
  }

  const {
    childrenMap,
    childrenErrorParents,
    getActions,
    onNodeClick,
    onNodeExpand,
    loadingPlaceholder,
    errorPlaceholder,
  } = options;

  return items.map((node) => {
    const id = idPrefix ? `${idPrefix}/${node.name}` : node.name;
    const item: TreeDataItem = {
      id,
      name: trim(node.name),
      actions: getActions?.(node, parentName),
      onClick: () => onNodeClick(node, parentName),
      onExpand: () => onNodeExpand(node),
    };

    if (node.has_children) {
      item.hasChildren = true;
      const children = childrenMap[node.name];
      if (childrenErrorParents?.[node.name]) {
        item.children = [{ id: `${id}/__error__`, name: errorPlaceholder }];
      } else if (children?.length) {
        item.children = buildNavTreeData(children, options, node.name, id);
      } else if (!children) {
        // Children not fetched yet: a placeholder keeps the node rendered as
        // an expandable branch until the request resolves.
        item.children = [{ id: `${id}/__loading__`, name: loadingPlaceholder }];
      }
      // Fetched but empty: leave children unset so the branch opens to nothing.
    } else if (node.doc_id) {
      // Document leaf: expandable into its structure graph entities.
      mountDocumentStructure(item, node, id, options);
    }

    return item;
  });
}
