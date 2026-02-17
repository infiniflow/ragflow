import {
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { camelCase, isEqual } from 'lodash';
import { useCallback } from 'react';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';
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
import { LLMFormField } from './llm-form-field';
import { SliderInputSwitchFormField } from './slider';
import { useHandleFreedomChange } from './use-watch-change';

interface LlmSettingFieldItemsProps {
  prefix?: string;
  options?: any[];
  llmId?: string;
  showFields?: Array<
    | 'temperature'
    | 'top_p'
    | 'presence_penalty'
    | 'frequency_penalty'
    | 'max_tokens'
  >;
}

export const LLMIdFormField = {
  llm_id: z.string(),
};

export const LlmSettingEnabledSchema = {
  temperatureEnabled: z.boolean().optional(),
  topPEnabled: z.boolean().optional(),
  presencePenaltyEnabled: z.boolean().optional(),
  frequencyPenaltyEnabled: z.boolean().optional(),
  maxTokensEnabled: z.boolean().optional(),
};

export const LlmSettingFieldSchema = {
  temperature: z.coerce.number().optional(),
  top_p: z.number().optional(),
  presence_penalty: z.coerce.number().optional(),
  frequency_penalty: z.coerce.number().optional(),
  max_tokens: z.number().optional(),
};

export const LlmSettingSchema = {
  ...LLMIdFormField,
  ...LlmSettingFieldSchema,
  ...LlmSettingEnabledSchema,
};

export function LlmSettingFieldItems({
  prefix,
  options,
  showFields = [
    'temperature',
    'top_p',
    'presence_penalty',
    'frequency_penalty',
    'max_tokens',
  ],
  llmId,
}: LlmSettingFieldItemsProps) {
  const form = useFormContext();
  const { t } = useTranslate('chat');

  const getFieldWithPrefix = useCallback(
    (name: string) => {
      return prefix ? `${prefix}.${name}` : name;
    },
    [prefix],
  );

  const handleChange = useHandleFreedomChange(getFieldWithPrefix);

  const parameterOptions = Object.values(ModelVariableType).map((x) => ({
    label: t(camelCase(x)),
    value: x,
  })) as { label: string; value: ModelVariableType | 'Custom' }[];

  parameterOptions.push({
    label: t(camelCase('Custom')),
    value: 'Custom',
  });
  const checkParameterIsEqual = () => {
    const [
      parameter,
      topPValue,
      frequencyPenaltyValue,
      temperatureValue,
      presencePenaltyValue,
      maxTokensValue,
    ] = form.getValues([
      getFieldWithPrefix('parameter'),
      getFieldWithPrefix('temperature'),
      getFieldWithPrefix('top_p'),
      getFieldWithPrefix('frequency_penalty'),
      getFieldWithPrefix('presence_penalty'),
      getFieldWithPrefix('max_tokens'),
    ]);
    if (parameter && parameter !== 'Custom') {
      const parameterValue =
        settledModelVariableMap[parameter as keyof typeof ModelVariableType];
      const parameterRealValue = {
        top_p: topPValue,
        temperature: temperatureValue,
        frequency_penalty: frequencyPenaltyValue,
        presence_penalty: presencePenaltyValue,
        max_tokens: maxTokensValue,
      };
      if (!isEqual(parameterValue, parameterRealValue)) {
        form.setValue(getFieldWithPrefix('parameter'), 'Custom');
      }
    }
  };

  return (
    <div className="space-y-5">
      <LLMFormField
        options={options}
        name={llmId ?? getFieldWithPrefix('llm_id')}
      ></LLMFormField>
      <FormField
        control={form.control}
        name={getFieldWithPrefix('parameter')}
        render={({ field }) => (
          <FormItem className="flex justify-between items-center">
            <FormLabel className="flex-1">{t('freedom')}</FormLabel>
            <FormControl>
              <Select
                {...field}
                onValueChange={(val) => {
                  handleChange(val);
                  field.onChange(val);
                }}
              >
                <SelectTrigger className="flex-1 !m-0">
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
      {showFields.some((item) => item === 'temperature') && (
        <SliderInputSwitchFormField
          name={getFieldWithPrefix('temperature')}
          checkName="temperatureEnabled"
          label="temperature"
          max={1}
          step={0.01}
          min={0}
          onChange={() => {
            checkParameterIsEqual();
          }}
        ></SliderInputSwitchFormField>
      )}
      {showFields.some((item) => item === 'top_p') && (
        <SliderInputSwitchFormField
          name={getFieldWithPrefix('top_p')}
          checkName="topPEnabled"
          label="topP"
          max={1}
          step={0.01}
          min={0}
          onChange={() => {
            checkParameterIsEqual();
          }}
        ></SliderInputSwitchFormField>
      )}
      {showFields.some((item) => item === 'presence_penalty') && (
        <SliderInputSwitchFormField
          name={getFieldWithPrefix('presence_penalty')}
          checkName="presencePenaltyEnabled"
          label="presencePenalty"
          max={1}
          step={0.01}
          min={0}
          onChange={() => {
            checkParameterIsEqual();
          }}
        ></SliderInputSwitchFormField>
      )}
      {showFields.some((item) => item === 'frequency_penalty') && (
        <SliderInputSwitchFormField
          name={getFieldWithPrefix('frequency_penalty')}
          checkName="frequencyPenaltyEnabled"
          label="frequencyPenalty"
          max={1}
          step={0.01}
          min={0}
          onChange={() => {
            checkParameterIsEqual();
          }}
        ></SliderInputSwitchFormField>
      )}
      {showFields.some((item) => item === 'max_tokens') && (
        <SliderInputSwitchFormField
          name={getFieldWithPrefix('max_tokens')}
          checkName="maxTokensEnabled"
          numberInputClassName="w-20"
          label="maxTokens"
          max={128000}
          min={0}
          onChange={() => {
            checkParameterIsEqual();
          }}
        ></SliderInputSwitchFormField>
      )}
    </div>
  );
}
