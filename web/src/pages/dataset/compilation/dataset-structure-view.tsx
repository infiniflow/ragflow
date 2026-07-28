import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { getEntityDisplayName } from '@/components/structure-graph/adapters';
import { RepresentationRenderer } from '@/components/structure-graph/representation-renderer';
import { Card } from '@/components/ui/card';
import {
  DatasetStructureKeys,
  useFetchDatasetStructureGraph,
  useFetchKnowledgeBaseConfiguration,
  useKnowledgeBaseId,
} from '@/hooks/use-knowledge-request';
import { GenerateStatus } from '@/pages/dataset/dataset/generate-button/constants';
import { useTraceRunData } from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { StructureKind, ViewMode, ViewModeGenerateTypeMap } from './constants';
import CompilationEmptyState from './empty-state';
import { CompilationLoadingCard } from './loading-card';

interface DatasetStructureViewProps {
  kind: StructureKind;
}

export function DatasetStructureView({ kind }: DatasetStructureViewProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const knowledgeBaseId = useKnowledgeBaseId();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const [graphKeywords, setGraphKeywords] = useState('');
  const [selectedNodeId, setSelectedNodeId] = useState('');
  const { data, loading } = useFetchDatasetStructureGraph(kind, graphKeywords);
  const template = data?.templates?.[0];

  const { data: structureRunData } = useTraceRunData(
    ViewModeGenerateTypeMap[kind],
  );
  const { status: structureStatus } = useGenerateStatus(structureRunData);

  useEffect(() => {
    if (structureStatus === GenerateStatus.completed) {
      queryClient.invalidateQueries({
        queryKey: DatasetStructureKeys.kind(knowledgeBaseId, kind),
      });
    }
  }, [structureStatus, queryClient, knowledgeBaseId, kind]);

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
    if (!name) {
      setGraphKeywords('');
      setSelectedNodeId('');
      return;
    }
    setSelectedNodeId(name);
  }, []);

  const handleNoMatchEnter = useCallback((keywords: string) => {
    setGraphKeywords(keywords);
    setSelectedNodeId('');
  }, []);

  const canGenerate = (knowledgeBase?.chunk_count ?? 0) > 0;

  if (loading && !data) {
    return <CompilationLoadingCard />;
  }

  if (!template && !graphKeywords) {
    return (
      <CompilationEmptyState
        type={kind}
        disabled={!canGenerate}
        data={structureRunData}
      />
    );
  }

  return (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
      {kind === ViewMode.Graph && (
        <div className="flex justify-end px-4 pt-4">
          <SelectWithSearch
            options={entityOptions}
            value={selectedEntityName || graphKeywords}
            onChange={handleSelectEntity}
            placeholder={t('knowledgeDetails.searchEntity')}
            allowClear
            triggerClassName="w-96 max-w-full"
            onNoMatchEnter={handleNoMatchEnter}
            disableAutoSelectOnEnter
          />
        </div>
      )}
      <RepresentationRenderer
        template={template}
        highlightNodeId={selectedEntityName || null}
      />
    </Card>
  );
}
