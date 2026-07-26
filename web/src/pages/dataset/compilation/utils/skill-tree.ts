import type { TreeDataItem } from '@/components/ui/tree-view';
import type { DatasetSkillTreeNode } from '@/interfaces/database/dataset-skill';
import type { ReactNode } from 'react';

export type SkillTreeActionsFactory = (skillKwd: string) => ReactNode;

export function buildSkillTreeData(
  nodes: DatasetSkillTreeNode[] = [],
  getActions?: SkillTreeActionsFactory,
): TreeDataItem[] {
  return nodes.map((node) => {
    const item: TreeDataItem = {
      id: node.skill_kwd,
      name: node.skill_kwd,
      actions: getActions?.(node.skill_kwd),
    };
    // Only set children when non-empty: TreeView branches on
    // `item.children ? TreeNode : TreeLeaf` and [] is truthy, so an empty
    // array would render a leaf as a branch with a chevron and no children.
    if (node.children_kwd?.length) {
      item.children = buildSkillTreeData(node.children_kwd, getActions);
    }
    return item;
  });
}

// Same prune logic as filterTreeDataByKeyword in
// pages/chunk/representation/utils/adapters.ts — that module carries
// chunk-specific TreeDataItem augmentation, so a feature-local copy is kept
// here instead of a cross-feature import.
export function filterSkillTreeData(
  items: TreeDataItem[],
  keyword: string,
): TreeDataItem[] {
  const normalizedKeyword = keyword.trim().toLowerCase();
  if (!normalizedKeyword) {
    return items;
  }

  return items.reduce<TreeDataItem[]>((acc, item) => {
    const children = item.children
      ? filterSkillTreeData(item.children, keyword)
      : [];
    const matches = item.name.toLowerCase().includes(normalizedKeyword);

    if (matches || children.length > 0) {
      acc.push({
        ...item,
        // A matched node keeps its full original subtree; an unmatched
        // ancestor keeps only the pruned children leading to matches.
        children: children.length > 0 ? children : item.children,
      });
    }
    return acc;
  }, []);
}

export function countSkillTreeNodes(
  nodes: DatasetSkillTreeNode[] = [],
): number {
  return nodes.reduce(
    (total, node) => total + 1 + countSkillTreeNodes(node.children_kwd),
    0,
  );
}
