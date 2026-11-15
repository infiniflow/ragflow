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
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useEffect, useMemo, useState } from 'react';
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

/** Parses a crawl timeout only when it is an integer in the supported range. */
const parseCrawlTimeout = (value: unknown) => {
  const numberValue =
    typeof value === 'number'
      ? value
      : typeof value === 'string' && value.trim() !== ''
        ? Number(value)
        : Number.NaN;

  return Number.isInteger(numberValue) && numberValue >= 1 && numberValue <= 60
    ? numberValue
    : undefined;
};

/** Replaces a missing or invalid saved timeout with the tool's default. */
const normalizeCrawlTimeout = (value: unknown) =>
  parseCrawlTimeout(value) ?? initialQueritContentsValues.crawl_timeout;

/** Renders the Querit Contents form and keeps its timeout valid while editing. */
function QueritContentsForm({ node }: INextOperatorForm) {
  const { t } = useTranslate('flow');
  const values = useFormValues(initialQueritContentsValues, node);
  const defaultValues = useMemo(
    () => ({
      ...values,
      crawl_timeout: normalizeCrawlTimeout(values.crawl_timeout),
    }),
    [values],
  );
  const [crawlTimeoutDraft, setCrawlTimeoutDraft] = useState(() =>
    String(defaultValues.crawl_timeout),
  );
  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useEffect(() => {
    setCrawlTimeoutDraft(String(normalizeCrawlTimeout(values.crawl_timeout)));
  }, [node?.id, values.crawl_timeout]);

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
                  <SelectWithSearch
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
            render={({ field }) => {
              const handleTimeoutChange = (
                event: React.ChangeEvent<HTMLInputElement>,
              ) => {
                const value = String(event.target.value);
                setCrawlTimeoutDraft(value);

                const timeout = parseCrawlTimeout(value);
                if (timeout !== undefined) {
                  field.onChange(timeout);
                }
              };
              const handleTimeoutBlur = () => {
                field.onBlur();
                setCrawlTimeoutDraft(
                  String(normalizeCrawlTimeout(field.value)),
                );
              };

              return (
                <FormItem>
                  <FormLabel tooltip={t('queritContentsTimeoutTip')}>
                    {t('queritContentsTimeout')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      min={1}
                      max={60}
                      step={1}
                      {...field}
                      value={crawlTimeoutDraft}
                      onChange={handleTimeoutChange}
                      onBlur={handleTimeoutBlur}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              );
            }}
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
