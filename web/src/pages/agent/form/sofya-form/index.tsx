import { FormContainer } from '@/components/form-container';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { useTranslate } from '@/hooks/common-hooks';
import i18n from '@/locales/config';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
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
  api_key: z.string().min(1, i18n.t('flow.apiKeyRequired')),
  search_depth: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...SofyaFormPartialSchema,
});

export function SofyaWidgets() {
  const { t } = useTranslate('flow');

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
      <ApiKeyField placeholder={t('sofyaApiKeyTip')} required></ApiKeyField>
      <RAGFlowFormItem
        name="search_depth"
        label={t('sofyaSearchDepth')}
        tooltip={t('sofyaSearchDepthTip')}
      >
        <SelectWithSearch options={searchDepthOptions} />
      </RAGFlowFormItem>
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
    mode: 'onChange',
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
