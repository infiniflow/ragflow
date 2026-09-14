import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import {
  QueritContentFormat,
  initialQueritContentsValues,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';

const outputList = buildOutputList(initialQueritContentsValues.outputs);

const FormSchema = z.object({
  api_key: z.string(),
  urls: z.string().min(1),
  format: z.enum([
    QueritContentFormat.Text,
    QueritContentFormat.Markdown,
    QueritContentFormat.Html,
  ]),
  crawl_timeout: z.coerce.number().int().min(1).max(60),
  extras_meta: z.boolean(),
});

function QueritContentsForm({ node }: INextOperatorForm) {
  const { t } = useTranslate('flow');
  const values = useFormValues(initialQueritContentsValues, node);
  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <ApiKeyField />
          <FormField
            control={form.control}
            name="urls"
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={t('queritContentsUrlsTip')}>
                  {t('queritContentsUrls')}
                </FormLabel>
                <FormControl>
                  <PromptEditor
                    {...field}
                    multiLine={false}
                    showToolbar={false}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="format"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('format')}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={buildOptions(QueritContentFormat)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="crawl_timeout"
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={t('queritContentsTimeoutTip')}>
                  {t('queritContentsTimeout')}
                </FormLabel>
                <FormControl>
                  <Input type="number" min={1} max={60} step={1} {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="extras_meta"
            render={({ field }) => (
              <FormItem className="flex items-center justify-between">
                <FormLabel tooltip={t('queritContentsMetadataTip')}>
                  {t('queritContentsMetadata')}
                </FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList} />
      </div>
    </Form>
  );
}

export default memo(QueritContentsForm);
