import BackButton from '@/components/back-button';
import MarkdownEditor from '@/components/markdown-editor';
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
import { useFetchDatasetSkillPage } from '@/hooks/use-dataset-skill-request';
import {
  useFetchKnowledgeBaseConfiguration,
  useFetchKnowledgeGraph,
} from '@/hooks/use-knowledge-request';
import { IArtifact } from '@/interfaces/database/dataset';
import KnowledgeForceGraph from '@/pages/dataset/compilation/knowledge-force-graph';
import { SkillsLeftPanel } from '@/pages/dataset/compilation/skills-left-panel';

enum ViewMode {
  Graph = 'graph',
  LlmWiki = 'llm-wiki',
  Skills = 'skills',
}

export default function Compilation() {
  const { t } = useTranslation();
  const { id } = useParams();
  const { navigateToDataFile } = useNavigatePage();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const { data: knowledgeGraph } = useFetchKnowledgeGraph();
  const [leftTab, setLeftTab] = useState<LeftPanelTab>(LeftPanelTab.Contents);
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.Graph);
  const [selectedArtifact, setSelectedArtifact] = useState<IArtifact | null>(
    null,
  );
  const [selectedSkill, setSelectedSkill] = useState<string | null>(null);
  const { data: skillPage } = useFetchDatasetSkillPage(selectedSkill);

  const handleSwitchToGraph = useCallback(() => {
    setViewMode(ViewMode.Graph);
  }, []);

  const handleSwitchToLlmWiki = useCallback(() => {
    setViewMode(ViewMode.LlmWiki);
  }, []);

  const handleSwitchToSkills = useCallback(() => {
    setViewMode(ViewMode.Skills);
  }, []);

  const handleLeftTabChange = useCallback((value: string) => {
    setLeftTab(value as LeftPanelTab);
  }, []);

  const handleSelectArtifact = useCallback((artifact: IArtifact) => {
    setSelectedArtifact(artifact);
  }, []);

  return (
    <section className="flex flex-col p-4 gap-4 h-full">
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
            <Button
              variant={viewMode === ViewMode.Skills ? 'default' : 'outline'}
              size="sm"
              onClick={handleSwitchToSkills}
            >
              {t('knowledgeDetails.skills')}
            </Button>
          </div>
        </section>
      </header>

      {viewMode === ViewMode.Graph ? (
        <div className="flex-1 min-h-0 flex flex-col">
          <KnowledgeForceGraph data={knowledgeGraph?.graph} show />
        </div>
      ) : viewMode === ViewMode.LlmWiki ? (
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
      ) : (
        <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
          <ResizablePanelGroup direction="horizontal" className="flex-1">
            <ResizablePanel defaultSize={33} minSize={20} maxSize={50}>
              <SkillsLeftPanel
                selectedSkill={selectedSkill}
                onSelectSkill={setSelectedSkill}
              />
            </ResizablePanel>
            <ResizableHandle withHandle />
            <ResizablePanel className="flex flex-col">
              <MarkdownEditor
                content={skillPage?.md_with_weight ?? ''}
                readOnly
              />
            </ResizablePanel>
          </ResizablePanelGroup>
        </Card>
      )}
    </section>
  );
}
