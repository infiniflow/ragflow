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

import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Clipboard, ClipboardPlus, FileInput, LucideIcon } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { AgentTabCategory, FlowType, FlowTypeConfig } from '../constant';

export interface AgentCreateAction {
  key: 'blank' | 'template' | 'import';
  icon: LucideIcon;
  label: string;
  testId?: string;
  onClick: () => void;
}

// The active tab decides which create actions exist: compilation groups only
// support blank creation, while workflow/pipeline also offer templates and
// JSON import.
export function useAgentCreateActions(
  activeType: FlowType,
  activeCategory: AgentTabCategory,
  showCreatingModal: () => void,
  handleImportJson: () => void,
): AgentCreateAction[] {
  const { t } = useTranslation();
  const { navigateToAgentTemplates, navigateToCompilationEditor } =
    useNavigatePage();

  const handleCreateFromTemplate = useCallback(() => {
    navigateToAgentTemplates(activeCategory);
  }, [navigateToAgentTemplates, activeCategory]);

  const handleNavigateToCompilationEditor = useCallback(() => {
    navigateToCompilationEditor();
  }, [navigateToCompilationEditor]);

  return useMemo(() => {
    const config = FlowTypeConfig[activeType];
    return [
      {
        key: 'blank' as const,
        icon: Clipboard,
        label: t('flow.createFromBlank'),
        testId: undefined,
        onClick:
          activeType === FlowType.Compiler
            ? handleNavigateToCompilationEditor
            : showCreatingModal,
      },
      ...(config.supportsTemplates
        ? [
            {
              key: 'template' as const,
              icon: ClipboardPlus,
              label: t('flow.createFromTemplate'),
              testId: undefined,
              onClick: handleCreateFromTemplate,
            },
          ]
        : []),
      ...(config.supportsImport
        ? [
            {
              key: 'import' as const,
              icon: FileInput,
              label: t('flow.importJsonFile'),
              testId: 'agent-import-json',
              onClick: handleImportJson,
            },
          ]
        : []),
    ];
  }, [
    activeType,
    t,
    handleNavigateToCompilationEditor,
    showCreatingModal,
    handleCreateFromTemplate,
    handleImportJson,
  ]);
}
