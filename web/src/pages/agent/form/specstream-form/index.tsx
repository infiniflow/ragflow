import { FormContainer } from '@/components/form-container';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialSpecStreamSearchValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const SpecStreamFormPartialSchema = {
  api_key: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...SpecStreamFormPartialSchema,
});

export function SpecStreamWidgets() {
  const { t } = useTranslate('flow');

  return (
    <>
      <ApiKeyField placeholder={t('specStreamApiKeyTip')}></ApiKeyField>
      <TopNFormField max={20}></TopNFormField>
    </>
  );
}

const SpecStreamOutputList = buildOutputList(
  initialSpecStreamSearchValues.outputs,
);

function SpecStreamForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialSpecStreamSearchValues, node);

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
          <SpecStreamWidgets></SpecStreamWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={SpecStreamOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(SpecStreamForm);
