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
import { ReactNode, useMemo } from 'react';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { BlueprintSection } from './components/blueprint-section';
import { TemplateConfiguration } from './components/template-configuration';
import { CompilationTemplateGroupSaveResult } from './hooks/use-compilation-template-group-submit';
import { useEditNextCompilationTemplateGroup } from './hooks/use-edit-next-compilation-template-group';
import {
  draftFromSections,
  OntologySection,
  validateDraft,
} from './ontology/model';

const SelectedTemplateIndex = 0;

/**
 * Form state the surrounding page needs to render its own header: the
 * compiled-output panels ask about the model the form is declaring, so they
 * cannot live inside the editor without leaking into the create dialog.
 */
export type CompilationTemplateGroupEditorState = {
  isOntology: boolean;
  templateId?: string;
  ontologyClasses: string[];
  ontologyProperties: string[];
};

type CompilationTemplateGroupEditorProps = {
  id?: string;
  onSuccess: (result: CompilationTemplateGroupSaveResult) => void;
  onCancel: () => void;
  cancelText: string;
  renderHeader?: (state: CompilationTemplateGroupEditorState) => ReactNode;
};

// Shared body of the edit-next route page and the create dialog: the form,
// its scrollable configuration area, and the cancel/save footer. The parent
// must give it a bounded-height flex column to fill.
export function CompilationTemplateGroupEditor({
  id,
  onSuccess,
  onCancel,
  cancelText,
  renderHeader,
}: CompilationTemplateGroupEditorProps) {
  const { t } = useTranslation();
  const { form, kindOptions, builtins, onSubmit, isLoading } =
    useEditNextCompilationTemplateGroup({ id, onSuccess });

  const selectedKind = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.kind`,
  });

  const isArtifacts = selectedKind === CompilationTemplateKind.Artifacts;
  const isOntology = selectedKind === CompilationTemplateKind.Ontology;

  // A model that was never saved has produced nothing, so the compiled-output
  // panel stays disabled until the form carries an id.
  const templateId = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.id`,
  });

  // The service rejects an ontology template whose cross-references do not
  // resolve, and that rejection arrives with nothing on screen (see the schema
  // refinement). Computing the same findings here is what lets the footer say
  // WHY the save is blocked instead of leaving a dead button.
  const entitySection = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.config.entity`,
  }) as OntologySection | undefined;
  const relationSection = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.config.relation`,
  }) as OntologySection | undefined;
  const ontologyIssues = useMemo(
    () =>
      isOntology
        ? validateDraft(draftFromSections(entitySection, relationSection))
        : [],
    [entitySection, relationSection, isOntology],
  );

  // What the model declares, which is what the compiled-output panel can be
  // asked about: an instance list is per class, an assertion list per property.
  const ontologyClasses = useMemo(
    () =>
      (entitySection?.fields ?? [])
        .map((field) => String(field.type ?? ''))
        .filter(Boolean),
    [entitySection],
  );
  const ontologyProperties = useMemo(
    () =>
      (relationSection?.fields ?? [])
        .map((field) => String(field.type ?? ''))
        .filter(Boolean),
    [relationSection],
  );

  const handleSave = useMemo(
    () => form.handleSubmit(onSubmit),
    [form, onSubmit],
  );

  return (
    <>
      {renderHeader?.({
        isOntology,
        templateId,
        ontologyClasses,
        ontologyProperties,
      })}
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
            {ontologyIssues.length > 0 && (
              <span className="mr-auto text-xs text-[#F0A020]">
                {t('knowledgeCompilation.ontologySaveBlocked', {
                  count: ontologyIssues.length,
                  defaultValue:
                    '{{count}} ontology problem(s) must be fixed first — the canvas lists them.',
                })}
              </span>
            )}
            <Button type="button" variant="outline" onClick={onCancel}>
              {cancelText}
            </Button>
            <Button
              type="submit"
              loading={isLoading}
              disabled={ontologyIssues.length > 0}
            >
              {t('common.save')}
            </Button>
          </footer>
        </form>
      </Form>
    </>
  );
}
