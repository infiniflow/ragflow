import { FormContainer } from '@/components/form-container';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { BlockButton } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Position } from '@xyflow/react';
import { useContext, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { Operator, initialAgentValues } from '../../constant';
import { AgentInstanceContext } from '../../context';
import { INextOperatorForm } from '../../interface';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { ToolPopover } from './tool-popover';
import { useToolOptions, useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const FormSchema = z.object({
  sys_prompt: z.string(),
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

  const defaultValues = useValues(node);

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

  const { addCanvasNode } = useContext(AgentInstanceContext);

  const toolOptions = useToolOptions();

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
          {/* <DynamicPrompt></DynamicPrompt> */}
          <FormField
            control={form.control}
            name={`prompts`}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormControl>
                  <section>
                    <PromptEditor {...field} showToolbar={false}></PromptEditor>
                  </section>
                </FormControl>
              </FormItem>
            )}
          />
        </FormContainer>
        <ToolPopover>
          <BlockButton>Add Tool</BlockButton>
        </ToolPopover>
        <BlockButton
          onClick={addCanvasNode(Operator.Agent, {
            nodeId: node?.id,
            position: Position.Bottom,
          })}
        >
          Add Agent
        </BlockButton>
        <Output list={outputList}></Output>
      </form>
    </Form>
  );
};

export default AgentForm;
