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
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialQueritValues } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import { DynamicList } from './dynamic-list';
import { QueritFormSchema } from './utils';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const outputList = buildOutputList(initialQueritValues.outputs);

export function QueritFormWidgets({ includeQuery = true }) {
  const form = useFormContext();
  const { t } = useTranslate('flow');
  const listPlaceholder = t('queritListPlaceholder');

  return (
    <>
      <ApiKeyField />
      {includeQuery && <QueryVariable />}
      <FormField
        control={form.control}
        name="count"
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('queritCountTip')}>
              {t('queritCount')}
            </FormLabel>
            <FormControl>
              <Input type="number" min={1} step={1} {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="chunks_per_doc"
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('queritChunksPerDocTip')}>
              {t('queritChunksPerDoc')}
            </FormLabel>
            <FormControl>
              <Input type="number" min={1} max={3} step={1} {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <DynamicList
        name="site_include"
        label={t('queritSiteInclude')}
        tooltip={t('queritSiteIncludeTip')}
        placeholder={listPlaceholder}
      />
      <DynamicList
        name="site_exclude"
        label={t('queritSiteExclude')}
        tooltip={t('queritSiteExcludeTip')}
        placeholder={listPlaceholder}
      />
      <FormField
        control={form.control}
        name="time_range"
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('queritTimeRangeTip')}>
              {t('queritTimeRange')}
            </FormLabel>
            <FormControl>
              <Input placeholder="d7" {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <DynamicList
        name="country_include"
        label={t('queritCountryInclude')}
        tooltip={t('queritCountryIncludeTip')}
        placeholder={listPlaceholder}
      />
      <DynamicList
        name="language_include"
        label={t('queritLanguageInclude')}
        tooltip={t('queritLanguageIncludeTip')}
        placeholder={listPlaceholder}
      />
    </>
  );
}

function QueritForm({ node }: INextOperatorForm) {
  const values = useValues(node);
  const form = useForm<z.infer<typeof QueritFormSchema>>({
    defaultValues: values,
    resolver: zodResolver(QueritFormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueritFormWidgets />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList} />
      </div>
    </Form>
  );
}

export default memo(QueritForm);
