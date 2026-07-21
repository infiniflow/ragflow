import { TreeDataItem } from '@/components/ui/tree-view';
import { DatasetNavNode } from '@/interfaces/database/dataset-nav';
import { ReactNode } from 'react';

export type NavTreeActionsFactory = (
  node: DatasetNavNode,
  parentName: string | null,
) => ReactNode;

type BuildNavTreeDataOptions = {
  childrenMap: Record<string, DatasetNavNode[]>;
  getActions?: NavTreeActionsFactory;
  onParentClick: (node: DatasetNavNode) => void;
  onChildClick: (node: DatasetNavNode, parentName: string) => void;
  loadingPlaceholder: string;
};

export function buildNavTreeData(
  items: DatasetNavNode[] = [],
  {
    childrenMap,
    getActions,
    onParentClick,
    onChildClick,
    loadingPlaceholder,
  }: BuildNavTreeDataOptions,
): TreeDataItem[] {
  return items.map((node) => {
    const item: TreeDataItem = {
      id: node.name,
      name: node.name,
      actions: getActions?.(node, null),
      onClick: () => onParentClick(node),
    };

    if (node.has_children) {
      const children = childrenMap[node.name];
      if (children?.length) {
        item.children = children.map((child) => ({
          id: `${node.name}/${child.name}`,
          name: child.name,
          actions: getActions?.(child, node.name),
          onClick: () => onChildClick(child, node.name),
        }));
      } else if (!children) {
        // Children not fetched yet: a placeholder keeps the node rendered as
        // an expandable branch until the request resolves.
        item.children = [
          { id: `${node.name}/__loading__`, name: loadingPlaceholder },
        ];
      }
      // Fetched but empty: leave children unset so the node becomes a leaf.
    }

    return item;
  });
}
