import { type SelectWithSearchFlagOptionType } from '@/components/originui/select-with-search';
import { getEntityDisplayName } from '@/components/structure-graph/adapters';
import { type ClickableNode } from '@/components/structure-graph/representation-renderer';
import { CompilationTemplateKind } from '@/constants/compilation';
import { useFetchDocumentStructureGraph } from '@/hooks/use-document-request';
import { useDebounce } from 'ahooks';
import { useCallback, useMemo, useState } from 'react';
import { useSelectedTemplate } from './use-selected-template';

export function useGraphEntitySearch(
  onNodeClick?: (node: ClickableNode) => void,
) {
  const [graphKeywords, setGraphKeywords] = useState('');
  const [searchKeyword, setSearchKeyword] = useState(''); // ExpandableSearchInput value
  const [selectedNodeId, setSelectedNodeId] = useState(''); // entity name

  // Non-graph kinds search server-side with the same ?keywords= query the
  // graph-kind Enter search uses; the two inputs never show at once, and the
  // handlers below keep at most one of the two keyword sources non-empty.
  const debouncedSearchKeyword = useDebounce(searchKeyword, { wait: 500 });
  const { data, loading } = useFetchDocumentStructureGraph(
    debouncedSearchKeyword || graphKeywords,
  );
  const templates = useMemo(() => data?.templates ?? [], [data?.templates]);
  const {
    selectedTemplateId,
    setSelectedTemplateId,
    selectedTemplate,
    selectedKind,
  } = useSelectedTemplate(templates);
  const isGraphKind = selectedKind === CompilationTemplateKind.KnowledgeGraph;

  const entityOptions = useMemo<SelectWithSearchFlagOptionType[]>(
    () =>
      (selectedTemplate?.entities ?? []).map((entity) => {
        const name = getEntityDisplayName(entity);
        return {
          label: name,
          value: name,
          keywords: [name, ...(entity.aliases ?? [])],
        };
      }),
    [selectedTemplate?.entities],
  );

  // Only refill the select when the selected entity is still in the current
  // graph data, to avoid showing raw text with no matching option
  const selectedEntityName =
    selectedNodeId &&
    (selectedTemplate?.entities ?? []).some(
      (entity) => getEntityDisplayName(entity) === selectedNodeId,
    )
      ? selectedNodeId
      : '';

  const handleSelectEntity = useCallback(
    (name: string) => {
      if (!name) {
        setGraphKeywords('');
        setSearchKeyword('');
        setSelectedNodeId('');
        return;
      }
      setSelectedNodeId(name);
      const entity = (selectedTemplate?.entities ?? []).find(
        (item) => getEntityDisplayName(item) === name,
      );
      if (entity?.source_chunk_ids?.length) {
        onNodeClick?.({
          id: entity.id ?? name,
          name,
          source_chunk_ids: entity.source_chunk_ids,
        });
      }
    },
    [selectedTemplate?.entities, onNodeClick],
  );

  const handleNoMatchEnter = useCallback((keywords: string) => {
    setGraphKeywords(keywords);
    setSearchKeyword('');
    setSelectedNodeId('');
  }, []);

  const handleSearchKeywordChange = useCallback((value: string) => {
    setSearchKeyword(value);
  }, []);

  // Two-way binding: clicking a graph node selects it in the dropdown,
  // then forwards to chunk navigation like before
  const handleNodeClick = useCallback(
    (node: ClickableNode) => {
      if (isGraphKind && node.name) {
        setSelectedNodeId(node.name);
      }
      if (!node.source_chunk_ids?.length) return;
      onNodeClick?.(node);
    },
    [isGraphKind, onNodeClick],
  );

  const handleTemplateChange = useCallback(
    (templateId: string) => {
      setSelectedTemplateId(templateId);
      setGraphKeywords('');
      setSearchKeyword('');
      setSelectedNodeId('');
    },
    [setSelectedTemplateId],
  );

  return {
    data,
    loading,
    templates,
    selectedTemplateId,
    selectedTemplate,
    isGraphKind,
    entityOptions,
    searchKeyword,
    graphSelectValue: selectedEntityName || graphKeywords,
    highlightNodeId: selectedEntityName || null,
    handleSelectEntity,
    handleNoMatchEnter,
    handleSearchKeywordChange,
    handleTemplateChange,
    handleNodeClick,
  };
}
