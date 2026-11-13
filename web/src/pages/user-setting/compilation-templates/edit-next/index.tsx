/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import BackButton from '@/components/back-button';
import { CompilationModelsSheet } from '@/components/compilation-models-sheet';
import {
  CompilationTemplateGroupEditor,
  CompilationTemplateGroupEditorState,
} from '@/components/compilation-template-group';
import { Button } from '@/components/ui/button';
import { useCompilationTemplateSidebarCollapse } from '@/hooks/use-compilation-template-sidebar-collapse';
import { useFetchCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { Routes } from '@/routes';
import { isCreateCompilationTemplateGroup } from '@/utils/compilation-template-util';
import { Redo2, Undo2 } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams, useSearchParams } from 'react-router';

export default function EditNextCompilationTemplate() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const { navigateToAgentsFromCompilation } = useNavigatePage();
  const { collapsed, setCollapsed } = useCompilationTemplateSidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  useEffect(() => {
    setCollapsed(false);
    setPrimarySidebarCollapsed(true);
  }, [setCollapsed, setPrimarySidebarCollapsed]);
  /** Where the model's three workspaces hang off. */
  const basePath = Routes.CompilationTemplatesEditNext;

  const isCreate = isCreateCompilationTemplateGroup(id);
  const { data: group } = useFetchCompilationTemplateGroup(id);

  const navigateToAgents = navigateToAgentsFromCompilation(
    searchParams.get('source'),
  );

  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    setPrimarySidebarCollapsed(true);
  }, [setCollapsed, setPrimarySidebarCollapsed]);

  // Three questions about the same model, and they need three places: here what
  // it says, the panel below what it produced, and the two sheets here what it
  // produced and whether the version that says it is the one datasets compile
  // with. They read the form, so the editor hands its state up.
  const renderHeader = useCallback(
    ({
      isOntology,
      templateId,
      canUndo,
      canRedo,
      onUndo,
      onRedo,
      isDirty,
    }: CompilationTemplateGroupEditorState) => (
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex gap-3 items-center">
        <BackButton onClick={navigateToAgents} />
        <h2 className="font-medium text-text-secondary">
          {isCreate
            ? t('knowledgeCompilation.addTemplateGroup')
            : group?.name || t('knowledgeCompilation.editTemplateGroup')}
        </h2>
        {isOntology && (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              disabled={!canUndo}
              onClick={onUndo}
              title={t('knowledgeCompilation.undo', { defaultValue: 'Undo' })}
            >
              <Undo2 className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={!canRedo}
              onClick={onRedo}
              title={t('knowledgeCompilation.redo', { defaultValue: 'Redo' })}
            >
              <Redo2 className="size-4" />
            </Button>
            {isDirty && (
              <span className="text-xs text-state-warning">
                {t('knowledgeCompilation.unsavedChanges', {
                  defaultValue: 'Unsaved changes',
                })}
              </span>
            )}
          </div>
        )}
        <div className="ml-auto flex items-center gap-3">
          {/* The three workspaces that answer different questions about the same
              model. They are pages, not sheets: browsing a few hundred rows is
              what a reviewer does, and a modal gets in the way of it.

              They hang off the group's URL (`:id`), not the template's: the
              group is what the editor route addresses, and a sub-page that
              carried the template id fetched a group by a foreign id — 404 —
              while its body asked the model APIs about a model the id did not
              name. A group that was never saved has no compiled rows, so the
              links stay off until it has an id. */}
          {isOntology && !isCreate && templateId && (
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => navigate(`${basePath}/${id}/instances`)}
              >
                {t('knowledgeCompilation.ontologyOpenInstances', {
                  defaultValue: 'Instances',
                })}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => navigate(`${basePath}/${id}/actions`)}
              >
                {t('knowledgeCompilation.ontologyOpenActions', {
                  defaultValue: 'Actions & bindings',
                })}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => navigate(`${basePath}/${id}/action-audit`)}
              >
                {t('knowledgeCompilation.ontologyOpenAudit', {
                  defaultValue: 'Audit',
                })}
              </Button>
            </div>
          )}
          <CompilationModelsSheet templateId={templateId} />
        </div>
      </header>
    ),
    [basePath, group?.name, id, isCreate, navigate, navigateToAgents, t],
  );

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <CompilationTemplateGroupEditor
        id={id}
        onSuccess={navigateToAgents}
        onCancel={navigateToAgents}
        cancelText={t('common.back')}
        renderHeader={renderHeader}
        sidebarCollapsed={collapsed}
        onSidebarCollapse={collapseSidebars}
      />
    </section>
  );
}
