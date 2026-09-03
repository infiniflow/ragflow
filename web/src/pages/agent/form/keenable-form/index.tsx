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
import { RAGFlowSelect } from '@/components/ui/select';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { KeenableMode, initialKeenableValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const KeenableFormPartialSchema = {
  api_key: z.string().optional(),
  mode: z.string(),
  site: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string(),
  ...KeenableFormPartialSchema,
});

export function KeenableWidgets() {
  const { t } = useTranslate('flow');
  const form = useFormContext();

  const modeOptions = useMemo(
    () =>
      Object.values(KeenableMode).map((x) => ({
        value: x,
        label: x.charAt(0).toUpperCase() + x.slice(1),
      })),
    [],
  );

  return (
    <>
      <ApiKeyField placeholder={t('keenableApiKeyTip')}></ApiKeyField>
      <FormField
        control={form.control}
        name={'mode'}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('keenableMode')}</FormLabel>
            <FormControl>
              <RAGFlowSelect {...field} options={modeOptions} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'site'}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('keenableSite')}</FormLabel>
            <FormControl>
              <Input {...field} placeholder="example.com"></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <TopNFormField></TopNFormField>
    </>
  );
}

const outputList = buildOutputList(initialKeenableValues.outputs);

function KeenableForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialKeenableValues, node);

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
          <KeenableWidgets></KeenableWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(KeenableForm);
