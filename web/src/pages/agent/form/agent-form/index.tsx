import { Collapse } from '@/components/collapse';
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
import { Input, NumberInput } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  AgentExceptionMethod,
  VariableType,
  initialAgentValues,
} from '../../constant';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { isBottomSubAgent } from '../../utils';
import { DescriptionField } from '../components/description-field';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { QueryVariable } from '../components/query-variable';
import { AgentTools, Agents } from './agent-tools';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const exceptionMethodOptions = buildOptions(AgentExceptionMethod);

const FormSchema = z.object({
  sys_prompt: z.string(),
  description: z.string().optional(),
  user_prompt: z.string().optional(),
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
  max_retries: z.coerce.number(),
  delay_after_error: z.coerce.number().optional(),
  visual_files_var: z.string().optional(),
  max_rounds: z.coerce.number().optional(),
  exception_method: z.string().nullable(),
  exception_comment: z.string().optional(),
  exception_goto: z.string().optional(),
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

  const form = useForm<z.infer<typeof FormSchema>>({
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
          {isSubAgent && (
            <>
              <DescriptionField></DescriptionField>
              <FormField
                control={form.control}
                name={`user_prompt`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormLabel>Subagent Input</FormLabel>
                    <FormControl>
                      <Textarea {...field}></Textarea>
                    </FormControl>
                  </FormItem>
                )}
              />
            </>
          )}
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
        <Collapse title={<div>Advanced Settings</div>}>
          <FormContainer>
            <QueryVariable
              name="visual_files_var"
              label="Visual files var"
            ></QueryVariable>
            <FormField
              control={form.control}
              name={`max_retries`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>Max retries</FormLabel>
                  <FormControl>
                    <NumberInput {...field} max={8}></NumberInput>
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`delay_after_error`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>Delay after error</FormLabel>
                  <FormControl>
                    <NumberInput {...field} max={5} step={0.1}></NumberInput>
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`max_rounds`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>Max rounds</FormLabel>
                  <FormControl>
                    <NumberInput {...field}></NumberInput>
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`exception_method`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>Exception method</FormLabel>
                  <FormControl>
                    <RAGFlowSelect
                      {...field}
                      options={exceptionMethodOptions}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`exception_comment`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>Exception comment</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                </FormItem>
              )}
            />
            <QueryVariable
              name="exception_goto"
              label="Exception goto"
              type={VariableType.File}
            ></QueryVariable>
          </FormContainer>
        </Collapse>
        <Output list={outputList}></Output>
      </form>
    </Form>
  );
};

export default AgentForm;
