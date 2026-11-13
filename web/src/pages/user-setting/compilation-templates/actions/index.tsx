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
import { ActionWorkbench } from '@/components/compilation-template-group/ontology/action-workbench';
import { useFetchCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import { Routes } from '@/routes';
import { ontologyTemplateIdOf } from '@/utils/compilation-template-util';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

/**
 * The action workbench as a page rather than a sheet.
 *
 * Binding and execution are security operations and live on their own route:
 * mixed into the editor, the same panel would put an operator-level entry point
 * behind a modeller-level page. The body is shared with the sheet so the two can
 * never disagree about a verdict.
 */
export default function CompilationTemplateActionsPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { data: group } = useFetchCompilationTemplateGroup(id);
  // `:id` addresses the group; the workbench is keyed by the template row.
  const templateId = ontologyTemplateIdOf(group);
  const backTo = `${Routes.CompilationTemplatesEditNext}/${id}`;

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex gap-3 items-center">
        <BackButton to={backTo} />
        <h2 className="font-medium text-text-secondary">
          {t('knowledgeCompilation.ontologyActionsPage', {
            name: group?.name ?? '',
            defaultValue: 'Actions & bindings — {{name}}',
          })}
        </h2>
      </header>
      <div className="flex-1 min-h-0 p-4 flex flex-col">
        {/* The template id only exists once the group has loaded; until then the
            workbench must not fire model-scoped reads with an empty id. */}
        <ActionWorkbench templateId={templateId} enabled={!!templateId} />
      </div>
    </section>
  );
}
