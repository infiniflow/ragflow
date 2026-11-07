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
import { Plus } from 'lucide-react';
import { FormEvent, MouseEvent, useCallback, useEffect, useState } from 'react';
import { useFieldArray, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { BlueprintSection } from './components/blueprint-section';
import { TemplateConfiguration } from './components/template-configuration';
import { CompilationTemplateGroupSaveResult } from './hooks/use-compilation-template-group-submit';
import { useEditNextCompilationTemplateGroup } from './hooks/use-edit-next-compilation-template-group';
import { buildConfigFromBuiltin } from './utils';

type CompilationTemplateGroupEditorProps = {
  id?: string;
  onSuccess: (result: CompilationTemplateGroupSaveResult) => void;
  onCancel: () => void;
  cancelText: string;
};

function unusedTemplateName(label: string, names: string[]) {
  if (!names.includes(label)) {
    return label;
  }
  let suffix = 2;
  while (names.includes(`${label}(${suffix})`)) {
    suffix += 1;
  }
  return `${label}(${suffix})`;
}

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
  const { append, remove } = useFieldArray({
    control: form.control,
    name: 'templates',
    keyName: 'fieldId',
  });
  const templates = useWatch({ control: form.control, name: 'templates' });
  const templateList = templates ?? [];
  const [selectedTemplateIndex, setSelectedTemplateIndex] = useState(0);

  useEffect(() => {
    if (selectedTemplateIndex > templateList.length - 1) {
      setSelectedTemplateIndex(Math.max(0, templateList.length - 1));
    }
  }, [selectedTemplateIndex, templateList.length]);

  const selectedKind = templateList[selectedTemplateIndex]?.kind;
  const isArtifacts = selectedKind === CompilationTemplateKind.Artifacts;

  const usedKinds = new Set(
    templateList.map((item) => item.kind).filter((kind) => Boolean(kind)),
  );
  const hasWiki = usedKinds.has(CompilationTemplateKind.Artifacts);
  const hasOtherKind = templateList.some(
    (item) => item.kind && item.kind !== CompilationTemplateKind.Artifacts,
  );
  const nextKind = hasWiki
    ? undefined
    : kindOptions.find((option) => {
        if (usedKinds.has(option.value)) {
          return false;
        }
        if (
          hasOtherKind &&
          option.value === CompilationTemplateKind.Artifacts
        ) {
          return false;
        }
        return true;
      });

  const handleSave = form.handleSubmit(onSubmit);

  const handleSubmit = useCallback(
    (event: FormEvent<HTMLFormElement>) => {
      event.stopPropagation();
      handleSave(event);
    },
    [handleSave],
  );

  const handleSelectTemplate = useCallback(
    (event: MouseEvent<HTMLButtonElement>) => {
      const index = Number(event.currentTarget.dataset.index);
      if (Number.isInteger(index)) {
        setSelectedTemplateIndex(index);
      }
    },
    [],
  );

  const handleAddTemplate = useCallback(() => {
    if (!nextKind) {
      return;
    }
    const builtin = builtins.find((item) => item.kind === nextKind.value);
    if (!builtin) {
      return;
    }
    const current = form.getValues('templates') ?? [];
    append({
      name: unusedTemplateName(
        nextKind.label,
        current.map((item) => item.name),
      ),
      description: '',
      kind: nextKind.value,
      config: buildConfigFromBuiltin(builtin, nextKind.value),
    });
    setSelectedTemplateIndex(current.length);
  }, [append, builtins, form, nextKind]);

  const handleRemoveTemplate = useCallback(() => {
    const current = form.getValues('templates') ?? [];
    if (current.length <= 1) {
      return;
    }
    const nextIndex =
      selectedTemplateIndex >= current.length - 1
        ? selectedTemplateIndex - 1
        : selectedTemplateIndex;
    remove(selectedTemplateIndex);
    setSelectedTemplateIndex(nextIndex);
  }, [form, remove, selectedTemplateIndex]);

  return (
    <Form {...form}>
      <form className="flex-1 min-h-0 flex flex-col" onSubmit={handleSubmit}>
        <div className="shrink-0 px-5 py-3 border-b border-border-button flex items-center gap-2 flex-wrap">
          {templateList.map((template, index) => (
            <Button
              key={template.id || `${template.kind}-${index}`}
              type="button"
              size="sm"
              variant={index === selectedTemplateIndex ? 'default' : 'outline'}
              data-index={index}
              onClick={handleSelectTemplate}
            >
              {template.name?.trim() ||
                kindOptions.find((option) => option.value === template.kind)
                  ?.label ||
                String(index + 1)}
            </Button>
          ))}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!nextKind}
            onClick={handleAddTemplate}
          >
            <Plus />
            {t('common.add')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="danger"
            className="ml-auto"
            disabled={templateList.length <= 1}
            onClick={handleRemoveTemplate}
          >
            {t('common.remove')}
          </Button>
        </div>
        <TemplateConfiguration
          key={selectedTemplateIndex}
          form={form}
          builtins={builtins}
          kindOptions={kindOptions}
          selectedTemplateIndex={selectedTemplateIndex}
        >
          {isArtifacts && (
            <BlueprintSection
              form={form}
              selectedTemplateIndex={selectedTemplateIndex}
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
