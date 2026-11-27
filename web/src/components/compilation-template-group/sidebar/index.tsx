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

import {
  SecondarySidebar,
  SecondarySidebarMenu,
} from '@/components/secondary-sidebar';
import { Braces, LucideSettings } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { CompilationTemplateEditorView } from '../constant';

interface CompilationTemplateEditorSidebarProps {
  activeView: CompilationTemplateEditorView;
  templateName?: string;
  onViewChange: (view: CompilationTemplateEditorView) => void;
  onCollapse?: () => void;
}

export function CompilationTemplateEditorSidebar({
  activeView,
  templateName,
  onViewChange,
  onCollapse,
}: CompilationTemplateEditorSidebarProps) {
  const { t } = useTranslation();

  const showConfiguration = useCallback(() => {
    onViewChange(CompilationTemplateEditorView.Configuration);
  }, [onViewChange]);

  const showJson = useCallback(() => {
    onViewChange(CompilationTemplateEditorView.Json);
  }, [onViewChange]);

  const items = useMemo(
    () => [
      {
        key: CompilationTemplateEditorView.Configuration,
        icon: <LucideSettings className="size-4 shrink-0" />,
        label: t('knowledgeCompilation.configuration'),
        active: activeView === CompilationTemplateEditorView.Configuration,
        onSelect: showConfiguration,
      },
      {
        key: CompilationTemplateEditorView.Json,
        icon: <Braces className="size-4 shrink-0" />,
        label: 'JSON',
        active: activeView === CompilationTemplateEditorView.Json,
        onSelect: showJson,
      },
    ],
    [activeView, showConfiguration, showJson, t],
  );

  return (
    <SecondarySidebar
      title={onCollapse ? templateName || t('common.name') : undefined}
      onCollapse={onCollapse}
    >
      <SecondarySidebarMenu
        items={items}
        navigationLabel={t('knowledgeCompilation.configuration')}
        className={onCollapse ? undefined : 'pt-5'}
      />
    </SecondarySidebar>
  );
}
