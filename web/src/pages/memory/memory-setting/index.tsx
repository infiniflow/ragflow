import { DynamicForm } from '@/components/dynamic-form';
import { SettingsFormLayout } from '@/components/settings-form-layout';
import {
  ModelResourceAccessUserIdContext,
  ModelResourceTenantIdContext,
} from '@/hooks/use-llm-request';
import { Button } from '@/components/ui/button';
import Divider from '@/components/ui/divider';
import { Form } from '@/components/ui/form';
import { TopTitle } from '@/pages/dataset/dataset-title';
import { IMemory } from '@/pages/memories/interface';
import { showPermissionField } from '@/ee/constants/feature-flags';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { FieldValues, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useFetchMemoryBaseConfiguration } from '../hooks/use-memory-setting';
import {
  AdvancedSettingsForm,
  defaultAdvancedSettingsForm,
} from './advanced-settings-form';
import { BasicInfo, defaultBasicInfo } from './basic-form';
import { useUpdateMemoryConfig } from './hook';
import { MemoryModelForm, defaultMemoryModelForm } from './memory-model-form';
import {
  useMemoryFormSchema,
  useRevalidatePersistedModels,
} from './memory-setting-hooks';
import { MemorySettingProvider } from './memory-setting-context';
import {
  canManageMemoryConfig,
  useCanWriteMemoryRole,
} from '@/ee/pages/memories/memory-dropdown-guard';

export default function MemoryMessage() {
  const { t } = useTranslation();
  const { data, loading: memoryConfigLoading } =
    useFetchMemoryBaseConfiguration();
  const { formSchema, modelsFetched, modelsEditable } = useMemoryFormSchema(
    data?.tenant_id,
    memoryConfigLoading ? null : data?.user_id,
  );
  const form = useForm<IMemory>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      id: '',
      ...defaultBasicInfo,
      ...defaultMemoryModelForm,
      ...defaultAdvancedSettingsForm,
    } as unknown as IMemory,
  });
  const { onMemoryRenameOk, loading } = useUpdateMemoryConfig();
  const hasRoleWrite = useCanWriteMemoryRole(data?.tenant_id ?? '');
  const canEditConfig =
    hasRoleWrite && canManageMemoryConfig(data?.operator_permission ?? 0);

  useEffect(() => {
    form.reset({
      id: data?.id,
      embd_id: data?.embd_id,
      llm_id: data?.llm_id,
      name: data?.name || '',
      description: data?.description || '',
      avatar: data?.avatar || '',
      memory_size: data?.memory_size,
      memory_type: data?.memory_type,
      temperature: data?.temperature,
      system_prompt: data?.system_prompt || '',
      user_prompt: data?.user_prompt || '',
      forgetting_policy: data?.forgetting_policy || 'FIFO',
      storage_type: data?.storage_type || 'Table',
      ...(showPermissionField
        ? { permissions: data?.permissions || 'me' }
        : {}),
    });
  }, [data, form]);

  useRevalidatePersistedModels({
    control: form.control,
    trigger: form.trigger,
    clearErrors: form.clearErrors,
    modelsFetched,
    modelsEditable,
  });

  const onSubmit = (data: IMemory) => {
    onMemoryRenameOk(data);
  };

  const handleReset = () => {
    form.reset();
  };

  const handleSavingSubmit = (value: FieldValues) => {
    onSubmit(value as IMemory);
  };

  const memoryActions = canEditConfig ? (
    <>
      <Button
        type="reset"
        className="bg-transparent text-color-white hover:bg-transparent border-border-button border"
        onClick={handleReset}
      >
        {t('knowledgeConfiguration.cancel')}
      </Button>
      <DynamicForm.SavingButton
        submitLoading={loading}
        submitFunc={handleSavingSubmit}
      ></DynamicForm.SavingButton>
    </>
  ) : undefined;

  return (
    <MemorySettingProvider data={data}>
      <ModelResourceTenantIdContext.Provider value={data?.tenant_id}>
        <ModelResourceAccessUserIdContext.Provider
          value={memoryConfigLoading ? null : (data?.user_id ?? null)}
        >
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(() => {})}
              className="flex min-h-0 w-full flex-1 flex-col"
            >
              <SettingsFormLayout
                header={
                  <TopTitle
                    title={t('knowledgeDetails.configuration')}
                    description={t('memory.config.titleDescription')}
                  ></TopTitle>
                }
                actions={memoryActions}
              >
                <section className="space-y-5 text-text-secondary !space-y-10">
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.baseInfo')}
                  </div>
                  <BasicInfo readOnly={!canEditConfig}></BasicInfo>
                  <Divider />
                  <MemoryModelForm readOnly={!canEditConfig} />
                  <AdvancedSettingsForm readOnly={!canEditConfig} />
                </section>
              </SettingsFormLayout>
            </form>
          </Form>
        </ModelResourceAccessUserIdContext.Provider>
      </ModelResourceTenantIdContext.Provider>
    </MemorySettingProvider>
  );
}
