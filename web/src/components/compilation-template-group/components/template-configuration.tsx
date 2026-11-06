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

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SwitchFormField } from '@/components/switch-form-field';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { ICompilationTemplateBuiltin } from '@/interfaces/database/compilation-template';
import { isEmpty, startCase } from 'lodash';
import { ReactNode, useCallback, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { CompilationTemplateKind } from '@/constants/compilation';
import { OntologyCanvasEditor } from '../ontology/canvas-editor';
import { TreeTemplateFields } from './tree-template-fields';
import { useTemplateKindChange } from '../hooks/use-template-kind-change';
import { FormSchemaType } from '../schema';
import { SectionTitleKeyMap } from '../constant';

import { useActiveSectionTab } from '../hooks/use-active-section-tab';
import { useAvailableKindOptions } from '../hooks/use-available-kind-options';
import { useBuiltinTemplate } from '../hooks/use-builtin-template';
import { useFieldArrayHandlers } from '../hooks/use-field-array-handlers';
import { useFieldModal } from '../hooks/use-field-modal';
import { useTemplatePreviewSheets } from '../hooks/use-template-preview-sheets';
import { useTemplateSectionData } from '../hooks/use-template-section-data';

import { AddFieldModal } from './add-field-modal';
import { SectionFieldGrid } from './section-field-grid';
import { TemplatePreviewHeader } from './template-preview-header';

type TemplateConfigurationProps = {
  form: UseFormReturn<FormSchemaType>;
  builtins: ICompilationTemplateBuiltin[];
  kindOptions: { label: string; value: string }[];
  selectedTemplateIndex: number;
  children?: ReactNode;
};

export function TemplateConfiguration({
  form,
  builtins,
  kindOptions,
  selectedTemplateIndex,
  children,
}: TemplateConfigurationProps) {
  const { t } = useTranslation();

  const {
    addFieldModalOpen,
    editingFieldIndex,
    setEditingFieldIndex,
    handleModalOpenChange,
    handleOpenAddField,
    handleOpenEditField,
  } = useFieldModal();

  const kind = useWatch({
    control: form.control,
    name: `templates.${selectedTemplateIndex}.kind`,
  });
  const rechunk = useWatch({
    control: form.control,
    name: `templates.${selectedTemplateIndex}.config.rechunk`,
  });

  /**
   * The ontology layout folds its metadata away; the other kinds keep the
   * stacked form they have always had. Nothing is dropped — the same fields are
   * one click away, they just do not spend the canvas's height.
   */
  const [metaOpen, setMetaOpen] = useState(false);

  const availableKindOptions = useAvailableKindOptions(
    form,
    kindOptions,
    selectedTemplateIndex,
  );

  const { builtinTemplate, sectionNames } = useBuiltinTemplate(builtins, kind);

  const {
    jsonSheetOpen,
    setJsonSheetOpen,
    workflowSheetOpen,
    setWorkflowSheetOpen,
    allFormValues,
    templateName,
  } = useTemplatePreviewSheets(form, selectedTemplateIndex);

  const { activeSectionTab, setActiveSectionTab } =
    useActiveSectionTab(sectionNames);

  const handleKindChange = useTemplateKindChange({
    form,
    index: selectedTemplateIndex,
    builtins,
  });

  const { activeFieldsPath, builtinSection, existingFields, editingField } =
    useTemplateSectionData(
      form,
      selectedTemplateIndex,
      activeSectionTab,
      builtinTemplate,
      editingFieldIndex,
    );

  const { handleAddField } = useFieldArrayHandlers(
    form,
    activeFieldsPath,
    editingFieldIndex,
    setEditingFieldIndex,
  );

  const renderSectionTabs = useCallback(
    (sectionName: string) => {
      return (
        sectionName === activeSectionTab && (
          <SectionFieldGrid
            key={`${activeFieldsPath}-${kind}`}
            fieldsPath={activeFieldsPath}
            sectionName={sectionName}
            onOpenAddField={handleOpenAddField}
            onEditField={handleOpenEditField}
          />
        )
      );
    },
    [
      activeFieldsPath,
      activeSectionTab,
      kind,
      handleOpenAddField,
      handleOpenEditField,
    ],
  );

  // The fields are held as elements so both layouts render the same ones: the
  // ontology strip and the stacked form must never drift into two definitions of
  // "the template's name".
  const nameField = (
    <RAGFlowFormItem
      name={`templates.${selectedTemplateIndex}.name`}
      label={t('common.name')}
      required
    >
      <Input placeholder={t('common.namePlaceholder')} />
    </RAGFlowFormItem>
  );

  const descriptionField = (
    <RAGFlowFormItem
      name={`templates.${selectedTemplateIndex}.description`}
      label={t('knowledgeCompilation.description')}
    >
      <Textarea
        placeholder={t('common.descriptionPlaceholder')}
        rows={2}
        resize="vertical"
      />
    </RAGFlowFormItem>
  );

  const kindField = (
    <RAGFlowFormItem
      name={`templates.${selectedTemplateIndex}.kind`}
      label={t('knowledgeCompilation.builtinTemplates')}
      required
    >
      {(field) => (
        <SelectWithSearch
          value={field.value}
          onChange={(value) => handleKindChange(field, value)}
          disabled={field.disabled}
          options={availableKindOptions}
          placeholder={t('common.selectPlaceholder')}
        />
      )}
    </RAGFlowFormItem>
  );

  const globalRulesField = (
    <RAGFlowFormItem
      name={`templates.${selectedTemplateIndex}.config.global_rules`}
      label={t('knowledgeCompilation.globalRules')}
    >
      <Textarea
        placeholder={t('knowledgeCompilation.globalRulesPlaceholder')}
        rows={8}
        resize="vertical"
      />
    </RAGFlowFormItem>
  );

  const rechunkFields = (
    <>
      <SwitchFormField
        name={`templates.${selectedTemplateIndex}.config.rechunk`}
        label={t('knowledgeCompilation.rechunkInput')}
        tooltip={t('knowledgeCompilation.rechunkInputTip')}
        vertical={false}
      />
      {rechunk && (
        <RAGFlowFormItem
          name={`templates.${selectedTemplateIndex}.config.rechunk_rules`}
          label={t('knowledgeCompilation.rechunkRules')}
        >
          <Textarea
            placeholder={t('knowledgeCompilation.rechunkRulesPlaceholder')}
            rows={6}
            resize="vertical"
          />
        </RAGFlowFormItem>
      )}
    </>
  );

  const isOntology = kind === CompilationTemplateKind.Ontology;

  return (
    <>
      <TemplatePreviewHeader
        templateName={templateName}
        jsonSheetOpen={jsonSheetOpen}
        onJsonSheetOpenChange={setJsonSheetOpen}
        workflowSheetOpen={workflowSheetOpen}
        onWorkflowSheetOpenChange={setWorkflowSheetOpen}
        allFormValues={allFormValues}
      />
      {isOntology ? (
        // The canvas is the editor here, so it takes the whole column: the stacked
        // form used to sit above it inside a `max-w-4xl` scroll box, and a graph
        // squeezed into a few hundred pixels reads as a diagram, not a workspace.
        // The metadata keeps its fields but moves into a strip, and everything
        // that is not a class or a property folds away behind one button.
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="flex shrink-0 items-end gap-3 border-b border-border-button px-4 py-2">
            <div className="min-w-0 flex-1">{nameField}</div>
            <div className="w-56 shrink-0">{kindField}</div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-expanded={metaOpen}
              data-testid="ontology-meta-toggle"
              onClick={() => setMetaOpen((open) => !open)}
            >
              {metaOpen
                ? t('knowledgeCompilation.ontologyFoldMeta', {
                    defaultValue: 'Fold settings',
                  })
                : t('knowledgeCompilation.ontologyMoreSettings', {
                    defaultValue: 'More settings',
                  })}
            </Button>
          </div>

          {metaOpen && (
            // One column, full width each: a description and a rule list are
            // both prose, and half the width makes them a keyhole. The folded
            // area is capped and scrolls, because it is `shrink-0` inside the
            // column the canvas lives in — an uncapped fold would eat the canvas.
            <div className="flex max-h-[45%] shrink-0 flex-col gap-3 overflow-y-auto border-b border-border-button px-4 py-3">
              {descriptionField}
              {globalRulesField}
              {rechunkFields}
            </div>
          )}

          <div className="min-h-0 flex-1 p-3">
            <OntologyCanvasEditor
              form={form}
              templateIndex={selectedTemplateIndex}
            />
          </div>

          {children}
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto p-5">
          <div className="max-w-4xl mx-auto space-y-6">
            {nameField}
            {descriptionField}
            {kindField}
            {globalRulesField}

            {kind === CompilationTemplateKind.Artifacts && (
              <RAGFlowFormItem
                name={`templates.${selectedTemplateIndex}.config.mode`}
                label={t('knowledgeCompilation.wikiMode')}
                tooltip={t('knowledgeCompilation.wikiModeTip')}
              >
                {(field) => (
                  <SelectWithSearch
                    value={typeof field.value === 'string' ? field.value : ''}
                    onChange={field.onChange}
                    disabled={field.disabled}
                    options={[
                      {
                        label: t('knowledgeCompilation.entityMode'),
                        value: 'entity',
                      },
                      {
                        label: t('knowledgeCompilation.topicMode'),
                        value: 'topic',
                      },
                    ]}
                  />
                )}
              </RAGFlowFormItem>
            )}

            {kind === CompilationTemplateKind.Tree ? (
              <TreeTemplateFields index={selectedTemplateIndex} />
            ) : (
              <>
                {kind !== CompilationTemplateKind.Artifacts &&
                  !isEmpty(kind) && <>{rechunkFields}</>}
                {sectionNames.length > 0 && activeSectionTab && (
                  <Tabs
                    value={activeSectionTab}
                    onValueChange={setActiveSectionTab}
                    className="w-full"
                  >
                    <TabsList className="w-full justify-start">
                      {sectionNames.map((sectionName) => (
                        <TabsTrigger
                          key={sectionName}
                          value={sectionName}
                          className="flex-1"
                        >
                          {t(
                            SectionTitleKeyMap[sectionName] ??
                              startCase(sectionName),
                          )}
                        </TabsTrigger>
                      ))}
                    </TabsList>

                    {sectionNames.map((sectionName) => (
                      <TabsContent
                        key={sectionName}
                        value={sectionName}
                        className="mt-4"
                      >
                        {renderSectionTabs(sectionName)}
                      </TabsContent>
                    ))}
                  </Tabs>
                )}
              </>
            )}

            {children}
          </div>
        </div>
      )}

      <AddFieldModal
        open={addFieldModalOpen}
        onOpenChange={handleModalOpenChange}
        sectionName={activeSectionTab}
        builtinSection={builtinSection}
        existingFields={existingFields}
        initialField={editingField}
        onAdd={handleAddField}
      />
    </>
  );
}
