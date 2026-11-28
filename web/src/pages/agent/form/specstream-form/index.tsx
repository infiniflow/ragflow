import { FormContainer } from '@/components/form-container';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
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

export function useSpecStreamFormPartialSchema() {
  const { t } = useTranslate('flow');

  return useMemo(
    () => ({
      api_key: z.string().min(1, t('apiKeyRequired')),
      passages: z.boolean().optional(),
      top_n: z.coerce.number(),
    }),
    [t],
  );
}

export function SpecStreamWidgets() {
  const { t } = useTranslate('flow');

  return (
    <>
      <ApiKeyField
        placeholder={t('specStreamApiKeyTip')}
        required
      ></ApiKeyField>
      <RAGFlowFormItem
        name="passages"
        label={t('specStreamPassages')}
        tooltip={t('specStreamPassagesTip')}
        horizontal={true}
        labelClassName="w-full"
        valueClassName="w-8"
      >
        {(field) => (
          <Switch checked={field.value} onCheckedChange={field.onChange} />
        )}
      </RAGFlowFormItem>
      <TopNFormField max={20}></TopNFormField>
    </>
  );
}

const SpecStreamOutputList = buildOutputList(
  initialSpecStreamSearchValues.outputs,
);

function SpecStreamForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialSpecStreamSearchValues, node);
  const partialSchema = useSpecStreamFormPartialSchema();
  const FormSchema = useMemo(
    () => z.object({ query: z.string(), ...partialSchema }),
    [partialSchema],
  );

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
