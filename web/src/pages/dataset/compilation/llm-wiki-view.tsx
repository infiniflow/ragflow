import { Card } from '@/components/ui/card';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import {
  ArtifactKeys,
  ArtifactTopicKeys,
  useFetchArtifactTopicList,
  useFetchKnowledgeBaseConfiguration,
} from '@/hooks/use-knowledge-request';
import {
  GenerateStatus,
  GenerateType,
} from '@/pages/dataset/dataset/generate-button/constants';
import { useTraceRunData } from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router';

import { LeftPanelTab } from './constants';
import CompilationEmptyState from './empty-state';
import { useCompilationArtifact } from './hooks/use-compilation-artifact';
import { CompilationLoadingCard } from './loading-card';
import { WikiDetailContent } from './wiki-detail-content';
import { WikiLeftPanel } from './wiki-left-panel';

export function LlmWikiView() {
  const { id } = useParams();
  const queryClient = useQueryClient();
  const [leftTab, setLeftTab] = useState<LeftPanelTab>(LeftPanelTab.Contents);
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const { topics, loading: topicListLoading } = useFetchArtifactTopicList();
  const {
    selectedArtifact,
    selectedVersion,
    selectVersion,
    handleSelectArtifact,
    clearSelectedArtifact,
  } = useCompilationArtifact();

  const { data: artifactRunData } = useTraceRunData(GenerateType.Artifact);
  const { status: artifactStatus } = useGenerateStatus(artifactRunData);

  useEffect(() => {
    if (artifactStatus === GenerateStatus.completed) {
      queryClient.invalidateQueries({
        queryKey: ArtifactKeys.listByDataset(id!),
      });
      queryClient.invalidateQueries({
        queryKey: ArtifactTopicKeys.listByDataset(id!),
      });
    }
  }, [artifactStatus, queryClient, id]);

  const handleLeftTabChange = useCallback((value: string) => {
    setLeftTab(value as LeftPanelTab);
  }, []);

  const canGenerate = (knowledgeBase?.chunk_count ?? 0) > 0;
  const isLoading = topicListLoading && topics.length === 0;
  const isEmpty = topics.length === 0 && !topicListLoading;

  if (isLoading) {
    return <CompilationLoadingCard />;
  }

  if (isEmpty) {
    return (
      <CompilationEmptyState
        type="llm-wiki"
        disabled={!canGenerate}
        data={artifactRunData}
      />
    );
  }

  return (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
      <ResizablePanelGroup direction="horizontal" className="flex-1">
        <ResizablePanel defaultSize={33} minSize={20} maxSize={50}>
          <WikiLeftPanel
            tab={leftTab}
            onTabChange={handleLeftTabChange}
            selectedArtifact={selectedArtifact}
            onSelectArtifact={handleSelectArtifact}
            onClearWiki={clearSelectedArtifact}
          />
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel>
          <WikiDetailContent
            selectedArtifact={selectedArtifact}
            selectedVersion={selectedVersion}
            onSelectVersion={selectVersion}
            onSelectArtifact={handleSelectArtifact}
          />
        </ResizablePanel>
      </ResizablePanelGroup>
    </Card>
  );
}
