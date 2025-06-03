import { FormContainer } from '@/components/form-container';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { PromptEditor } from '@/components/prompt-editor';
import { Form, FormControl, FormField, FormItem } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialAgentValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { INextOperatorForm } from '../../interface';

const FormSchema = z.object({
  sys_prompt: z.string(),
  prompts: z
    .array(
      z.object({
        role: z.string(),
        content: z.string(),
      }),
    )
    .optional(),
  message_history_window_size: z.coerce.number(),
  ...LlmSettingSchema,
  tools: z
    .array(
      z.object({
        component_name: z.string(),
      }),
    )
    .optional(),
});

const AgentForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();
  const defaultValues = useFormValues(initialAgentValues, node);

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-4"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <LargeModelFormField></LargeModelFormField>
          <FormField
            control={form.control}
            name={`sys_prompt`}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormControl>
                  <PromptEditor
                    {...field}
                    placeholder={t('flow.messagePlaceholder')}
                  ></PromptEditor>
                </FormControl>
              </FormItem>
            )}
          />
        </FormContainer>
      </form>
    </Form>
  );
};

export default AgentForm;
