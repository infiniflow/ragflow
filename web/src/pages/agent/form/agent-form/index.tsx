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
import { zodResolver } from '@hookform/resolvers/zod';
import { useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialAgentValues } from '../../constant';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { isBottomSubAgent } from '../../utils';
import { DescriptionField } from '../components/description-field';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { AgentTools, Agents } from './agent-tools';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const FormSchema = z.object({
  sys_prompt: z.string(),
  description: z.string().optional(),
  prompts: z.string().optional(),
  // prompts: z
  //   .array(
  //     z.object({
  //       role: z.string(),
  //       content: z.string(),
  //     }),
  //   )
  //   .optional(),
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
  const { edges } = useGraphStore((state) => state);

  const defaultValues = useValues(node);

  const isSubAgent = useMemo(() => {
    return isBottomSubAgent(edges, node?.id);
  }, [edges, node?.id]);

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
          {isSubAgent && <DescriptionField></DescriptionField>}
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
        {isSubAgent || (
          <FormContainer>
            {/* <DynamicPrompt></DynamicPrompt> */}
            <FormField
              control={form.control}
              name={`prompts`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>User Prompt</FormLabel>
                  <FormControl>
                    <section>
                      <PromptEditor
                        {...field}
                        showToolbar={false}
                      ></PromptEditor>
                    </section>
                  </FormControl>
                </FormItem>
              )}
            />
          </FormContainer>
        )}
        <FormContainer>
          <AgentTools></AgentTools>
          <Agents node={node}></Agents>
        </FormContainer>
        <Output list={outputList}></Output>
      </form>
    </Form>
  );
};

export default AgentForm;
