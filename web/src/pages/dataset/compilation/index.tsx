import BackButton from '@/components/back-button';
import MarkdownEditor from '@/components/markdown-editor';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { SkeletonCard } from '@/components/skeleton-card';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchArtifactTopicList,
  useFetchKnowledgeBaseConfiguration,
  useFetchKnowledgeGraph,
} from '@/hooks/use-knowledge-request';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { ViewMode } from './constants';
import CompilationEmptyState from './empty-state';
import { useCompilationArtifact } from './hooks/use-compilation-artifact';
import { useCompilationSkill } from './hooks/use-compilation-skill';
import { useCompilationView } from './hooks/use-compilation-view';
import KnowledgeForceGraph from './knowledge-force-graph';
import { SkillsLeftPanel } from './skills-left-panel';
import { WikiDetailContent } from './wiki-detail-content';
import { WikiLeftPanel } from './wiki-left-panel';

export default function Compilation() {
  const { t } = useTranslation();
  const { id } = useParams();
  const { navigateToDataFile } = useNavigatePage();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const { data: knowledgeGraph, loading: knowledgeGraphLoading } =
    useFetchKnowledgeGraph();
  const { topics, loading: topicListLoading } = useFetchArtifactTopicList();

  const {
    leftTab,
    viewMode,
    artifactRunData,
    skillRunData,
    handleSwitchToLlmWiki,
    handleSwitchToSkills,
    handleLeftTabChange,
  } = useCompilationView();

  const {
    selectedArtifact,
    selectedVersion,
    selectVersion,
    handleSelectArtifact,
    clearSelectedArtifact,
  } = useCompilationArtifact();

  const {
    selectedSkill,
    setSelectedSkill,
    skillTree,
    skillTreeLoading,
    skillPage,
  } = useCompilationSkill();

  const isLlmWikiEmpty = topics.length === 0 && !topicListLoading;
  const canGenerate = (knowledgeBase?.chunk_count ?? 0) > 0;

  const isGraphLoading = knowledgeGraphLoading && !knowledgeGraph?.graph;
  const isLlmWikiLoading = topicListLoading && topics.length === 0;
  const isSkillsLoading =
    skillTreeLoading && !skillTree?.skill_with_weight?.length;
  const isSkillsEmpty =
    !skillTree?.skill_with_weight?.length && !skillTreeLoading;

  const loadingCard = (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col p-8">
      <SkeletonCard className="flex-1" />
    </Card>
  );

  return (
    <section className="flex flex-col p-4 gap-4 h-full">
      <header className="space-y-5">
        <BackButton onClick={navigateToDataFile(id!)}>
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
        isGraphLoading ? (
          loadingCard
        ) : (
          <div className="flex-1 min-h-0 flex flex-col">
            <KnowledgeForceGraph data={knowledgeGraph?.graph} show />
          </div>
        )
      ) : viewMode === ViewMode.LlmWiki ? (
        isLlmWikiLoading ? (
          loadingCard
        ) : isLlmWikiEmpty ? (
          <CompilationEmptyState
            type="llm-wiki"
            disabled={!canGenerate}
            data={artifactRunData}
          />
        ) : (
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
        )
      ) : isSkillsLoading ? (
        loadingCard
      ) : isSkillsEmpty ? (
        <CompilationEmptyState
          type="skills"
          disabled={!canGenerate}
          data={skillRunData}
        />
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
