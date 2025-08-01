import { LlmModelType, ModelVariableType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { camelCase } from 'lodash';
import { useCallback } from 'react';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { SelectWithSearch } from '../originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select';
import { SliderInputSwitchFormField } from './slider';
import { useHandleFreedomChange } from './use-watch-change';

interface LlmSettingFieldItemsProps {
  prefix?: string;
  options?: any[];
}

export const LlmSettingSchema = {
  llm_id: z.string(),
  temperature: z.coerce.number(),
  top_p: z.string(),
  presence_penalty: z.coerce.number(),
  frequency_penalty: z.coerce.number(),
  temperatureEnabled: z.boolean(),
  topPEnabled: z.boolean(),
  presencePenaltyEnabled: z.boolean(),
  frequencyPenaltyEnabled: z.boolean(),
  maxTokensEnabled: z.boolean(),
};

export function LlmSettingFieldItems({
  prefix,
  options,
}: LlmSettingFieldItemsProps) {
  const form = useFormContext();
  const { t } = useTranslate('chat');

  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  const handleChange = useHandleFreedomChange();

  const parameterOptions = Object.values(ModelVariableType).map((x) => ({
    label: t(camelCase(x)),
    value: x,
  }));

  const getFieldWithPrefix = useCallback(
    (name: string) => {
      return prefix ? `${prefix}.${name}` : name;
    },
    [prefix],
  );

  return (
    <div className="space-y-5">
      <FormField
        control={form.control}
        name={'llm_id'}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('model')}</FormLabel>
            <FormControl>
              <SelectWithSearch
                options={options || modelOptions}
                {...field}
              ></SelectWithSearch>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'parameter'}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('freedom')}</FormLabel>
            <FormControl>
              <Select
                {...field}
                onValueChange={(val) => {
                  handleChange(val);
                  field.onChange(val);
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {parameterOptions.map((x) => (
                    <SelectItem value={x.value} key={x.value}>
                      {x.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('temperature')}
        checkName="temperatureEnabled"
        label="temperature"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('top_p')}
        checkName="topPEnabled"
        label="topP"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('presence_penalty')}
        checkName="presencePenaltyEnabled"
        label="presencePenalty"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('frequency_penalty')}
        checkName="frequencyPenaltyEnabled"
        label="frequencyPenalty"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('max_tokens')}
        checkName="maxTokensEnabled"
        label="maxTokens"
        max={128000}
      ></SliderInputSwitchFormField>
    </div>
  );
}
