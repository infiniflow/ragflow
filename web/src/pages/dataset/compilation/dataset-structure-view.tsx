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
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { StructureKind, ViewModeGenerateTypeMap } from './constants';
import CompilationEmptyState from './empty-state';

interface DatasetStructureViewProps {
  kind: StructureKind;
}

export function DatasetStructureView({ kind }: DatasetStructureViewProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const knowledgeBaseId = useKnowledgeBaseId();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const { data, loading } = useFetchDatasetStructureGraph(kind);
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

  const canGenerate = (knowledgeBase?.chunk_count ?? 0) > 0;

  if (loading) {
    return (
      <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
        <div className="flex items-center justify-center flex-1 text-text-secondary">
          {t('common.loading', 'Loading...')}
        </div>
      </Card>
    );
  }

  if (!template) {
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
      <RepresentationRenderer template={template} />
    </Card>
  );
}
