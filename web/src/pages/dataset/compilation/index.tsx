import BackButton from '@/components/back-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { ViewMode } from './constants';
import { LlmWikiView } from './llm-wiki-view';
import { NavTreeView } from './nav-tree-view';
import { SkillsView } from './skills-view';

export default function Compilation() {
  const { t } = useTranslation();
  const { id } = useParams();
  const { navigateToDataFile } = useNavigatePage();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.LlmWiki);

  const handleSwitchToLlmWiki = useCallback(() => {
    setViewMode(ViewMode.LlmWiki);
  }, []);

  const handleSwitchToSkills = useCallback(() => {
    setViewMode(ViewMode.Skills);
  }, []);

  const handleSwitchToTree = useCallback(() => {
    setViewMode(ViewMode.Tree);
  }, []);

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
              To Skills
            </Button>

            <Button
              variant={viewMode === ViewMode.Tree ? 'default' : 'outline'}
              size="sm"
              onClick={handleSwitchToTree}
            >
              {t('knowledgeDetails.navTree')}
            </Button>
          </div>
        </section>
      </header>

      {viewMode === ViewMode.LlmWiki && <LlmWikiView />}
      {viewMode === ViewMode.Skills && <SkillsView />}
      {viewMode === ViewMode.Tree && <NavTreeView />}
    </section>
  );
}
