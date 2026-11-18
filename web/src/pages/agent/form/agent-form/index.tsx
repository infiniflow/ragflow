import { Collapse } from '@/components/collapse';
import {
  LargeModelFilterFormSchema,
  LargeModelFormField,
} from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SwitchFormField } from '@/components/switch-form-field';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import NumberInput from '@/components/originui/number-input';
import { useFindLlmByUuid } from '@/hooks/use-llm-request';
import {
  useRevalidateUnavailableValue,
  useUnavailableModelFormSchema,
} from '@/hooks/use-unavailable-value-validation';
import { zodResolver } from '@hookform/resolvers/zod';
import { get } from 'lodash';
import { memo, useEffect, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  AgentExceptionMethod,
  AgentStructuredOutputField,
  NodeHandleId,
  VariableType,
} from '../../constant';
import { useOwnerTenantId } from '../../context';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { hasSubAgentOrTool, isBottomSubAgent } from '../../utils';
import { buildOutputList } from '../../utils/build-output-list';
import { DescriptionField } from '../components/description-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { QueryVariable } from '../components/query-variable';
import { SchemaDialog } from '../components/schema-dialog';
import { SchemaPanel } from '../components/schema-panel';
import { AgentTools, Agents } from './agent-tools';
import { useBuildPromptExtraPromptOptions } from './use-build-prompt-options';
import {
  useHandleShowStructuredOutput,
  useShowStructuredOutputDialog,
} from './use-show-structured-output-dialog';
import { useGetAgentMCPIds } from './use-get-tools';
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
  ...LlmSettingSchema,
  max_retries: z.coerce.number(),
  delay_after_error: z.coerce.number().optional(),
  visual_files_var: z.string().optional(),
  max_rounds: z.coerce.number().optional(),
  exception_method: z.string().optional(),
  exception_goto: z.array(z.string()).optional(),
  exception_default_value: z.string().optional(),
  tool_timeout: z.coerce.number().optional(),
  ...LargeModelFilterFormSchema,
  cite: z.boolean().optional(),
  showStructuredOutput: z.boolean().optional(),
});

export type AgentFormSchemaType = z.infer<typeof FormSchema>;

function AgentForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const { edges, deleteEdgesBySourceAndSourceHandle } = useGraphStore(
    (state) => state,
  );

  const outputList = buildOutputList(node?.data.form.outputs);

  const defaultValues = useValues(node);

  const ownerTenantId = useOwnerTenantId();
  const { formSchema, modelsFetched } = useUnavailableModelFormSchema(
    FormSchema,
    { ownerTenantId },
  );

  const { extraOptions } = useBuildPromptExtraPromptOptions(edges, node?.id);

  const ExceptionMethodOptions = Object.values(AgentExceptionMethod).map(
    (x) => ({
      label: t(`flow.${x}`),
      value: x,
    }),
  );

  const isSubAgent = useMemo(() => {
    return isBottomSubAgent(edges, node?.id);
  }, [edges, node?.id]);

  const form = useForm<AgentFormSchemaType>({
    defaultValues: defaultValues,
    resolver: zodResolver(formSchema),
  });

  const llmId = useWatch({ control: form.control, name: 'llm_id' });

  const findLlmByUuid = useFindLlmByUuid();

  const exceptionMethod = useWatch({
    control: form.control,
    name: 'exception_method',
  });

  const { mcpIds } = useGetAgentMCPIds();

  const showStructuredOutput = useWatch({
    control: form.control,
    name: 'showStructuredOutput',
  });

  const {
    showStructuredOutputDialog,
    structuredOutputDialogVisible,
    hideStructuredOutputDialog,
    handleStructuredOutputDialogOk,
  } = useShowStructuredOutputDialog(node?.id);

  const structuredOutput = get(
    node,
    `data.form.outputs.${AgentStructuredOutputField}`,
  );

  const { handleShowStructuredOutput } = useHandleShowStructuredOutput(
    node?.id,
  );

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

  // A persisted model from a shared canvas may be unusable to the current
  // user — surface the error once the model list has loaded.
  useRevalidateUnavailableValue(form, modelsFetched, 'llm_id');

  return (
    <>
      <Form {...form}>
        <FormWrapper>
          {isSubAgent && <DescriptionField></DescriptionField>}
          <LargeModelFormField ownerTenantId={ownerTenantId} />
          {findLlmByUuid(llmId)?.model_type?.includes('vision') && (
            <QueryVariable
              name="visual_files_var"
              label={t('flow.visualInputFile')}
              types={[VariableType.File]}
            ></QueryVariable>
          )}
          <RAGFlowFormItem
            name="sys_prompt"
            label={t('flow.systemPrompt')}
            className="flex-1"
          >
            <PromptEditor
              placeholder={t('flow.messagePlaceholder')}
              showToolbar={true}
              extraOptions={extraOptions}
            />
          </RAGFlowFormItem>
          {isSubAgent || (
            <RAGFlowFormItem
              name="prompts"
              label={t('flow.userPrompt')}
              className="flex-1"
            >
              <PromptEditor showToolbar={true} />
            </RAGFlowFormItem>
          )}
          <Separator></Separator>
          <AgentTools></AgentTools>
          <Agents node={node}></Agents>
          <Collapse defaultOpen title={<div>{t('flow.advancedSettings')}</div>}>
            <section className="space-y-5">
              <SwitchFormField
                name="cite"
                label={t('flow.cite')}
                tooltip={t('chat.quoteTip')}
              />
              {(mcpIds.length > 0 || hasSubAgentOrTool(edges, node?.id)) && (
                <RAGFlowFormItem
                  name="max_rounds"
                  label={t('flow.maxRounds')}
                  className="flex-1"
                >
                  <NumberInput min={0} className="w-full" />
                </RAGFlowFormItem>
              )}
              <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
              <RAGFlowFormItem
                name="max_retries"
                label={t('flow.maxRetries')}
                className="flex-1"
              >
                <NumberInput max={8} min={0} className="w-full" />
              </RAGFlowFormItem>
              <RAGFlowFormItem
                name="delay_after_error"
                label={t('flow.delayAfterError')}
                className="flex-1"
              >
                <NumberInput max={5} step={0.1} className="w-full" />
              </RAGFlowFormItem>
              {mcpIds.length > 0 && (
                <RAGFlowFormItem
                  label={t('flow.toolTimeout')}
                  tooltip={t('flow.toolTimeoutTip')}
                  name="tool_timeout"
                >
                  {(field) => (
                    <div className="flex gap-2 items-center">
                      <NumberInput
                        value={field.value}
                        onChange={field.onChange}
                        min={1}
                      />{' '}
                      {t('flow.seconds')}
                    </div>
                  )}
                </RAGFlowFormItem>
              )}

              <RAGFlowFormItem
                name="exception_method"
                label={t('flow.exceptionMethod')}
                className="flex-1"
              >
                <SelectWithSearch options={ExceptionMethodOptions} allowClear />
              </RAGFlowFormItem>
              {exceptionMethod === AgentExceptionMethod.Comment && (
                <RAGFlowFormItem
                  name="exception_default_value"
                  label={t('flow.ExceptionDefaultValue')}
                  className="flex-1"
                >
                  <Input />
                </RAGFlowFormItem>
              )}
            </section>
          </Collapse>
          <Output list={outputList}>
            <RAGFlowFormItem name="showStructuredOutput">
              {(field) => (
                <div className="flex items-center space-x-2">
                  <Label htmlFor="airplane-mode">
                    {t('flow.structuredOutput.structuredOutput')}
                  </Label>
                  <Switch
                    id="airplane-mode"
                    checked={field.value}
                    onCheckedChange={(val) => {
                      handleShowStructuredOutput(val);
                      field.onChange(val);
                    }}
                  />
                </div>
              )}
            </RAGFlowFormItem>
          </Output>
          {showStructuredOutput && (
            <section className="space-y-2">
              <div className="flex justify-between items-center">
                {t('flow.structuredOutput.structuredOutput')}
                <Button
                  variant={'outline'}
                  onClick={showStructuredOutputDialog}
                >
                  {t('flow.structuredOutput.configuration')}
                </Button>
              </div>

              <SchemaPanel value={structuredOutput}></SchemaPanel>
            </section>
          )}
        </FormWrapper>
      </Form>
      {structuredOutputDialogVisible && (
        <SchemaDialog
          hideModal={hideStructuredOutputDialog}
          onOk={handleStructuredOutputDialogOk}
          initialValues={structuredOutput}
        ></SchemaDialog>
      )}
    </>
  );
}

export default memo(AgentForm);
