import type { ReactNode } from 'react';

export interface TreeSelectNode {
  id: string;
  title: string;
  label?: ReactNode;
  children?: TreeSelectNode[];
  disabled?: boolean;
  /**
   * Extra strings the search box matches against, for text that only lives in
   * `label` (e.g. an alias rendered next to the title).
   */
  searchKeywords?: string[];
  data?: Record<string, any>;
}

// treeNodeMatchesTerm is the row-level search predicate.
export function treeNodeMatchesTerm(
  node: TreeSelectNode,
  loweredTerm: string,
): boolean {
  return (
    node.title.toLowerCase().includes(loweredTerm) ||
    !!node.searchKeywords?.some((keyword) =>
      keyword.toLowerCase().includes(loweredTerm),
    )
  );
}

// filterTreeNodes keeps matching rows. A matching parent keeps its entire
// subtree so its children stay visible; otherwise only branches containing
// matching descendants are kept.
export function filterTreeNodes(
  nodes: TreeSelectNode[],
  term: string,
): TreeSelectNode[] {
  if (!term) return nodes;
  const lowered = term.toLowerCase();
  return nodes.reduce<TreeSelectNode[]>((acc, node) => {
    if (treeNodeMatchesTerm(node, lowered)) {
      acc.push(node);
      return acc;
    }
    const filteredChildren = node.children
      ? filterTreeNodes(node.children, term)
      : undefined;
    if (filteredChildren?.length) {
      acc.push({ ...node, children: filteredChildren });
    }
    return acc;
  }, []);
}

// collectBranchIds returns the ids of every branch node in the tree.
export function collectBranchIds(nodes: TreeSelectNode[]): Set<string> {
  const ids = new Set<string>();
  const walk = (list: TreeSelectNode[]) => {
    for (const node of list) {
      if (node.children?.length) {
        ids.add(node.id);
        walk(node.children);
      }
    }
  };
  walk(nodes);
  return ids;
}

// withBranchIds returns a new set with every branch id from `nodes` added, or
// the previous set (same reference) when nothing changed, so React bails out
// of the state update instead of looping on a fresh-but-identical Set.
export function withBranchIds(
  prev: Set<string>,
  nodes: TreeSelectNode[],
): Set<string> {
  const branchIds = collectBranchIds(nodes);
  let added = false;
  const next = new Set(prev);
  branchIds.forEach((id) => {
    if (!next.has(id)) {
      next.add(id);
      added = true;
    }
  });
  return added ? next : prev;
}
