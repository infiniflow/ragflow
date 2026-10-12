import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialAnySearchExtractValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';

export const AnySearchExtractFormPartialSchema = {
  api_key: z.string().optional(),
};

const FormSchema = z.object({
  ...AnySearchExtractFormPartialSchema,
  url: z.string().min(1),
});

export function AnySearchExtractWidgets() {
  const { t } = useTranslation();

  return <ApiKeyField placeholder={t('flow.anySearchApiKeyTip')}></ApiKeyField>;
}

const AnySearchExtractOutputList = buildOutputList(
  initialAnySearchExtractValues.outputs,
);

function AnySearchExtractForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const defaultValues = useFormValues(initialAnySearchExtractValues, node);
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
          <AnySearchExtractWidgets></AnySearchExtractWidgets>
          <FormField
            control={form.control}
            name="url"
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={t('flow.anySearchExtractUrlTip')}>
                  {t('flow.anySearchExtractUrl')}
                </FormLabel>
                <FormControl>
                  <PromptEditor
                    {...field}
                    multiLine={false}
                    showToolbar={false}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={AnySearchExtractOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(AnySearchExtractForm);
