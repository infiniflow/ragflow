import {
  useDeleteDatasetNav,
  useDeleteDatasetNavNode,
  useFetchDatasetNav,
  useFetchDatasetNavChildren,
} from '@/hooks/use-dataset-nav-request';
import { useFetchDocumentStructureGraphById } from '@/hooks/use-document-request';
import { useKnowledgeBaseId } from '@/hooks/use-knowledge-request';
import { DatasetNavNode } from '@/interfaces/database/dataset-nav';
import { IStructureGraphTemplate } from '@/interfaces/database/document-structure';
import { useDebounce } from 'ahooks';
import { useCallback, useEffect, useState } from 'react';

export interface SelectedNavNode {
  parentName: string | null;
  name: string;
  description: string;
  docId?: string;
  doc_count?: number;
  keywords?: string[];
  entities?: string[];
  graph_content?: string;
}

export function useCompilationNav() {
  const kbId = useKnowledgeBaseId();
  const [keywords, setKeywords] = useState('');
  const debouncedKeywords = useDebounce(keywords, { wait: 500 });
  const {
    data: navList,
    loading: navLoading,
    isError: navError,
  } = useFetchDatasetNav(debouncedKeywords);
  const { deleteNav, loading: deleteNavLoading } = useDeleteDatasetNav();
  const { deleteNavNode, loading: deleteNodeLoading } =
    useDeleteDatasetNavNode();

  const [loadingParent, setLoadingParent] = useState<string | null>(null);
  const [childrenMap, setChildrenMap] = useState<
    Record<string, DatasetNavNode[]>
  >({});
  const [childrenErrorParents, setChildrenErrorParents] = useState<
    Record<string, boolean>
  >({});
  const [loadingDocId, setLoadingDocId] = useState<string | null>(null);
  const [structureMap, setStructureMap] = useState<
    Record<string, IStructureGraphTemplate[]>
  >({});
  const [selectedNode, setSelectedNode] = useState<SelectedNavNode | null>(
    null,
  );

  const { data: childrenData, isError: childrenError } =
    useFetchDatasetNavChildren(loadingParent);
  const { data: structureData, isPlaceholderData: structurePlaceholder } =
    useFetchDocumentStructureGraphById(kbId, loadingDocId ?? '');

  useEffect(() => {
    if (!loadingParent || !childrenData) {
      return;
    }
    const parent = loadingParent;
    setChildrenMap((prev) => ({
      ...prev,
      [parent]: childrenData.items,
    }));
    setChildrenErrorParents((prev) => {
      if (!prev[parent]) {
        return prev;
      }
      const next = { ...prev };
      delete next[parent];
      return next;
    });
    setLoadingParent(null);
  }, [loadingParent, childrenData]);

  useEffect(() => {
    if (!loadingParent || !childrenError) {
      return;
    }
    const parent = loadingParent;
    setChildrenMap((prev) => {
      if (!(parent in prev)) {
        return prev;
      }
      const next = { ...prev };
      delete next[parent];
      return next;
    });
    setChildrenErrorParents((prev) => ({
      ...prev,
      [parent]: true,
    }));
    setLoadingParent(null);
  }, [loadingParent, childrenError]);

  useEffect(() => {
    // keepPreviousData serves the previous document's graph while the new one
    // loads; only store the response once it belongs to loadingDocId.
    if (loadingDocId && structureData && !structurePlaceholder) {
      setStructureMap((prev) => ({
        ...prev,
        [loadingDocId]: structureData.templates,
      }));
    }
  }, [loadingDocId, structureData, structurePlaceholder]);

  const loadChildren = useCallback(
    (name: string) => {
      setChildrenErrorParents((prev) => {
        if (!prev[name]) {
          return prev;
        }
        const next = { ...prev };
        delete next[name];
        return next;
      });
      if (!(name in childrenMap) && loadingParent !== name) {
        setLoadingParent(name);
      }
    },
    [childrenMap, loadingParent],
  );

  const loadStructure = useCallback(
    (docId: string) => {
      if (!(docId in structureMap)) {
        setLoadingDocId(docId);
      }
    },
    [structureMap],
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
    setChildrenErrorParents((prev) => {
      if (!(name in prev)) {
        return prev;
      }
      const next = { ...prev };
      delete next[name];
      return next;
    });
  }, []);

  const dropStructure = useCallback((docId: string) => {
    setStructureMap((prev) => {
      if (!(docId in prev)) {
        return prev;
      }
      const next = { ...prev };
      delete next[docId];
      return next;
    });
  }, []);

  const resetNav = useCallback(() => {
    setSelectedNode(null);
    setChildrenMap({});
    setChildrenErrorParents({});
    setLoadingParent(null);
    setStructureMap({});
    setLoadingDocId(null);
  }, []);

  const handleKeywordsChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setKeywords(e.target.value);
    },
    [],
  );

  const handleNodeClick = useCallback(
    (node: DatasetNavNode, parentName: string | null) => {
      setSelectedNode({
        parentName,
        name: node.name,
        description: node.description,
        doc_count: node.doc_count,
        keywords: node.keywords,
        entities: node.entities,
        graph_content: node.graph_content,
      });
    },
    [],
  );

  const handleNodeExpand = useCallback(
    (node: DatasetNavNode) => {
      if (node.has_children) {
        loadChildren(node.name);
      } else if (node.doc_id) {
        loadStructure(node.doc_id);
      }
    },
    [loadChildren, loadStructure],
  );

  const handleEntityClick = useCallback(
    (docNode: DatasetNavNode, name: string, description: string) => {
      setSelectedNode({
        parentName: docNode.name,
        name,
        description,
        docId: docNode.doc_id,
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
      const removed = parentName
        ? childrenMap[parentName]?.find((node) => node.name === name)
        : undefined;
      const data = await deleteNavNode(name);
      if (data?.code === 0) {
        if (parentName) {
          // The children query of this parent may have no active observer, so
          // invalidation alone would not refetch — filter the local map too.
          removeChild(parentName, name);
          // A deleted sub-cluster may have loaded children; a deleted document
          // may have a loaded structure graph. Drop both from the local maps.
          dropChildren(name);
          if (removed?.doc_id) {
            dropStructure(removed.doc_id);
          }
          setSelectedNode((current) =>
            (current?.parentName === parentName && current?.name === name) ||
            (removed?.doc_id && current?.docId === removed.doc_id)
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
    [deleteNavNode, childrenMap, removeChild, dropChildren, dropStructure],
  );

  return {
    navList,
    navLoading,
    navError,
    keywords,
    childrenMap,
    childrenErrorParents,
    structureMap,
    selectedNode,
    deleteNavLoading,
    deleteNodeLoading,
    handleKeywordsChange,
    handleNodeClick,
    handleNodeExpand,
    handleEntityClick,
    handleDeleteAll,
    handleDeleteNode,
  };
}
