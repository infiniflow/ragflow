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
import { ActionAuditTable } from '@/components/compilation-template-group/ontology/action-workbench-sheet';
import { useFetchCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import { useFetchActionExecutions } from '@/hooks/use-ontology-action-request';
import { Routes } from '@/routes';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

/**
 * Execution audit: every attempt, refusals included.
 *
 * Read-only on purpose. A trail that can be edited is not evidence, so there is
 * no delete here even for a tenant administrator.
 */
export default function CompilationTemplateActionAuditPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { data: group } = useFetchCompilationTemplateGroup(id);
  const [page, setPage] = useState(1);
  const { executions, total } = useFetchActionExecutions(id, page, !!id);
  const backTo = `${Routes.CompilationTemplatesEditNext}/${id}`;

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex gap-3 items-center">
        <BackButton to={backTo} />
        <h2 className="font-medium text-text-secondary">
          {t('knowledgeCompilation.ontologyActionAuditPage', {
            name: group?.name ?? '',
            defaultValue: 'Action audit — {{name}}',
          })}
        </h2>
      </header>
      <div className="flex-1 min-h-0 p-4 flex flex-col">
        <ActionAuditTable
          filterable
          executions={executions ?? []}
          total={total ?? 0}
          page={page}
          onPage={setPage}
        />
      </div>
    </section>
  );
}
