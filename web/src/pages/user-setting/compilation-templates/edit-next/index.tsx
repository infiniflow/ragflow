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
import { OntologyInspectSheet } from '@/components/compilation-template-group/ontology/inspect-sheet';
import { useFetchCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import { isCreateCompilationTemplateGroup } from '@/utils/compilation-template-util';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams, useSearchParams } from 'react-router';

import { resolveAgentsUrl } from './utils';

export default function EditNextCompilationTemplate() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const agentsUrl = resolveAgentsUrl(searchParams.get('source'));

  const isCreate = isCreateCompilationTemplateGroup(id);
  const { data: group } = useFetchCompilationTemplateGroup(id);

  const navigateToAgents = useCallback(() => {
    navigate(agentsUrl);
  }, [navigate, agentsUrl]);

  // Three questions about the same model, and they need three places: here what
  // it says, the panel below what it produced, and the two sheets here what it
  // produced and whether the version that says it is the one datasets compile
  // with. They read the form, so the editor hands its state up.
  const renderHeader = useCallback(
    ({
      isOntology,
      templateId,
      ontologyClasses,
      ontologyProperties,
    }: CompilationTemplateGroupEditorState) => (
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex gap-3 items-center">
        <BackButton to={agentsUrl} />
        <h2 className="font-medium text-text-secondary">
          {isCreate
            ? t('knowledgeCompilation.addTemplateGroup')
            : group?.name || t('knowledgeCompilation.editTemplateGroup')}
        </h2>
        <div className="ml-auto flex items-center gap-3">
          {isOntology && (
            <OntologyInspectSheet
              templateId={templateId}
              classes={ontologyClasses}
              properties={ontologyProperties}
            />
          )}
          <CompilationModelsSheet templateId={templateId} />
        </div>
      </header>
    ),
    [agentsUrl, group?.name, isCreate, t],
  );

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <CompilationTemplateGroupEditor
        id={id}
        onSuccess={navigateToAgents}
        onCancel={navigateToAgents}
        cancelText={t('common.back')}
        renderHeader={renderHeader}
      />
    </section>
  );
}
