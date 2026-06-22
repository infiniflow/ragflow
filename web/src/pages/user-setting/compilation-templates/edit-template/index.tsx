import BackButton from '@/components/back-button';
import { ModelTreeSelectFormField } from '@/components/model-tree-select';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { Routes } from '@/routes';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';

import { ICompilationTemplateSection } from '@/interfaces/database/compilation-template';

import { FieldsSection } from './components/fields-section';
import { useEditCompilationTemplate } from './hooks/use-edit-compilation-template';
import {
  DefaultFieldKeys,
  getFieldKeyOrder,
  SectionTitleKeyMap,
} from './utils';

export default function EditCompilationTemplate() {
  const { t } = useTranslation();
  const {
    isCreate,
    form,
    watchedValues,
    kindOptions,
    sectionNames,
    builtinTemplate,
    typeOptions,
    onSubmit,
    isLoading,
    navigateToCompilationTemplates,
  } = useEditCompilationTemplate();

  return (
    <section className="h-full flex flex-col">
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex flex-col items-start gap-2">
        <BackButton
          to={`${Routes.UserSetting}${Routes.CompilationTemplates}`}
        />
        <h2 className="text-xl font-medium text-text-primary">
          {isCreate ? t('setting.addTemplate') : t('setting.editTemplate')}
        </h2>
      </header>

      <div className="flex-1 min-h-0 flex">
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="flex-1 min-w-0 flex flex-col min-h-0"
          >
            <div className="flex-1 min-h-0 overflow-y-auto p-5">
              <div className="max-w-2xl space-y-6 mx-auto">
                <RAGFlowFormItem
                  name="name"
                  label={t('setting.templateName')}
                  required
                >
                  <Input placeholder={t('common.namePlaceholder')} />
                </RAGFlowFormItem>

                <RAGFlowFormItem
                  name="description"
                  label={t('setting.templateDescription')}
                >
                  <Textarea
                    placeholder={t('common.descriptionPlaceholder')}
                    rows={3}
                  />
                </RAGFlowFormItem>

                <ModelTreeSelectFormField
                  name="llm_id"
                  label={t('setting.llmForExtraction')}
                  required
                />

                <RAGFlowFormItem
                  name="kind"
                  label={t('knowledgeCompilation.builtinTemplates')}
                  required
                >
                  <SelectWithSearch
                    options={kindOptions}
                    placeholder={t('common.selectPlaceholder')}
                  />
                </RAGFlowFormItem>

                {sectionNames.map((sectionName, index) => {
                  const builtinSection = builtinTemplate?.config?.[sectionName];

                  const firstField = (
                    builtinSection as ICompilationTemplateSection | undefined
                  )?.fields?.[0];
                  const firstFieldKeys = firstField
                    ? Object.keys(firstField)
                    : DefaultFieldKeys;
                  const fieldKeys = getFieldKeyOrder(firstFieldKeys);

                  return (
                    <FieldsSection
                      key={sectionName}
                      name={`config.${sectionName}` as `config.${string}`}
                      title={t(SectionTitleKeyMap[sectionName] ?? sectionName)}
                      fieldKeys={fieldKeys}
                      defaultOpen={index === 0}
                      typeOptions={typeOptions}
                    />
                  );
                })}

                <RAGFlowFormItem
                  name="config.global_rules"
                  label={t('setting.globalRules')}
                >
                  <Textarea
                    placeholder={t('setting.globalRulesPlaceholder')}
                    rows={4}
                  />
                </RAGFlowFormItem>
              </div>
            </div>

            <div className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-end gap-3">
              <Button
                type="button"
                variant="outline"
                onClick={navigateToCompilationTemplates}
                disabled={isLoading}
              >
                {t('common.cancel')}
              </Button>
              <Button type="submit" loading={isLoading} disabled={isLoading}>
                {t('common.confirm')}
              </Button>
            </div>
          </form>
        </Form>

        <aside className="flex-1 min-w-0 border-l border-border-button p-5 hidden lg:flex flex-col">
          <Tabs defaultValue="json" className="h-full flex flex-col min-h-0">
            <TabsList className="mb-3 self-start shrink-0">
              <TabsTrigger value="json">JSON</TabsTrigger>
              <TabsTrigger value="processFlow">
                {t('setting.processFlow')}
              </TabsTrigger>
            </TabsList>
            <TabsContent
              value="json"
              className="mt-0 flex-1 min-h-0 overflow-y-auto"
            >
              <JsonView
                src={watchedValues}
                displaySize
                collapseStringsAfterLength={100000000000}
                className="w-full h-full break-words p-2 bg-muted rounded-md"
              />
            </TabsContent>
            <TabsContent
              value="processFlow"
              className="mt-0 flex-1 min-h-0 overflow-y-auto"
            >
              <div className="w-full h-full flex items-center justify-center text-sm text-text-secondary">
                {t('setting.processFlowComingSoon')}
              </div>
            </TabsContent>
          </Tabs>
        </aside>
      </div>
    </section>
  );
}
