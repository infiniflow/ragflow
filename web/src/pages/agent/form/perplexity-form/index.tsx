import { FormContainer } from '@/components/form-container';
import { TopNFormField } from '@/components/top-n-item';
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
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import {
  PerplexityRecency,
  PerplexitySearchType,
  initialPerplexityValues,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

const PerplexitySearchTypeLabelKeys: Record<PerplexitySearchType, string> = {
  [PerplexitySearchType.Web]: 'perplexitySearchTypeWeb',
  [PerplexitySearchType.Fast]: 'perplexitySearchTypeFast',
};

const PerplexityRecencyLabelKeys: Record<PerplexityRecency, string> = {
  [PerplexityRecency.Any]: 'perplexityRecencyAny',
  [PerplexityRecency.Hour]: 'perplexityRecencyHour',
  [PerplexityRecency.Day]: 'perplexityRecencyDay',
  [PerplexityRecency.Week]: 'perplexityRecencyWeek',
  [PerplexityRecency.Month]: 'perplexityRecencyMonth',
  [PerplexityRecency.Year]: 'perplexityRecencyYear',
};

export const PerplexityFormPartialSchema = {
  api_key: z.string().optional(),
  search_type: z.string().optional(),
  search_recency_filter: z.string().optional(),
  search_domain_filter: z.string().optional(),
  country: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...PerplexityFormPartialSchema,
});

export function PerplexityWidgets() {
  const { t } = useTranslate('flow');
  const form = useFormContext();

  const searchTypeOptions = useMemo(
    () =>
      Object.values(PerplexitySearchType).map((x) => ({
        value: x,
        label: t(PerplexitySearchTypeLabelKeys[x]),
      })),
    [t],
  );

  const recencyOptions = useMemo(
    () =>
      Object.values(PerplexityRecency).map((x) => ({
        value: x,
        label: t(PerplexityRecencyLabelKeys[x]),
      })),
    [t],
  );

  return (
    <>
      <ApiKeyField
        placeholder={t('perplexityApiKeyTip')}
        required
      ></ApiKeyField>
      <FormField
        control={form.control}
        name={'search_type'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('perplexitySearchTypeTip')}>
              {t('perplexitySearchType')}
            </FormLabel>
            <FormControl>
              <SelectWithSearch {...field} options={searchTypeOptions} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'search_recency_filter'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('perplexityRecencyTip')}>
              {t('perplexityRecency')}
            </FormLabel>
            <FormControl>
              <SelectWithSearch {...field} options={recencyOptions} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'search_domain_filter'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('perplexityDomainFilterTip')}>
              {t('perplexityDomainFilter')}
            </FormLabel>
            <FormControl>
              <Input
                {...field}
                placeholder="ragflow.io, docs.example.com"
              ></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'country'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('perplexityCountryTip')}>
              {t('perplexityCountry')}
            </FormLabel>
            <FormControl>
              <Input {...field} placeholder="US"></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <TopNFormField max={20}></TopNFormField>
    </>
  );
}

const PerplexityOutputList = buildOutputList(initialPerplexityValues.outputs);

function PerplexityForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialPerplexityValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable></QueryVariable>
          <PerplexityWidgets></PerplexityWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={PerplexityOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(PerplexityForm);
