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
import { RAGFlowSelect } from '@/components/ui/select';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { YouComFreshness, initialYouComValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

// Freshness is a user-facing word, so each option carries its own key rather
// than deriving an English label from the enum value.
const YouComFreshnessLabelKeys: Record<YouComFreshness, string> = {
  [YouComFreshness.Any]: 'youComFreshnessAny',
  [YouComFreshness.Day]: 'youComFreshnessDay',
  [YouComFreshness.Week]: 'youComFreshnessWeek',
  [YouComFreshness.Month]: 'youComFreshnessMonth',
  [YouComFreshness.Year]: 'youComFreshnessYear',
};

export const YouComFormPartialSchema = {
  api_key: z.string().optional(),
  freshness: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...YouComFormPartialSchema,
});

export function YouComWidgets() {
  const { t } = useTranslate('flow');
  const form = useFormContext();

  const freshnessOptions = useMemo(
    () =>
      Object.values(YouComFreshness).map((x) => ({
        value: x,
        label: t(YouComFreshnessLabelKeys[x]),
      })),
    [t],
  );

  return (
    <>
      <ApiKeyField placeholder={t('youComApiKeyTip')}></ApiKeyField>
      <FormField
        control={form.control}
        name={'freshness'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('youComFreshnessTip')}>
              {t('youComFreshness')}
            </FormLabel>
            <FormControl>
              <RAGFlowSelect {...field} options={freshnessOptions} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <TopNFormField></TopNFormField>
    </>
  );
}

const YouComOutputList = buildOutputList(initialYouComValues.outputs);

function YouComForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialYouComValues, node);

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
          <YouComWidgets></YouComWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={YouComOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(YouComForm);
