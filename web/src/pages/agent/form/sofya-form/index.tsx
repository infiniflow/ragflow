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
import { SofyaSearchDepth, initialSofyaValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

// Search depth is a user-facing word, so each option carries its own key rather
// than deriving an English label from the enum value.
const SofyaSearchDepthLabelKeys: Record<SofyaSearchDepth, string> = {
  [SofyaSearchDepth.Basic]: 'sofyaSearchDepthBasic',
  [SofyaSearchDepth.Snippets]: 'sofyaSearchDepthSnippets',
};

export const SofyaFormPartialSchema = {
  api_key: z.string().optional(),
  search_depth: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...SofyaFormPartialSchema,
});

export function SofyaWidgets() {
  const { t } = useTranslate('flow');
  const form = useFormContext();

  const searchDepthOptions = useMemo(
    () =>
      Object.values(SofyaSearchDepth).map((x) => ({
        value: x,
        label: t(SofyaSearchDepthLabelKeys[x]),
      })),
    [t],
  );

  return (
    <>
      <ApiKeyField placeholder={t('sofyaApiKeyTip')}></ApiKeyField>
      <FormField
        control={form.control}
        name={'search_depth'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('sofyaSearchDepthTip')}>
              {t('sofyaSearchDepth')}
            </FormLabel>
            <FormControl>
              <RAGFlowSelect {...field} options={searchDepthOptions} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <TopNFormField></TopNFormField>
    </>
  );
}

const SofyaOutputList = buildOutputList(initialSofyaValues.outputs);

function SofyaForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialSofyaValues, node);

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
          <SofyaWidgets></SofyaWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={SofyaOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(SofyaForm);
