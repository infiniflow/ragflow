import {
  useDeleteDatasetNav,
  useDeleteDatasetNavNode,
  useFetchDatasetNav,
  useFetchDatasetNavChildren,
} from '@/hooks/use-dataset-nav-request';
import { DatasetNavNode } from '@/interfaces/database/dataset-nav';
import { useCallback, useEffect, useState } from 'react';

export interface SelectedNavNode {
  parentName: string | null;
  name: string;
  description: string;
  doc_count: number;
}

export function useCompilationNav() {
  const { data: navList, loading: navLoading } = useFetchDatasetNav();
  const { deleteNav, loading: deleteNavLoading } = useDeleteDatasetNav();
  const { deleteNavNode, loading: deleteNodeLoading } =
    useDeleteDatasetNavNode();

  const [loadingParent, setLoadingParent] = useState<string | null>(null);
  const [childrenMap, setChildrenMap] = useState<
    Record<string, DatasetNavNode[]>
  >({});
  const [selectedNode, setSelectedNode] = useState<SelectedNavNode | null>(
    null,
  );

  const { data: childrenData } = useFetchDatasetNavChildren(loadingParent);

  useEffect(() => {
    if (loadingParent && childrenData) {
      setChildrenMap((prev) => ({
        ...prev,
        [loadingParent]: childrenData.items,
      }));
    }
  }, [loadingParent, childrenData]);

  const loadChildren = useCallback(
    (name: string) => {
      if (!(name in childrenMap)) {
        setLoadingParent(name);
      }
    },
    [childrenMap],
  );

  const removeChild = useCallback((parentName: string, childName: string) => {
    setChildrenMap((prev) => {
      const children = prev[parentName];
      if (!children) {
        return prev;
      }
      return {
        ...prev,
        [parentName]: children.filter((node) => node.name !== childName),
      };
    });
  }, []);

  const dropChildren = useCallback((name: string) => {
    setChildrenMap((prev) => {
      if (!(name in prev)) {
        return prev;
      }
      const next = { ...prev };
      delete next[name];
      return next;
    });
  }, []);

  const resetNav = useCallback(() => {
    setSelectedNode(null);
    setChildrenMap({});
    setLoadingParent(null);
  }, []);

  // Row click selects the node and shows its description on the right,
  // same as a leaf; expansion is handled separately via handleParentExpand.
  const handleParentClick = useCallback((node: DatasetNavNode) => {
    setSelectedNode({
      parentName: null,
      name: node.name,
      description: node.description,
      doc_count: node.doc_count,
    });
  }, []);

  const handleParentExpand = useCallback(
    (node: DatasetNavNode) => {
      if (node.has_children) {
        loadChildren(node.name);
      }
    },
    [loadChildren],
  );

  const handleChildClick = useCallback(
    (node: DatasetNavNode, parentName: string) => {
      setSelectedNode({
        parentName,
        name: node.name,
        description: node.description,
        doc_count: node.doc_count,
      });
    },
    [],
  );

  const handleDeleteAll = useCallback(async () => {
    const data = await deleteNav();
    if (data?.code === 0) {
      resetNav();
    }
  }, [deleteNav, resetNav]);

  const handleDeleteNode = useCallback(
    async (name: string, parentName: string | null) => {
      const data = await deleteNavNode(name);
      if (data?.code === 0) {
        if (parentName) {
          // The children query of this parent may have no active observer, so
          // invalidation alone would not refetch — filter the local map too.
          removeChild(parentName, name);
          setSelectedNode((current) =>
            current?.parentName === parentName && current?.name === name
              ? null
              : current,
          );
        } else {
          dropChildren(name);
          // Clear the selection when the deleted root is the selected node
          // itself or the parent of the selected child.
          setSelectedNode((current) =>
            current?.parentName === name ||
            (current?.parentName === null && current?.name === name)
              ? null
              : current,
          );
        }
      }
    },
    [deleteNavNode, removeChild, dropChildren],
  );

  return {
    navList,
    navLoading,
    childrenMap,
    selectedNode,
    deleteNavLoading,
    deleteNodeLoading,
    handleParentClick,
    handleParentExpand,
    handleChildClick,
    handleDeleteAll,
    handleDeleteNode,
  };
}
