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
    // Self-referencing relation (same entity as its own child) creates
    // an infinite recursion in the tree renderer.  This is a backend
    // data integrity issue (duplicate entity names) but we defend
    // against it in the frontend so the UI never hangs.
    if (relation.from === relation.to) continue;

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

function buildUniqueTreeDataItems(
  entities: IStructureGraphEntity[],
  relations: IStructureGraphRelation[],
  relationTypes: string[],
): TreeDataItem[] {
  const normalized = [
    ...new Map(
      entities
        .map(normalizeEntity)
        .filter((entity) => entity.id)
        .map((entity) => [entity.id, entity]),
    ).values(),
  ];
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
  const parentMap = new Map<string, string>();

  for (const relation of relations) {
    if (!relationTypes.includes(relation.type ?? '')) continue;

    const parent = map.get(relation.from);
    const child = map.get(relation.to);
    if (!parent || !child) continue;
    if (childIds.has(child.id)) continue;

    // Avoid cycles: do not attach a node under one of its descendants.
    let cursor = relation.from;
    let wouldCycle = false;
    while (parentMap.has(cursor)) {
      const ancestor = parentMap.get(cursor)!;
      if (ancestor === child.id) {
        wouldCycle = true;
        break;
      }
      cursor = ancestor;
    }
    if (wouldCycle) continue;

    childIds.add(child.id);
    parentMap.set(child.id, relation.from);
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

function filterTreeDataItems(
  items: TreeDataItem[],
  keyword: string,
): TreeDataItem[] {
  const lowerKeyword = keyword.toLowerCase();

  return items.reduce<TreeDataItem[]>((acc, item) => {
    const children = item.children
      ? filterTreeDataItems(item.children, keyword)
      : [];
    const matches = item.name.toLowerCase().includes(lowerKeyword);

    if (matches || children.length > 0) {
      acc.push({
        ...item,
        children: children.length > 0 ? children : item.children,
      });
    }

    return acc;
  }, []);
}

export function filterTreeDataByKeyword(
  data: TreeDataItem[],
  keyword: string,
): TreeDataItem[] {
  if (!keyword.trim()) return data;
  return filterTreeDataItems(data, keyword);
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
    data: {
      name: item.name,
      source_chunk_ids: item.source_chunk_ids,
    },
  };

  if (item.children && item.children.length > 0) {
    node.children = item.children.map(treeDataItemToG6TreeData);
  }

  return node;
}

export interface TimelineX6NodeData {
  id: string;
  shape: 'rect' | 'circle';
  width: number;
  height: number;
  label: string;
  data?: IStructureGraphEntity;
  attrs?: Record<string, unknown>;
}

export interface TimelineX6EdgeData {
  id: string;
  shape: 'edge';
  source: string;
  target: string;
  attrs?: Record<string, unknown>;
}

export function adaptTimelineToX6Data(template: IStructureGraphTemplate): {
  nodes: TimelineX6NodeData[];
  edges: TimelineX6EdgeData[];
} {
  const normalized = template.entities
    .map(normalizeEntity)
    .filter((entity) => entity.id);
  const entityIds = new Set(normalized.map((entity) => entity.id));

  const nodes = normalized.map((entity) => {
    const isTimestamp = entity.type === 'timestamp';
    return {
      id: entity.id,
      shape: isTimestamp ? ('circle' as const) : ('rect' as const),
      width: isTimestamp ? 40 : 200,
      height: isTimestamp ? 40 : 80,
      label: entity.name,
      data: entity,
      attrs: {
        body: {
          fill: isTimestamp ? 'rgb(var(--accent-primary))' : 'transparent',
          stroke: 'rgb(var(--accent-primary))',
          rx: isTimestamp ? 48 : 8,
          ry: isTimestamp ? 48 : 8,
        },
        label: {
          fill: isTimestamp
            ? 'rgb(var(--accent-primary))'
            : 'rgb(var(--text-primary))',
          fontSize: 12,
          ...(isTimestamp && {
            refX: '50%',
            refY: 0,
            refY2: -8,
            textAnchor: 'middle',
            textVerticalAnchor: 'bottom',
          }),
          textWrap: {
            width: isTimestamp ? 80 : 180,
            height: isTimestamp ? 80 : 64,
            ellipsis: true,
          },
        },
      },
    };
  });

  const edges = template.relations
    .filter(
      (relation) => entityIds.has(relation.from) && entityIds.has(relation.to),
    )
    .map((relation, index) => ({
      id: `timeline-edge-${index}`,
      shape: 'edge' as const,
      source: relation.from,
      target: relation.to,
      attrs: {
        line: {
          stroke: '#8c8c8c',
          strokeWidth: 1,
          targetMarker: 'classic',
        },
      },
    }));

  return { nodes, edges };
}

export function adaptMindMapToIndentedTree(
  template: IStructureGraphTemplate,
): TreeData {
  const roots = buildUniqueTreeDataItems(
    template.entities,
    template.relations,
    ['has_branch', 'has_sub_branch'],
  );

  const g6Roots = roots.map(treeDataItemToG6TreeData);

  if (g6Roots.length === 1) {
    return g6Roots[0]!;
  }

  return {
    id: 'mindmap-root',
    children: g6Roots,
  };
}
