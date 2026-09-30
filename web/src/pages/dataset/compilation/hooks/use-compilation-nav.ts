import { GenerateType } from '@/constants/knowledge';
import {
  useGenerateStatus,
  useTraceRunData,
} from '@/hooks/use-dataset-generate';
import {
  DatasetNavKeys,
  useFetchDatasetNav,
  useFetchDatasetNavChildren,
} from '@/hooks/use-dataset-nav-request';
import { useFetchDocumentStructureGraphById } from '@/hooks/use-document-request';
import { useKnowledgeBaseId } from '@/hooks/use-knowledge-request';
import { DatasetNavNode } from '@/interfaces/database/dataset-nav';
import { IStructureGraphTemplate } from '@/interfaces/database/document-structure';
import { useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { trim } from 'lodash';
import { useCallback, useEffect, useState } from 'react';

import { useRunEndEffect } from './use-run-end-effect';

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
  // The filter actually applied to requests; the input value lags behind it by
  // the debounce window.
  const activeKeywords = trim(debouncedKeywords);
  const {
    data: navList,
    loading: navLoading,
    isError: navError,
  } = useFetchDatasetNav(debouncedKeywords);

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
    useFetchDatasetNavChildren(loadingParent, activeKeywords);
  // Opened documents get their FULL structure graph, without the nav keywords: a
  // keyword-filtered read would hide every entity that does not match (keyword
  // drill-down lives in the structure view's own search box).
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

  const clearExpandedData = useCallback(() => {
    setChildrenMap({});
    setChildrenErrorParents({});
    setLoadingParent(null);
    setStructureMap({});
    setLoadingDocId(null);
  }, []);

  useEffect(() => {
    // Loaded children/graphs were fetched under the previous keywords filter;
    // drop them so re-expansion refetches under the active filter.
    clearExpandedData();
  }, [activeKeywords, clearExpandedData]);

  const queryClient = useQueryClient();
  // The nav tree is a by-product of tree/structure knowledge compilation.
  // Poll the Tree-scoped scheduler status (kind "raptor" normalizes to "Tree")
  // so the view can surface compile progress/logs and refresh the tree when a
  // run ends.
  const { data: navRunData } = useTraceRunData(GenerateType.Raptor);
  const { status: navStatus } = useGenerateStatus(navRunData);

  const handleCompileRunEnd = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: DatasetNavKeys.all(kbId) });
    // Children/structure data cached in local state predates the compile, and
    // invalidation cannot refetch their inactive queries — drop the maps so
    // re-expansion fetches fresh data.
    clearExpandedData();
  }, [queryClient, kbId, clearExpandedData]);
  useRunEndEffect(navStatus, handleCompileRunEnd);

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

  return {
    navList,
    navLoading,
    navError,
    keywords,
    activeKeywords,
    childrenMap,
    childrenErrorParents,
    structureMap,
    selectedNode,
    navRunData,
    navStatus,
    handleKeywordsChange,
    handleNodeClick,
    handleNodeExpand,
    handleEntityClick,
  };
}
