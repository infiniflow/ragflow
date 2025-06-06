import { FormContainer } from '@/components/form-container';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { useFetchModelId } from '@/hooks/logic-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialAgentValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import DynamicPrompt from './dynamic-prompt';

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
  tools: z
    .array(
      z.object({
        component_name: z.string(),
      }),
    )
    .optional(),
  ...LlmSettingSchema,
});

const AgentForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();
  const llmId = useFetchModelId();
  const defaultValues = useFormValues(
    { ...initialAgentValues, llm_id: llmId },
    node,
  );

  const outputList = useMemo(() => {
    return [
      { title: 'content', type: initialAgentValues.outputs.content.type },
    ];
  }, []);

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

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
                <FormLabel>Prompt</FormLabel>
                <FormControl>
                  <PromptEditor
                    {...field}
                    placeholder={t('flow.messagePlaceholder')}
                    showToolbar={false}
                  ></PromptEditor>
                </FormControl>
              </FormItem>
            )}
          />
          <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
        </FormContainer>
        <FormContainer>
          <DynamicPrompt></DynamicPrompt>
        </FormContainer>
        <Output list={outputList}></Output>
      </form>
    </Form>
  );
};

export default AgentForm;
