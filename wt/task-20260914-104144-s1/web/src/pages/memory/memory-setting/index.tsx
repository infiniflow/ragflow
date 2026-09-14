import { DynamicForm } from '@/components/dynamic-form';
import { Button } from '@/components/ui/button';
import Divider from '@/components/ui/divider';
import { Form } from '@/components/ui/form';
import { MainContainer } from '@/pages/dataset/setting/python/configuration-form-container';
import { TopTitle } from '@/pages/dataset/dataset-title';
import { IMemory } from '@/pages/memories/interface';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
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

export default function MemoryMessage() {
  const { t } = useTranslation();
  const { data } = useFetchMemoryBaseConfiguration();
  const { formSchema, modelsFetched, modelsEditable } = useMemoryFormSchema(
    data?.tenant_id,
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
      permissions: data?.permissions || 'me',
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
  return (
    <section className="h-full flex flex-col">
      <TopTitle
        title={t('knowledgeDetails.configuration')}
        description={t('memory.config.titleDescription')}
      ></TopTitle>
      <div className="flex gap-14 flex-1 min-h-0">
        <MemorySettingProvider data={data}>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(() => {})} className="space-y-6 ">
              <div className="w-[768px] h-[calc(100vh-300px)] pr-1 overflow-y-auto scrollbar-auto pb-4">
                <MainContainer className="text-text-secondary !space-y-10">
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.baseInfo')}
                  </div>
                  <BasicInfo></BasicInfo>
                  <Divider />
                  <MemoryModelForm />
                  <AdvancedSettingsForm />
                </MainContainer>
              </div>
              <div className="text-right items-center flex justify-end gap-3 w-[768px]">
                <Button
                  type="reset"
                  className="bg-transparent text-color-white hover:bg-transparent border-border-button border"
                  onClick={() => {
                    form.reset();
                  }}
                >
                  {t('knowledgeConfiguration.cancel')}
                </Button>
                <DynamicForm.SavingButton
                  submitLoading={loading}
                  submitFunc={(value) => {
                    onSubmit(value as IMemory);
                  }}
                ></DynamicForm.SavingButton>
              </div>
            </form>
          </Form>
        </MemorySettingProvider>
      </div>
    </section>
  );
}
