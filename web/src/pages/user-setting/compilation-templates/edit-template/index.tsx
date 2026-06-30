import BackButton from '@/components/back-button';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { Routes } from '@/routes';
import { Plus } from 'lucide-react';
import { useMemo } from 'react';
import { useFieldArray, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';

import { CompilationTemplateKind } from '@/constants/compilation';
import { TemplateCard } from './components/template-card';
import { useEditCompilationTemplateGroup } from './hooks/use-edit-compilation-template-group';
import { DefaultTemplateValues } from './utils';

export default function EditCompilationTemplate() {
  const { t } = useTranslation();
  const {
    isCreate,
    form,
    watchedValues,
    kindOptions,
    builtins,
    onSubmit,
    isLoading,
    navigateToCompilationTemplates,
  } = useEditCompilationTemplateGroup();

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'templates',
  });

  const watchedTemplates = useWatch({
    control: form.control,
    name: 'templates',
  });

  const isAddTemplateDisabled = watchedTemplates?.some(
    (template) => template.kind === CompilationTemplateKind.Artifacts,
  );

  const hasNonArtifactsTemplate = watchedTemplates?.some(
    (template) =>
      template.kind && template.kind !== CompilationTemplateKind.Artifacts,
  );

  const availableKindOptions = useMemo(
    () =>
      hasNonArtifactsTemplate
        ? kindOptions.filter(
            (option) => option.value !== CompilationTemplateKind.Artifacts,
          )
        : kindOptions,
    [hasNonArtifactsTemplate, kindOptions],
  );

  const handleAddTemplate = () => {
    const firstTemplateLlmId = form.getValues('templates.0.llm_id');
    append({
      ...DefaultTemplateValues,
      llm_id: firstTemplateLlmId || '',
    });
  };

  return (
    <section className="h-full flex flex-col">
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex flex-col items-start gap-2">
        <BackButton
          to={`${Routes.UserSetting}${Routes.CompilationTemplates}`}
        />
        <h2 className="text-xl font-medium text-text-primary">
          {isCreate
            ? t('setting.addTemplateGroup')
            : t('setting.editTemplateGroup')}
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
                  label={t('setting.groupName')}
                  required
                >
                  <Input placeholder={t('common.namePlaceholder')} />
                </RAGFlowFormItem>

                <RAGFlowFormItem
                  name="description"
                  label={t('setting.groupDescription')}
                >
                  <Textarea
                    placeholder={t('common.descriptionPlaceholder')}
                    rows={3}
                  />
                </RAGFlowFormItem>

                <section className="space-y-4">
                  {fields.map((field, index) => (
                    <TemplateCard
                      key={field.id}
                      index={index}
                      form={form}
                      kindOptions={availableKindOptions}
                      builtins={builtins}
                      onRemove={remove}
                      canRemove={fields.length > 1}
                    />
                  ))}
                </section>

                <Button
                  type="button"
                  variant="outline"
                  onClick={handleAddTemplate}
                  disabled={isAddTemplateDisabled}
                >
                  <Plus className="size-4 mr-2" />
                  {t('setting.addTemplate')}
                </Button>
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
