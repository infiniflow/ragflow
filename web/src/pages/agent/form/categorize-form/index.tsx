import { FormContainer } from '@/components/form-container';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { INextOperatorForm } from '../../interface';
import { QueryVariable } from '../components/query-variable';
import DynamicCategorize from './dynamic-categorize';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const CategorizeForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();

  const values = useValues(node);

  const FormSchema = z.object({
    query: z.string().optional(),
    parameter: z.string().optional(),
    ...LlmSettingSchema,
    message_history_window_size: z.coerce.number(),
    items: z.array(
      z
        .object({
          name: z.string().min(1, t('flow.nameMessage')).trim(),
          description: z.string().optional(),
          examples: z
            .array(
              z.object({
                value: z.string(),
              }),
            )
            .optional(),
        })
        .optional(),
    ),
  });

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-5 "
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <QueryVariable></QueryVariable>
          <LargeModelFormField></LargeModelFormField>
        </FormContainer>
        <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
        <DynamicCategorize nodeId={node?.id}></DynamicCategorize>
      </form>
    </Form>
  );
};

export default CategorizeForm;
