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
  structureMap: Record<string, IStructureGraphTemplate[]>;
  getActions?: NavTreeActionsFactory;
  onNodeClick: (node: DatasetNavNode, parentName: string | null) => void;
  onNodeExpand: (node: DatasetNavNode) => void;
  onEntityClick?: NavEntityClickHandler;
  loadingPlaceholder: string;
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

function buildStructureTreeData(
  templates: IStructureGraphTemplate[],
  idPrefix: string,
  onEntityClick?: (entity: IStructureGraphEntity) => void,
): TreeDataItem[] {
  return templates.map((template) => {
    const entityById = new Map<string, IStructureGraphEntity>(
      template.entities
        .map((entity) => [entity.id ?? entity.name ?? '', entity] as const)
        .filter(([id]) => !!id),
    );
    const templateId = `${idPrefix}/${template.template_id}`;
    const children = decorateStructureItems(
      buildTemplateChildren(template),
      entityById,
      templateId,
      onEntityClick,
    );
    return {
      id: templateId,
      name: trim(template.template_name),
      hasChildren: children.length > 0,
      children,
    };
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
    structureMap,
    getActions,
    onNodeClick,
    onNodeExpand,
    onEntityClick,
    loadingPlaceholder,
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
      if (children?.length) {
        item.children = buildNavTreeData(children, options, node.name, id);
      } else if (!children) {
        // Children not fetched yet: a placeholder keeps the node rendered as
        // an expandable branch until the request resolves.
        item.children = [{ id: `${id}/__loading__`, name: loadingPlaceholder }];
      }
      // Fetched but empty: leave children unset so the branch opens to nothing.
    } else if (node.doc_id) {
      // Document leaf: expandable into its structure graph templates.
      const templates = structureMap[node.doc_id];
      if (templates?.length) {
        item.hasChildren = true;
        item.children = buildStructureTreeData(templates, id, (entity) =>
          onEntityClick?.(
            node,
            getEntityDisplayName(entity),
            getEntityDescription(entity),
          ),
        );
      } else if (!templates) {
        item.hasChildren = true;
        item.children = [{ id: `${id}/__loading__`, name: loadingPlaceholder }];
      }
      // Fetched but empty: leave hasChildren unset so the node stays a leaf.
    }

    return item;
  });
}
