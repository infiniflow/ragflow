import { FormContainer } from '@/components/form-container';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { useTranslate } from '@/hooks/common-hooks';
import i18n from '@/locales/config';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialContextFetchValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const ContextFetchFormPartialSchema = {
  api_key: z.string().min(1, i18n.t('flow.apiKeyRequired')),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...ContextFetchFormPartialSchema,
});

export function ContextFetchWidgets() {
  const { t } = useTranslate('flow');

  return (
    <>
      <ApiKeyField
        placeholder={t('contextFetchApiKeyTip')}
        required
      ></ApiKeyField>
      <TopNFormField></TopNFormField>
    </>
  );
}

const ContextFetchOutputList = buildOutputList(
  initialContextFetchValues.outputs,
);

function ContextFetchForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialContextFetchValues, node);

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
          <ContextFetchWidgets></ContextFetchWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={ContextFetchOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(ContextFetchForm);
