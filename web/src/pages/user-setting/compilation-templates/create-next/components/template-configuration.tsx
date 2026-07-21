import { ModelTreeSelectFormField } from '@/components/model-tree-select';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { ICompilationTemplateBuiltin } from '@/interfaces/database/compilation-template';
import { startCase } from 'lodash';
import { useCallback } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { CompilationTemplateKind } from '@/constants/compilation';
import { TreeTemplateFields } from '@/pages/user-setting/compilation-templates/create-next/components/tree-template-fields';
import { useTemplateKindChange } from '@/pages/user-setting/compilation-templates/create-next/hooks/use-template-kind-change';
import { FormSchemaType } from '@/pages/user-setting/compilation-templates/create-next/schema';
import { SectionTitleKeyMap } from '@/pages/user-setting/compilation-templates/create-next/utils';

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
  onNext: () => void;
  onBack: () => void;
  isArtifacts?: boolean;
  isLoading?: boolean;
};

export function TemplateConfiguration({
  form,
  builtins,
  kindOptions,
  selectedTemplateIndex,
  onNext,
  onBack,
  isArtifacts = false,
  isLoading = false,
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

  const { activeFieldsPath, builtinSection, editingField } =
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
            key={activeFieldsPath}
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
            label={t('setting.templateName')}
            required
          >
            <Input placeholder={t('common.namePlaceholder')} />
          </RAGFlowFormItem>

          <RAGFlowFormItem
            name={`templates.${selectedTemplateIndex}.description`}
            label={t('setting.templateDescription')}
          >
            <Textarea
              placeholder={t('common.descriptionPlaceholder')}
              rows={2}
              resize="vertical"
            />
          </RAGFlowFormItem>

          <ModelTreeSelectFormField
            name={`templates.${selectedTemplateIndex}.llm_id`}
            label={t('setting.llmForExtraction')}
            required
          />

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
            label={t('setting.globalRules')}
          >
            <Textarea
              placeholder={t('setting.globalRulesPlaceholder')}
              rows={8}
              resize="vertical"
            />
          </RAGFlowFormItem>

          {kind === CompilationTemplateKind.Tree ? (
            <TreeTemplateFields index={selectedTemplateIndex} />
          ) : (
            sectionNames.length > 0 &&
            activeSectionTab && (
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
            )
          )}
        </div>
      </div>

      <footer className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-end gap-5">
        <Button type="button" variant="outline" onClick={onBack}>
          {t('common.back')}
        </Button>
        <Button type="button" loading={isLoading} onClick={onNext}>
          {isArtifacts ? t('common.next') : t('common.save')}
        </Button>
      </footer>

      <AddFieldModal
        open={addFieldModalOpen}
        onOpenChange={handleModalOpenChange}
        sectionName={activeSectionTab}
        builtinSection={builtinSection}
        initialField={editingField}
        onAdd={handleAddField}
      />
    </>
  );
}
