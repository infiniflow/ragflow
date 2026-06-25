import BackButton from '@/components/back-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { WikiDetailContent } from '@/pages/dataset/compilation/wiki-detail-content';
import {
  LeftPanelTab,
  WikiLeftPanel,
} from '@/pages/dataset/compilation/wiki-left-panel';
import { Routes } from '@/routes';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchKnowledgeBaseConfiguration,
  useFetchKnowledgeGraph,
} from '@/hooks/use-knowledge-request';
import { IArtifact } from '@/interfaces/database/dataset';
import KnowledgeForceGraph from '@/pages/dataset/compilation/knowledge-force-graph';

enum ViewMode {
  Graph = 'graph',
  LlmWiki = 'llm-wiki',
}

export default function Compilation() {
  const { t } = useTranslation();
  const { id } = useParams();
  const { navigateToDataFile } = useNavigatePage();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const { data: knowledgeGraph } = useFetchKnowledgeGraph();
  const [leftTab, setLeftTab] = useState<LeftPanelTab>(LeftPanelTab.Graph);
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.Graph);
  const [selectedArtifact, setSelectedArtifact] = useState<IArtifact | null>(
    null,
  );

  const handleSwitchToGraph = useCallback(() => {
    setViewMode(ViewMode.Graph);
  }, []);

  const handleSwitchToLlmWiki = useCallback(() => {
    setViewMode(ViewMode.LlmWiki);
  }, []);

  const handleLeftTabChange = useCallback((value: string) => {
    setLeftTab(value as LeftPanelTab);
  }, []);

  const handleSelectArtifact = useCallback((artifact: IArtifact) => {
    setSelectedArtifact(artifact);
  }, []);

  return (
    <section className="min-h-screen w-full flex flex-col p-4 gap-4 bg-bg-base">
      <header className="space-y-5">
        <BackButton
          to={`${Routes.DatasetBase}${Routes.Files}/${id}`}
          onClick={navigateToDataFile(id!)}
        >
          {t('common.back')}
        </BackButton>

        <section className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <RAGFlowAvatar
              avatar={knowledgeBase?.avatar}
              name={knowledgeBase?.name}
              className="size-10 rounded-lg"
            />
            <h2 className="text-xl font-medium text-text-primary">
              {knowledgeBase?.name}
              {t('knowledgeDetails.compilationTitleSuffix')}
            </h2>
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant={viewMode === ViewMode.Graph ? 'default' : 'outline'}
              size="sm"
              onClick={handleSwitchToGraph}
            >
              {t('knowledgeDetails.graph')}
            </Button>
            <Button
              variant={viewMode === ViewMode.LlmWiki ? 'default' : 'outline'}
              size="sm"
              onClick={handleSwitchToLlmWiki}
            >
              {t('knowledgeDetails.llmWiki')}
            </Button>
          </div>
        </section>
      </header>

      {viewMode === ViewMode.Graph ? (
        <div className="flex-1 min-h-0 flex flex-col">
          <KnowledgeForceGraph data={knowledgeGraph?.graph} show />
        </div>
      ) : (
        <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
          <ResizablePanelGroup direction="horizontal" className="flex-1">
            <ResizablePanel defaultSize={33} minSize={20} maxSize={50}>
              <WikiLeftPanel
                tab={leftTab}
                onTabChange={handleLeftTabChange}
                selectedArtifact={selectedArtifact}
                onSelectArtifact={handleSelectArtifact}
              />
            </ResizablePanel>
            <ResizableHandle withHandle />
            <ResizablePanel>
              <WikiDetailContent selectedArtifact={selectedArtifact} />
            </ResizablePanel>
          </ResizablePanelGroup>
        </Card>
      )}
    </section>
  );
}
