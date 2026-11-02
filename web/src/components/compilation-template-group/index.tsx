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

import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { CompilationTemplateKind } from '@/constants/compilation';
import { useMemo } from 'react';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { useEditNextCompilationTemplateGroup } from './hooks/use-edit-next-compilation-template-group';
import { CompilationTemplateGroupSaveResult } from './hooks/use-compilation-template-group-submit';
import { BlueprintSection } from './components/blueprint-section';
import { TemplateConfiguration } from './components/template-configuration';

const SelectedTemplateIndex = 0;

type CompilationTemplateGroupEditorProps = {
  id?: string;
  onSuccess: (result: CompilationTemplateGroupSaveResult) => void;
  onCancel: () => void;
  cancelText: string;
};

// Shared body of the edit-next route page and the create dialog: the form,
// its scrollable configuration area, and the cancel/save footer. The parent
// must give it a bounded-height flex column to fill.
export function CompilationTemplateGroupEditor({
  id,
  onSuccess,
  onCancel,
  cancelText,
}: CompilationTemplateGroupEditorProps) {
  const { t } = useTranslation();
  const { form, kindOptions, builtins, onSubmit, isLoading } =
    useEditNextCompilationTemplateGroup({ id, onSuccess });

  const selectedKind = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.kind`,
  });

  const isArtifacts = selectedKind === CompilationTemplateKind.Artifacts;

  const handleSave = useMemo(
    () => form.handleSubmit(onSubmit),
    [form, onSubmit],
  );

  return (
    <Form {...form}>
      <form className="flex-1 min-h-0 flex flex-col" onSubmit={handleSave}>
        <TemplateConfiguration
          form={form}
          builtins={builtins}
          kindOptions={kindOptions}
          selectedTemplateIndex={SelectedTemplateIndex}
        >
          {isArtifacts && (
            <BlueprintSection
              form={form}
              selectedTemplateIndex={SelectedTemplateIndex}
              builtins={builtins}
            />
          )}
        </TemplateConfiguration>

        <footer className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-end gap-5">
          <Button type="button" variant="outline" onClick={onCancel}>
            {cancelText}
          </Button>
          <Button type="submit" loading={isLoading}>
            {t('common.save')}
          </Button>
        </footer>
      </form>
    </Form>
  );
}
