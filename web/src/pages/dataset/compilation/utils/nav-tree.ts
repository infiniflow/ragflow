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

export function buildNavTreeData(
  items: DatasetNavNode[] = [],
  options: BuildNavTreeDataOptions,
  parentName: string | null = null,
  idPrefix = '',
): TreeDataItem[] {
  const {
    childrenMap,
    childrenErrorParents,
    structureMap,
    getActions,
    onNodeClick,
    onNodeExpand,
    onEntityClick,
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
      const templates = structureMap[node.doc_id];
      if (!templates) {
        // Not fetched yet: a placeholder keeps the node rendered as an
        // expandable branch until the request resolves.
        item.hasChildren = true;
        item.children = [{ id: `${id}/__loading__`, name: loadingPlaceholder }];
      } else {
        const children = buildStructureTreeData(templates, id, (entity) =>
          onEntityClick?.(
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
    }

    return item;
  });
}
