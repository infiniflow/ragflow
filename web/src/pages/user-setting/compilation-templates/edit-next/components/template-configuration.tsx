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
import { SwitchFormField } from '@/components/switch-fom-field';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { ICompilationTemplateBuiltin } from '@/interfaces/database/compilation-template';
import { startCase } from 'lodash';
import { ReactNode, useCallback } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { CompilationTemplateKind } from '@/constants/compilation';
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
      <div className="flex-1 min-h-0 overflow-y-auto p-5">
        <div className="max-w-4xl mx-auto space-y-6">
          <RAGFlowFormItem
            name={`templates.${selectedTemplateIndex}.name`}
            label={t('common.name')}
            required
          >
            <Input placeholder={t('common.namePlaceholder')} />
          </RAGFlowFormItem>

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
                    { label: t('knowledgeCompilation.entityMode'), value: 'entity' },
                    { label: t('knowledgeCompilation.topicMode'), value: 'topic' },
                  ]}
                />
              )}
            </RAGFlowFormItem>
          )}

          {kind === CompilationTemplateKind.Tree ? (
            <TreeTemplateFields index={selectedTemplateIndex} />
          ) : (
            <>
              {kind !== CompilationTemplateKind.Artifacts && (
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
              )}
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
