import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { RAGFlowSelect } from '@/components/ui/select';
import { LLMFactory } from '@/constants/llm';
import { useFetchFactoryModels } from '@/hooks/use-llm-request';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { LLMHeader } from '../../components/llm-header';

const baseFormSchema = z.object({
  model_type: z.string().min(1, 'Model type is required'),
  provider: z.string().optional(),
  llm_name: z.string().min(1, 'Model is required'),
  api_base: z.string().optional(),
  api_key: z.string().optional(),
  max_tokens: z.number().optional(),
});

type FormValues = z.infer<typeof baseFormSchema>;

const DynamicProviderModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
  editMode = false,
  initialValues,
}: IModalProps<Partial<IAddLlmRequestBody>> & {
  llmFactory: string;
  editMode?: boolean;
}) => {
  const { t } = useTranslation();
  const [selectedModelType, setSelectedModelType] = useState('chat');
  const [selectedProvider, setSelectedProvider] = useState<string | null>(null);

  const formSchema = useMemo(() => {
    if (llmFactory === LLMFactory.OpenRouter) {
      return baseFormSchema.extend({
        api_key: z.string().min(1, 'API key is required'),
      });
    }
    return baseFormSchema;
  }, [llmFactory]);

  // Keep a ref to the latest schema so validation always uses the current one
  const formSchemaRef = useRef(formSchema);
  formSchemaRef.current = formSchema;

  const form = useForm<FormValues>({
    resolver: (data, context, options) =>
      zodResolver(formSchemaRef.current)(data, context, options),
    defaultValues: {
      model_type: 'chat',
      provider: 'all',
      llm_name: '',
      api_base: '',
      api_key: '',
      max_tokens: 8192,
    },
  });

  // Reset form when schema or factory changes to ensure validation rules are updated
  useEffect(() => {
    form.reset(form.getValues());
  }, [formSchema, llmFactory, form]);

  const {
    dataByCategory,
    supportedCategories,
    defaultBaseUrl,
    loading: modelsLoading,
  } = useFetchFactoryModels(llmFactory, selectedModelType, visible || false);

  // 1. Initialize form with initialValues OR default values (RUNS ONCE on open)
  useEffect(() => {
    if (!visible) return;

    if (editMode && initialValues) {
      form.reset({
        model_type: initialValues.model_type || 'chat',
        provider: (initialValues as any).provider || 'all',
        llm_name: initialValues.llm_name || '',
        api_base: initialValues.api_base || '',
        api_key: '',
        max_tokens: initialValues.max_tokens || 8192,
      });
      setSelectedModelType(initialValues.model_type || 'chat');
      setSelectedProvider((initialValues as any).provider || null);
    } else {
      // Not edit mode - strict reset
      form.reset({
        model_type: 'chat',
        provider: 'all',
        llm_name: '',
        api_base: '', // Start empty, let the second effect handle defaultBaseUrl
        api_key: '',
        max_tokens: 8192,
      });
      setSelectedModelType('chat');
      setSelectedProvider(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, editMode, initialValues, form]);

  // 2. Update api_base with defaultBaseUrl ONLY if field is empty (separate effect)
  useEffect(() => {
    if (visible && !editMode && defaultBaseUrl) {
      const currentApiBase = form.getValues('api_base');
      if (!currentApiBase) {
        form.setValue('api_base', defaultBaseUrl);
      }
    }
  }, [defaultBaseUrl, visible, editMode, form]);

  // Cascade Step 1: dynamicModels based on selectedModelType
  const dynamicModels = useMemo(() => {
    return dataByCategory[selectedModelType] || [];
  }, [dataByCategory, selectedModelType]);

  // Cascade Step 2: availableProviders from dynamicModels
  const availableProviders = useMemo(() => {
    const providers = Array.from(
      new Set(dynamicModels.map((m) => m.provider || 'unknown')),
    ).sort();
    return providers;
  }, [dynamicModels]);

  // Cascade Step 3: filteredModels based on selectedProvider
  const filteredModels = useMemo(() => {
    if (!selectedProvider) return dynamicModels;
    return dynamicModels.filter(
      (m) => (m.provider || 'unknown') === selectedProvider,
    );
  }, [dynamicModels, selectedProvider]);

  // Auto-populate max_tokens when model is selected
  const watchLlmName = form.watch('llm_name');
  useEffect(() => {
    const model = dynamicModels.find((m) => m.llm_name === watchLlmName);
    if (model && model.max_tokens) {
      form.setValue('max_tokens', model.max_tokens);
    }
  }, [watchLlmName, dynamicModels, form]);

  const handleSubmit = async (values: FormValues) => {
    const selectedModel = dynamicModels.find(
      (m) => m.llm_name === values.llm_name,
    );

    await onOk?.({
      llm_factory: llmFactory,
      llm_name: values.llm_name,
      model_type: values.model_type,
      api_base: values.api_base || defaultBaseUrl || '',
      api_key: values.api_key,
      max_tokens: values.max_tokens || selectedModel?.max_tokens || 8192,
    } as IAddLlmRequestBody);
  };

  const modelTypeOptions = useMemo(() => {
    return supportedCategories.map((cat) => ({
      label: cat,
      value: cat,
    }));
  }, [supportedCategories]);

  const providerOptions = useMemo(() => {
    const options = availableProviders.map((p) => ({
      label:
        p === 'unknown' ? 'Unknown' : p.charAt(0).toUpperCase() + p.slice(1),
      value: p,
    }));
    return [{ label: 'All Providers', value: 'all' }, ...options];
  }, [availableProviders]);

  const modelOptions = useMemo(() => {
    return filteredModels.map((m) => ({
      label: m.name,
      value: m.llm_name,
    }));
  }, [filteredModels]);

  return (
    <Modal
      title={<LLMHeader name={llmFactory} />}
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
          <FormField
            control={form.control}
            name="model_type"
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{t('setting.modelType')}</FormLabel>
                <RAGFlowSelect
                  {...field}
                  options={modelTypeOptions}
                  onValueChange={(val) => {
                    field.onChange(val);
                    setSelectedModelType(val);
                    setSelectedProvider(null);
                    form.setValue('provider', 'all');
                    form.setValue('llm_name', '');
                  }}
                  placeholder="Select model type"
                />
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="provider"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('setting.provider')}</FormLabel>
                <RAGFlowSelect
                  {...field}
                  options={providerOptions}
                  onValueChange={(val) => {
                    field.onChange(val);
                    setSelectedProvider(val === 'all' ? null : val);
                    form.setValue('llm_name', '');
                  }}
                  placeholder="Select provider"
                />
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="llm_name"
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{t('setting.model')}</FormLabel>
                <RAGFlowSelect
                  {...field}
                  options={modelOptions}
                  disabled={modelsLoading || filteredModels.length === 0}
                  placeholder={
                    modelsLoading ? 'Loading models...' : 'Select a model'
                  }
                />
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="api_base"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('setting.addLlmBaseUrl')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder={defaultBaseUrl || ''} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="api_key"
            render={({ field }) => (
              <FormItem>
                <FormLabel required={llmFactory === LLMFactory.OpenRouter}>
                  {t('setting.apiKey')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type="password"
                    placeholder={
                      editMode
                        ? 'Leave empty to keep existing key'
                        : 'Enter API key'
                    }
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className="flex justify-end gap-2 pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => hideModal?.()}
            >
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={loading}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('common.save')}
            </Button>
          </div>
        </form>
      </Form>
    </Modal>
  );
};

export default DynamicProviderModal;
