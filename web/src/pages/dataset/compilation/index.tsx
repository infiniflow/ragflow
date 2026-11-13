import BackButton from '@/components/back-button';
import { CompilationModelsSheet } from '@/components/compilation-models-sheet';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { StructureKinds, ViewMode } from './constants';
import { DatasetStructureView } from './dataset-structure-view';
import { LlmWikiView } from './llm-wiki-view';
import { NavTreeView } from './nav-tree-view';
import { SkillsView } from './skills-view';

export default function Compilation() {
  const { t } = useTranslation();
  const { id } = useParams();
  const { navigateToDataFile } = useNavigatePage();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.LlmWiki);
  const [artifactsOpen, setArtifactsOpen] = useState(true);

  const handleViewModeChange = useCallback((value: ViewMode) => {
    setViewMode(value);
  }, []);

  const structureKind = StructureKinds.find((kind) => kind === viewMode);
  const viewProps = {
    viewMode,
    artifactsOpen,
    onArtifactsOpenChange: setArtifactsOpen,
    onViewModeChange: handleViewModeChange,
  };

  return (
    <section className="flex flex-col p-4 gap-4 h-full min-h-0">
      <header className="flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-3">
          <BackButton onClick={navigateToDataFile(id!)}>
            {t('common.back')}
          </BackButton>
          <RAGFlowAvatar
            avatar={knowledgeBase?.avatar}
            name={knowledgeBase?.name}
            className="size-10 rounded-lg"
          />
          <h2 className="text-xl font-medium text-text-primary">
            {knowledgeBase?.name}
            {t('knowledgeCompilation.compilationTitleSuffix')}
          </h2>
        </div>

        {/* The artifacts below were produced by a model at some version. This is
            the way from the output back to the contract that produced it. The
            view selector itself lives in the view's own navigation, so this
            header keeps only what the views cannot answer. */}
        <CompilationModelsSheet />
      </header>

      {viewMode === ViewMode.LlmWiki && <LlmWikiView {...viewProps} />}
      {viewMode === ViewMode.Skills && <SkillsView />}
      {viewMode === ViewMode.Tree && <NavTreeView {...viewProps} />}
      {structureKind && (
        <DatasetStructureView kind={structureKind} {...viewProps} />
      )}
    </section>
  );
}
