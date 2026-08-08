import { TreeDataItem } from '@/components/ui/tree-view';
import { DatasetNavNode } from '@/interfaces/database/dataset-nav';
import trim from 'lodash/trim';
import { ReactNode } from 'react';

export type NavTreeActionsFactory = (
  node: DatasetNavNode,
  parentName: string | null,
) => ReactNode;

type BuildNavTreeDataOptions = {
  childrenMap: Record<string, DatasetNavNode[]>;
  childrenErrorParents?: Record<string, boolean>;
  loadingParent?: string | null;
  getActions?: NavTreeActionsFactory;
  onParentClick: (node: DatasetNavNode) => void;
  onChildClick: (node: DatasetNavNode, parentName: string) => void;
  loadingPlaceholder: string;
  errorPlaceholder: string;
};

export function buildNavTreeData(
  items: DatasetNavNode[] = [],
  {
    childrenMap,
    childrenErrorParents,
    loadingParent,
    getActions,
    onParentClick,
    onChildClick,
    loadingPlaceholder,
    errorPlaceholder,
  }: BuildNavTreeDataOptions,
): TreeDataItem[] {
  return items.map((node) => {
    const item: TreeDataItem = {
      id: node.name,
      name: trim(node.name),
      hasChildren: node.has_children,
      actions: getActions?.(node, null),
      onClick: () => onParentClick(node),
    };

    if (node.has_children) {
      const children = childrenMap[node.name];
      if (childrenErrorParents?.[node.name]) {
        item.children = [
          { id: `${node.name}/__error__`, name: errorPlaceholder },
        ];
      } else if (children?.length) {
        item.children = children.map((child) => ({
          id: `${node.name}/${child.name}`,
          name: trim(child.name),
          hasChildren: child.has_children,
          actions: getActions?.(child, node.name),
          onClick: () => onChildClick(child, node.name),
        }));
      } else if (loadingParent === node.name) {
        item.children = [
          { id: `${node.name}/__loading__`, name: loadingPlaceholder },
        ];
      }
      // Fetched but empty: leave children unset so the node becomes a leaf.
    }

    return item;
  });
}
