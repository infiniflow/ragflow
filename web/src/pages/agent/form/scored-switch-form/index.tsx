import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import { BlurTextarea } from '@/components/ui/textarea';
import { zodResolver } from '@hookform/resolvers/zod';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { useUpdateNodeInternals } from '@xyflow/react';
import { memo, useEffect, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ScoredSwitchEscalateHandleId } from '../../scored-switch';
import { useOwnerTenantId } from '../../context';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import { initialScoredSwitchValues } from '../../constant';
import { ScoredSwitchFields } from './fields-editor';
import { ScoredSwitchOptions } from './options-editor';
import { ScoredSwitchTasks } from './tasks-editor';
import { useCreateScoredSwitchFormSchema } from './use-form-schema';
import { useValues } from './use-values';

function ScoredSwitchForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const values = useValues(node);
  const FormSchema = useCreateScoredSwitchFormSchema();
  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });
  useWatchFormChange(node?.id, form);
  const ownerTenantId = useOwnerTenantId();
  const { data: addedModels } = useFetchAllAddedModels(undefined, ownerTenantId);
  const llmId = useWatch({ control: form.control, name: 'llm_id' });
  const selectedModel = (addedModels ?? []).find(
    (model) => model.model_id === llmId,
  );
  const features = new Set(selectedModel?.features ?? []);
  const imageSupport = features.has('supports_image');
  const boxSupport = features.has('supports_box');
  const yesNoSupport = features.has('supports_noul');
  const levelSupport = features.has('supports_score');
  const fieldMode = features.has('supports_fields');
  useEffect(() => {
    if (!addedModels) {
      return;
    }
    if (form.getValues('field_mode') !== fieldMode) {
      form.setValue('field_mode', fieldMode, {
        shouldDirty: true,
        shouldValidate: true,
      });
    }
  }, [addedModels, fieldMode, form]);
  const escalateEnabled = useWatch({
    control: form.control,
    name: 'escalate_enabled',
  });
  const deleteEdges = useGraphStore(
    (state) => state.deleteEdgesBySourceAndSourceHandle,
  );
  const updateNodeInternals = useUpdateNodeInternals();
  const outputList = useMemo(
    () => [
      {
        title: 'chosen_id',
        type: initialScoredSwitchValues.outputs.chosen_id.type,
        description: t('flow.scoredSwitchOutputChosenId'),
      },
      {
        title: 'chosen_label',
        type: initialScoredSwitchValues.outputs.chosen_label.type,
        description: t('flow.scoredSwitchOutputChosenLabel'),
      },
      {
        title: 'scores',
        type: initialScoredSwitchValues.outputs.scores.type,
        description: t('flow.scoredSwitchOutputScores'),
      },
      {
        title: 'margin',
        type: initialScoredSwitchValues.outputs.margin.type,
        description: t('flow.scoredSwitchOutputMargin'),
      },
      {
        title: 'escalated',
        type: initialScoredSwitchValues.outputs.escalated.type,
        description: t('flow.scoredSwitchOutputEscalated'),
      },
      {
        title: 'selected_handle',
        type: initialScoredSwitchValues.outputs.selected_handle.type,
        description: t('flow.scoredSwitchOutputSelectedHandle'),
      },
      {
        title: 'confidence',
        type: initialScoredSwitchValues.outputs.confidence.type,
        description: t('flow.scoredSwitchOutputConfidence'),
      },
      {
        title: 'raw',
        type: initialScoredSwitchValues.outputs.raw.type,
        description: t('flow.scoredSwitchOutputRaw'),
      },
      {
        title: 'answers',
        type: initialScoredSwitchValues.outputs.answers.type,
        description: t('flow.scoredSwitchOutputAnswers'),
      },
      {
        title: 'usage',
        type: initialScoredSwitchValues.outputs.usage.type,
        description: t('flow.scoredSwitchOutputUsage'),
      },
      {
        title: 'state_text',
        type: initialScoredSwitchValues.outputs.state_text.type,
        description: t('flow.scoredSwitchOutputStateText'),
      },
    ],
    [t],
  );

  return (
    <Form {...form}>
      <FormWrapper>
        <QueryVariable></QueryVariable>
        {imageSupport && (
          <QueryVariable
            name="image"
            label={t('flow.scoredSwitchImage')}
            tooltip={
              fieldMode
                ? t('flow.scoredSwitchFieldImageTip')
                : t('flow.scoredSwitchImageTip')
            }
          ></QueryVariable>
        )}
        <ModelTreeSelectFormField
          name="llm_id"
          label={t('flow.scoredSwitchModel')}
          tooltip={t('flow.scoredSwitchModelTip')}
          modelTypes={ModelTypeMap.scoring_id}
          ownerTenantId={ownerTenantId}
        ></ModelTreeSelectFormField>
        {imageSupport && !fieldMode && (
        <FormField
          control={form.control}
          name="request_format"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('flow.scoredSwitchRequestFormatTip')}>
                {t('flow.scoredSwitchRequestFormat')}
              </FormLabel>
              <Select onValueChange={field.onChange} value={field.value}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="content_parts">
                    {t('flow.scoredSwitchContentParts')}
                  </SelectItem>
                  <SelectItem value="assets">
                    {t('flow.scoredSwitchAssets')}
                  </SelectItem>
                </SelectContent>
              </Select>
              <FormDescription>
                {t('flow.scoredSwitchRequestFormatTip')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        )}
        {!fieldMode && (
        <>
        <FormField
          control={form.control}
          name="instructions"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('flow.scoredSwitchInstructionsTip')}>
                {t('flow.scoredSwitchInstructions')}
              </FormLabel>
              <FormControl>
                <BlurTextarea {...field} rows={3} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Separator />
        <ScoredSwitchOptions nodeId={node?.id}></ScoredSwitchOptions>
        <ScoredSwitchTasks
          allowYesNo={yesNoSupport}
          allowLevels={levelSupport}
          allowBox={boxSupport}
        ></ScoredSwitchTasks>
        <Separator />
        <FormField
          control={form.control}
          name="escalate_enabled"
          render={({ field }) => (
            <FormItem className="flex flex-row items-center justify-between gap-4">
              <FormLabel
                className="w-fit"
                tooltip={t('flow.scoredSwitchEscalateToLlmTip')}
              >
                {t('flow.scoredSwitchEscalateToLlm')}
              </FormLabel>
              <FormControl>
                <Switch
                  className="!m-0"
                  checked={field.value !== false}
                  onCheckedChange={(checked) => {
                    field.onChange(checked);
                    if (!checked && node?.id) {
                      deleteEdges(node.id, ScoredSwitchEscalateHandleId);
                      updateNodeInternals(node.id);
                    }
                  }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {escalateEnabled !== false && (
          <>
            <FormField
              control={form.control}
              name="margin_threshold"
              render={({ field }) => (
                <FormItem>
                  <FormLabel tooltip={t('flow.scoredSwitchMarginThresholdTip')}>
                    {t('flow.scoredSwitchMarginThreshold')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      min={0}
                      max={1}
                      step={0.01}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('flow.scoredSwitchMarginThresholdTip')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="min_top_score"
              render={({ field }) => (
                <FormItem>
                  <FormLabel tooltip={t('flow.scoredSwitchMinTopScoreTip')}>
                    {t('flow.scoredSwitchMinTopScore')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      min={0}
                      max={1}
                      step={0.01}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('flow.scoredSwitchMinTopScoreTip')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </>
        )}
        </>
        )}
        {fieldMode && (
          <>
            <FormField
              control={form.control}
              name="reference_date"
              render={({ field }) => (
                <FormItem>
                  <FormLabel tooltip={t('flow.scoredSwitchReferenceDateTip')}>
                    {t('flow.scoredSwitchReferenceDate')}
                  </FormLabel>
                  <FormControl>
                    <Input placeholder="2026-01-01" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="include_quantiles"
              render={({ field }) => (
                <FormItem className="flex flex-row items-center justify-between gap-4">
                  <FormLabel className="w-fit" tooltip={t('flow.scoredSwitchQuantilesTip')}>
                    {t('flow.scoredSwitchQuantiles')}
                  </FormLabel>
                  <FormControl>
                    <Switch
                      className="!m-0"
                      checked={field.value === true}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="return_state_text"
              render={({ field }) => (
                <FormItem className="flex flex-row items-center justify-between gap-4">
                  <FormLabel className="w-fit" tooltip={t('flow.scoredSwitchStateTextTip')}>
                    {t('flow.scoredSwitchStateText')}
                  </FormLabel>
                  <FormControl>
                    <Switch
                      className="!m-0"
                      checked={field.value === true}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <Separator />
            <ScoredSwitchFields
              allow={{
                text: features.has('supports_field_text'),
                date: features.has('supports_field_date'),
                time: features.has('supports_field_time'),
                datetime: features.has('supports_field_datetime'),
                duration: features.has('supports_field_duration'),
                number: features.has('supports_field_number'),
                list: features.has('supports_field_list'),
                estimate: features.has('supports_field_estimate'),
                forecast: features.has('supports_field_forecast'),
              }}
            />
          </>
        )}
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(ScoredSwitchForm);
