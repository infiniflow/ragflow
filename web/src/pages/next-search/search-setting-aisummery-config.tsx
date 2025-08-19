import { SliderInputSwitchFormField } from '@/components/llm-setting-items/slider';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  LlmModelType,
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { camelCase } from 'lodash';
import { useCallback } from 'react';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';

interface LlmSettingFieldItemsProps {
  prefix?: string;
  options?: any[];
}
const LlmSettingEnableSchema = {
  temperatureEnabled: z.boolean(),
  topPEnabled: z.boolean(),
  presencePenaltyEnabled: z.boolean(),
  frequencyPenaltyEnabled: z.boolean(),
};
export const LlmSettingSchema = {
  llm_id: z.string(),
  parameter: z.string().optional(),
  temperature: z.coerce.number().optional(),
  top_p: z.coerce.number().optional(),
  presence_penalty: z.coerce.number().optional(),
  frequency_penalty: z.coerce.number().optional(),
  ...LlmSettingEnableSchema,
  // maxTokensEnabled: z.boolean(),
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

  const handleChange = useCallback(
    (parameter: string) => {
      const currentValues = { ...form.getValues() };
      console.log('currentValues', currentValues);
      const values =
        settledModelVariableMap[
          parameter as keyof typeof settledModelVariableMap
        ];
      const enabledKeys = Object.keys(LlmSettingEnableSchema);

      // const nextValues = { ...currentValues, ...values };

      for (const key in values) {
        if (Object.prototype.hasOwnProperty.call(values, key)) {
          const element = values[key];

          form.setValue(`${prefix}.${key}`, element);
        }
      }
      if (enabledKeys && enabledKeys.length) {
        for (const key of enabledKeys) {
          form.setValue(`${prefix}.${key}`, true);
        }
      }
    },
    [form, prefix],
  );

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
        name={getFieldWithPrefix('llm_id')}
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              <span className="text-destructive mr-1"> *</span>
              {t('model')}
            </FormLabel>
            <FormControl>
              <SelectWithSearch
                options={options || modelOptions}
                triggerClassName="bg-bg-card"
                {...field}
              ></SelectWithSearch>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={getFieldWithPrefix('parameter')}
        render={({ field }) => (
          <FormItem className="flex justify-between gap-4 items-center">
            <FormLabel>{t('freedom')}</FormLabel>
            <FormControl>
              <div className="w-28">
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
              </div>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('temperature')}
        checkName={getFieldWithPrefix('temperatureEnabled')}
        label="temperature"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('top_p')}
        checkName={getFieldWithPrefix('topPEnabled')}
        label="topP"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('presence_penalty')}
        checkName={getFieldWithPrefix('presencePenaltyEnabled')}
        label="presencePenalty"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      <SliderInputSwitchFormField
        name={getFieldWithPrefix('frequency_penalty')}
        checkName={getFieldWithPrefix('frequencyPenaltyEnabled')}
        label="frequencyPenalty"
        max={1}
        step={0.01}
      ></SliderInputSwitchFormField>
      {/* <SliderInputSwitchFormField
        name={getFieldWithPrefix('max_tokens')}
        checkName="maxTokensEnabled"
        label="maxTokens"
        max={128000}
      ></SliderInputSwitchFormField> */}
    </div>
  );
}
