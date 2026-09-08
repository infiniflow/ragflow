import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { getEntityDisplayName } from '@/components/structure-graph/adapters';
import { RepresentationRenderer } from '@/components/structure-graph/representation-renderer';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { GenerateStatus } from '@/constants/knowledge';
import {
  useDatasetGenerate,
  useGenerateStatus,
  useTraceRunData,
} from '@/hooks/use-dataset-generate';
import {
  ArtifactAlterationKeys,
  DatasetStructureKeys,
  useDeleteDatasetStructure,
  useFetchArtifactAlteration,
  useFetchDatasetStructureGraph,
  useFetchKnowledgeBaseConfiguration,
  useKnowledgeBaseId,
} from '@/hooks/use-knowledge-request';
import { useQueryClient } from '@tanstack/react-query';
import { Trash2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  StructureKind,
  ViewMode,
  ViewModeGenerateTypeMap,
  ViewModeLabelKeyMap,
} from './constants';
import CompilationEmptyState from './empty-state';
import { useRunEndEffect } from './hooks/use-run-end-effect';
import { CompilationLoadingCard } from './loading-card';
import { CompilationUpdateButton } from './update-button';
import { UpdateLogSheet } from './update-log-sheet';

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
  const { deleteDatasetStructure, loading: deleting } =
    useDeleteDatasetStructure();

  const generateType = ViewModeGenerateTypeMap[kind];
  const { data: structureRunData } = useTraceRunData(generateType);
  const { status: structureStatus } = useGenerateStatus(structureRunData);

  const { data: alteration, loading: alterationLoading } =
    useFetchArtifactAlteration(kind);
  const { runGenerate, loading: runLoading } = useDatasetGenerate();
  const [updateSheetOpen, setUpdateSheetOpen] = useState(false);

  const newlyUploaded = alteration?.newly_uploaded ?? 0;
  const removed = alteration?.removed ?? 0;
  const retryPageCount = alteration?.retry_page_count ?? 0;
  const hasChanges = newlyUploaded > 0 || removed > 0 || retryPageCount > 0;

  const handleRunEnd = useCallback(() => {
    queryClient.invalidateQueries({
      queryKey: DatasetStructureKeys.kind(knowledgeBaseId, kind),
    });
    queryClient.invalidateQueries({
      queryKey: ArtifactAlterationKeys.detail(knowledgeBaseId, kind),
    });
  }, [queryClient, knowledgeBaseId, kind]);

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

  const handleDeleteStructure = useCallback(async () => {
    const code = await deleteDatasetStructure(kind);
    if (code === 0) {
      setGraphKeywords('');
      setSelectedNodeId('');
    }
  }, [deleteDatasetStructure, kind]);

  const handleUpdateClick = useCallback(async () => {
    setUpdateSheetOpen(true);
    if (structureStatus === GenerateStatus.Running) {
      return;
    }
    await runGenerate({ type: generateType }).catch(() => {});
  }, [structureStatus, runGenerate, generateType]);

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
      <div className="flex justify-between gap-4 px-4 pt-4">
        <div className="flex items-center gap-2">
          <ConfirmDeleteDialog
            title={t('knowledgeCompilation.deleteStructureConfirm', {
              name: t(ViewModeLabelKeyMap[kind]),
            })}
            onOk={handleDeleteStructure}
          >
            <Button variant="outline" size="sm" disabled={deleting}>
              <Trash2 />
            </Button>
          </ConfirmDeleteDialog>
          <CompilationUpdateButton
            traceData={structureRunData}
            generateType={generateType}
            hasChanges={hasChanges}
            newlyUploaded={newlyUploaded}
            removed={removed}
            retryPageCount={retryPageCount}
            loading={alterationLoading || runLoading}
            tooltip={t('knowledgeCompilation.updateStructureTooltip', {
              newlyUploaded,
              removed,
              name: t(ViewModeLabelKeyMap[kind]),
            })}
            onClick={handleUpdateClick}
          />
        </div>
        {kind === ViewMode.Graph && (
          <SelectWithSearch
            options={entityOptions}
            value={selectedEntityName || graphKeywords}
            onChange={handleSelectEntity}
            placeholder={t('knowledgeCompilation.searchEntity')}
            allowClear
            triggerClassName="w-96 max-w-full"
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
      <UpdateLogSheet
        open={updateSheetOpen}
        onOpenChange={setUpdateSheetOpen}
        data={structureRunData}
        title={t('knowledgeCompilation.updateStructureSheetTitle', {
          name: t(ViewModeLabelKeyMap[kind]),
        })}
      />
    </Card>
  );
}
