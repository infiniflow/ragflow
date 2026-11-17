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

import { SecondarySidebarHeader } from '@/components/sidebar-header';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { Braces, LucideSettings } from 'lucide-react';
import { useCallback } from 'react';
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

  return (
    <aside className="flex w-64 shrink-0 flex-col border-r border-border-button">
      {onCollapse && (
        <SecondarySidebarHeader
          title={templateName || t('common.name')}
          onCollapse={onCollapse}
        />
      )}

      <nav
        className={cn(
          'overflow-y-auto px-5 pb-5',
          onCollapse ? 'pt-1' : 'pt-5',
        )}
        aria-label={t('knowledgeCompilation.configuration')}
      >
        <ul className="space-y-2">
          <li>
            <Button
              type="button"
              block
              variant="ghost"
              aria-pressed={
                activeView === CompilationTemplateEditorView.Configuration
              }
              className={cn(
                'relative h-10 justify-start gap-2.5 px-3 text-base',
                activeView === CompilationTemplateEditorView.Configuration &&
                  'bg-bg-card text-text-primary',
              )}
              onClick={showConfiguration}
            >
              <LucideSettings className="size-4 shrink-0" />
              <span>{t('knowledgeCompilation.configuration')}</span>
            </Button>
          </li>
          <li>
            <Button
              type="button"
              block
              variant="ghost"
              aria-pressed={activeView === CompilationTemplateEditorView.Json}
              className={cn(
                'relative h-10 justify-start gap-2.5 px-3 text-base',
                activeView === CompilationTemplateEditorView.Json &&
                  'bg-bg-card text-text-primary',
              )}
              onClick={showJson}
            >
              <Braces className="size-4 shrink-0" />
              <span>JSON</span>
            </Button>
          </li>
        </ul>
      </nav>
    </aside>
  );
}
