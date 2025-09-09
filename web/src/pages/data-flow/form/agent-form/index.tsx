import { Collapse } from '@/components/collapse';
import { FormContainer } from '@/components/form-container';
import {
  LargeModelFilterFormSchema,
  LargeModelFormField,
} from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { Input, NumberInput } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { LlmModelType } from '@/constants/knowledge';
import { useFindLlmByUuid } from '@/hooks/use-llm-request';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useEffect, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  AgentExceptionMethod,
  NodeHandleId,
  VariableType,
  initialAgentValues,
} from '../../constant';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { isBottomSubAgent } from '../../utils';
import { buildOutputList } from '../../utils/build-output-list';
import { DescriptionField } from '../components/description-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { QueryVariable } from '../components/query-variable';
import { AgentTools, Agents } from './agent-tools';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

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
  exception_method: z.string().optional(),
  exception_goto: z.array(z.string()).optional(),
  exception_default_value: z.string().optional(),
  ...LargeModelFilterFormSchema,
  cite: z.boolean().optional(),
});

const outputList = buildOutputList(initialAgentValues.outputs);

function AgentForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const { edges, deleteEdgesBySourceAndSourceHandle } = useGraphStore(
    (state) => state,
  );

  const defaultValues = useValues(node);

  const ExceptionMethodOptions = Object.values(AgentExceptionMethod).map(
    (x) => ({
      label: t(`flow.${x}`),
      value: x,
    }),
  );

  const isSubAgent = useMemo(() => {
    return isBottomSubAgent(edges, node?.id);
  }, [edges, node?.id]);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  const llmId = useWatch({ control: form.control, name: 'llm_id' });

  const findLlmByUuid = useFindLlmByUuid();

  const exceptionMethod = useWatch({
    control: form.control,
    name: 'exception_method',
  });

  useEffect(() => {
    if (exceptionMethod !== AgentExceptionMethod.Goto) {
      if (node?.id) {
        deleteEdgesBySourceAndSourceHandle(
          node?.id,
          NodeHandleId.AgentException,
        );
      }
    }
  }, [deleteEdgesBySourceAndSourceHandle, exceptionMethod, node?.id]);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          {isSubAgent && <DescriptionField></DescriptionField>}
          <LargeModelFormField showSpeech2TextModel></LargeModelFormField>
          {findLlmByUuid(llmId)?.model_type === LlmModelType.Image2text && (
            <QueryVariable
              name="visual_files_var"
              label="Visual Input File"
              type={VariableType.File}
            ></QueryVariable>
          )}
        </FormContainer>

        <FormContainer>
          <FormField
            control={form.control}
            name={`sys_prompt`}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{t('flow.systemPrompt')}</FormLabel>
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
        </FormContainer>
        {isSubAgent || (
          <FormContainer>
            {/* <DynamicPrompt></DynamicPrompt> */}
            <FormField
              control={form.control}
              name={`prompts`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>{t('flow.userPrompt')}</FormLabel>
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
        <Collapse title={<div>{t('flow.advancedSettings')}</div>}>
          <FormContainer>
            <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
            <FormField
              control={form.control}
              name={`cite`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel tooltip={t('flow.citeTip')}>
                    {t('flow.cite')}
                  </FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    ></Switch>
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`max_retries`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>{t('flow.maxRetries')}</FormLabel>
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
                  <FormLabel>{t('flow.delayEfterError')}</FormLabel>
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
                  <FormLabel>{t('flow.maxRounds')}</FormLabel>
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
                  <FormLabel>{t('flow.exceptionMethod')}</FormLabel>
                  <FormControl>
                    <SelectWithSearch
                      {...field}
                      options={ExceptionMethodOptions}
                      allowClear
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            {exceptionMethod === AgentExceptionMethod.Comment && (
              <FormField
                control={form.control}
                name={`exception_default_value`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormLabel>{t('flow.ExceptionDefaultValue')}</FormLabel>
                    <FormControl>
                      <Input {...field} />
                    </FormControl>
                  </FormItem>
                )}
              />
            )}
          </FormContainer>
        </Collapse>
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(AgentForm);
