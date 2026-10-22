import { FormContainer } from '@/components/form-container';
import { SelectWithSearch } from '@/components/originui/select-with-search';
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
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { AnySearchZone, initialAnySearchSearchValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

const AnySearchZoneLabelKeys: Record<AnySearchZone, string> = {
  [AnySearchZone.Intl]: 'flow.anySearchZoneIntl',
  [AnySearchZone.Cn]: 'flow.anySearchZoneCn',
};

export const AnySearchSearchFormPartialSchema = {
  api_key: z.string().optional(),
  top_n: z.coerce.number().int().min(1).max(10),
  zone: z.string().optional(),
  language: z.string().optional(),
};

const FormSchema = z.object({
  query: z.string(),
  ...AnySearchSearchFormPartialSchema,
});

export function AnySearchSearchWidgets() {
  const { t } = useTranslation();
  const form = useFormContext();

  const zoneOptions = useMemo(
    () =>
      Object.values(AnySearchZone).map((x) => ({
        value: x,
        label: t(AnySearchZoneLabelKeys[x]),
      })),
    [t],
  );

  return (
    <>
      <ApiKeyField placeholder={t('flow.anySearchApiKeyTip')}></ApiKeyField>
      <FormField
        control={form.control}
        name={'zone'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('flow.anySearchZoneTip')}>
              {t('flow.anySearchZone')}
            </FormLabel>
            <FormControl>
              <SelectWithSearch
                {...field}
                options={zoneOptions}
                placeholder={t('flow.anySearchZoneUnset')}
                allowClear
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'language'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('flow.anySearchLanguageTip')}>
              {t('flow.anySearchLanguage')}
            </FormLabel>
            <FormControl>
              <Input {...field} placeholder="en, zh-CN"></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <TopNFormField max={10}></TopNFormField>
    </>
  );
}

const AnySearchSearchOutputList = buildOutputList(
  initialAnySearchSearchValues.outputs,
);

function AnySearchSearchForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialAnySearchSearchValues, node);

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
          <AnySearchSearchWidgets></AnySearchSearchWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={AnySearchSearchOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(AnySearchSearchForm);
