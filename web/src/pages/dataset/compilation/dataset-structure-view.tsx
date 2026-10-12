import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import {
  findEntityDisplayNameByKeyword,
  getEntityDisplayName,
} from '@/components/structure-graph/adapters';
import { RepresentationRenderer } from '@/components/structure-graph/representation-renderer';
import { Card } from '@/components/ui/card';
import {
  useGenerateStatus,
  useTraceRunData,
} from '@/hooks/use-dataset-generate';
import {
  DatasetStructureKeys,
  useFetchDatasetStructureGraph,
  useKnowledgeBaseId,
} from '@/hooks/use-knowledge-request';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { StructureKind, ViewMode, ViewModeGenerateTypeMap } from './constants';
import { useRunEndEffect } from './hooks/use-run-end-effect';
import CompilationEmptyState from './empty-state';
import { CompilationLoadingCard } from './loading-card';

interface DatasetStructureViewProps {
  kind: StructureKind;
}

export function DatasetStructureView({ kind }: DatasetStructureViewProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const knowledgeBaseId = useKnowledgeBaseId();
  const [graphKeywords, setGraphKeywords] = useState('');
  const [selectedNodeId, setSelectedNodeId] = useState('');
  const { data, loading } = useFetchDatasetStructureGraph(kind, graphKeywords);
  const template = data?.templates?.[0];

  const generateType = ViewModeGenerateTypeMap[kind];
  const { data: structureRunData } = useTraceRunData(generateType);
  const { status: structureStatus } = useGenerateStatus(structureRunData);

  const handleRunEnd = useCallback(() => {
    queryClient.invalidateQueries({
      queryKey: DatasetStructureKeys.kind(knowledgeBaseId, kind),
    });
  }, [knowledgeBaseId, kind]);

  useRunEndEffect(structureStatus, handleRunEnd);

  const entityOptions = useMemo<SelectWithSearchFlagOptionType[]>(
    () =>
      (template?.entities ?? []).map((entity) => {
        const name = getEntityDisplayName(entity);
        return {
          label: name,
          value: name,
          keywords: [name, ...(entity.aliases ?? [])],
        };
      }),
    [template?.entities],
  );

  // Only refill the select when the selected entity is still in the current
  // graph data, to avoid showing raw text with no matching option
  const selectedEntityName =
    selectedNodeId &&
    (template?.entities ?? []).some(
      (entity) => getEntityDisplayName(entity) === selectedNodeId,
    )
      ? selectedNodeId
      : '';

  const handleSelectEntity = useCallback((name: string) => {
    // Picking an option behaves like an Enter search: refetch the server-side
    // keyword subgraph for that entity and keep the node highlighted.
    setSelectedNodeId(name);
    setGraphKeywords(name);
  }, []);

  const handleNoMatchEnter = useCallback(
    (keywords: string) => {
      // Enter on a keyword that exactly names an entity must behave like
      // picking it from the dropdown. Only unmatched text falls back to the
      // raw keyword subgraph with no highlighted node.
      const entityName = findEntityDisplayNameByKeyword(
        template?.entities ?? [],
        keywords,
      );
      if (entityName) {
        handleSelectEntity(entityName);
        return;
      }
      setGraphKeywords(keywords);
      setSelectedNodeId('');
    },
    [template?.entities, handleSelectEntity],
  );

  if (loading && !data) {
    return <CompilationLoadingCard />;
  }

  if (!template && !graphKeywords) {
    return <CompilationEmptyState type={kind} data={structureRunData} />;
  }

  return (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
      <div className="flex justify-between gap-4 px-4 pt-4">
        {kind === ViewMode.Graph && (
          <SelectWithSearch
            options={entityOptions}
            value={selectedEntityName || graphKeywords}
            onChange={handleSelectEntity}
            placeholder={t('knowledgeCompilation.searchEntity')}
            allowClear
            alwaysShowSearch
            triggerClassName="ml-auto w-96 max-w-full"
            onNoMatchEnter={handleNoMatchEnter}
            disableAutoSelectOnEnter
          />
        )}
      </div>
      <RepresentationRenderer
        template={template}
        highlightNodeId={selectedEntityName || null}
        totalEntities={data?.total_entities}
        returnedEntities={data?.returned_entities}
      />
    </Card>
  );
}
