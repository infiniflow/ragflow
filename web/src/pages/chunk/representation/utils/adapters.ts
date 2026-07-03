import { type TreeDataItem } from '@/components/ui/tree-view';
import {
  type IArtifactGraph,
  type IArtifactGraphEntity,
} from '@/interfaces/database/dataset';
import {
  type IStructureGraphEntity,
  type IStructureGraphRelation,
  type IStructureGraphTemplate,
} from '@/interfaces/database/document-structure';
import { type TreeData } from '@antv/g6/lib/types';

declare module '@/components/ui/tree-view' {
  interface TreeDataItem {
    source_chunk_ids?: string[];
  }
}

function normalizeEntity(entity: IStructureGraphEntity) {
  const id = entity.id ?? entity.name ?? '';
  const name = entity.name ?? entity.id ?? '';
  return { ...entity, id, name };
}

function getEntityDescription(entity: IStructureGraphEntity): string {
  return entity.description ?? entity.discription ?? '';
}

function buildTreeDataItems(
  entities: IStructureGraphEntity[],
  relations: IStructureGraphRelation[],
  relationTypes: string[],
): TreeDataItem[] {
  const normalized = entities
    .map(normalizeEntity)
    .filter((entity) => entity.id);
  const map = new Map<string, TreeDataItem>(
    normalized.map((entity) => [
      entity.id,
      {
        id: entity.id,
        name: entity.name,
        source_chunk_ids: entity.source_chunk_ids,
      },
    ]),
  );
  const childIds = new Set<string>();

  for (const relation of relations) {
    if (!relationTypes.includes(relation.type ?? '')) continue;

    const parent = map.get(relation.from);
    const child = map.get(relation.to);
    if (!parent || !child) continue;

    childIds.add(child.id);
    parent.children = parent.children ?? [];
    parent.children.push(child);
  }

  return normalized
    .filter((entity) => !childIds.has(entity.id))
    .map((entity) => map.get(entity.id))
    .filter((item): item is TreeDataItem => item !== undefined);
}

export function adaptPageIndexToTreeData(
  template: IStructureGraphTemplate,
): TreeDataItem[] {
  return buildTreeDataItems(template.entities, template.relations, ['include']);
}

export function adaptTreeToTreeData(
  template: IStructureGraphTemplate,
): TreeDataItem[] {
  return buildTreeDataItems(template.entities, template.relations, ['child']);
}

export function adaptKnowledgeGraphToForceGraph(
  template: IStructureGraphTemplate,
): IArtifactGraph {
  const entities: IArtifactGraphEntity[] = template.entities.map((entity) => {
    const normalized = normalizeEntity(entity);
    return {
      slug: normalized.id,
      name: normalized.name,
      aliases: normalized.aliases ?? [],
      description: getEntityDescription(normalized),
      type: normalized.type ?? '',
      weight: normalized.mention_count ?? 0,
      source_chunk_ids: normalized.source_chunk_ids,
    };
  });

  const entityNames = new Set(entities.map((entity) => entity.slug));

  return {
    entities,
    relations: template.relations
      .filter(
        (relation) =>
          // Only keep relations whose source and target entities both exist in the graph.
          entityNames.has(relation.from) && entityNames.has(relation.to),
      )
      .map((relation) => ({
        from: relation.from,
        to: relation.to,
      })),
  };
}

function treeDataItemToG6TreeData(item: TreeDataItem): TreeData {
  const node: TreeData = {
    id: item.id,
    source_chunk_ids: item.source_chunk_ids,
  };

  if (item.children && item.children.length > 0) {
    node.children = item.children.map(treeDataItemToG6TreeData);
  }

  return node;
}

export function adaptMindMapToIndentedTree(
  template: IStructureGraphTemplate,
): TreeData {
  const roots = buildTreeDataItems(template.entities, template.relations, [
    'has_branch',
    'has_sub_branch',
  ]);

  const g6Roots = roots.map(treeDataItemToG6TreeData);

  if (g6Roots.length === 1) {
    return g6Roots[0]!;
  }

  return {
    id: 'mindmap-root',
    children: g6Roots,
  };
}
