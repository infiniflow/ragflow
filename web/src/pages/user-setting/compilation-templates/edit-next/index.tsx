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

import { CompilationTemplateGroupEditor } from '@/components/compilation-template-group';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useCompilationTemplateSidebarCollapse } from '@/hooks/use-compilation-template-sidebar-collapse';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useSearchParams } from 'react-router';

export default function EditNextCompilationTemplate() {
  const { t } = useTranslation();
  const { navigateToAgentsFromCompilation } = useNavigatePage();
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const { collapsed, setCollapsed } = useCompilationTemplateSidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  useEffect(() => {
    setCollapsed(false);
    setPrimarySidebarCollapsed(true);
  }, [setCollapsed, setPrimarySidebarCollapsed]);

  const navigateToAgents = navigateToAgentsFromCompilation(
    searchParams.get('source'),
  );

  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    setPrimarySidebarCollapsed(true);
  }, [setCollapsed, setPrimarySidebarCollapsed]);

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <CompilationTemplateGroupEditor
        id={id}
        onSuccess={navigateToAgents}
        onCancel={navigateToAgents}
        cancelText={t('common.back')}
        sidebarCollapsed={collapsed}
        onSidebarCollapse={collapseSidebars}
      />
    </section>
  );
}
