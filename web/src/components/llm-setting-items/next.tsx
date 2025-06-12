import { LlmModelType, ModelVariableType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { camelCase } from 'lodash';
import { useCallback } from 'react';
import { useFormContext } from 'react-hook-form';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { Input } from '../ui/input';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '../ui/select';
import { FormSlider } from '../ui/slider';
import { Switch } from '../ui/switch';

interface SliderWithInputNumberFormFieldProps {
  name: string;
  label: string;
  checkName: string;
  max: number;
  min?: number;
  step?: number;
}

function SliderWithInputNumberFormField({
  name,
  label,
  checkName,
  max,
  min = 0,
  step = 1,
}: SliderWithInputNumberFormFieldProps) {
  const { control, watch } = useFormContext();
  const { t } = useTranslate('chat');
  const disabled = !watch(checkName);

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <div className="flex items-center justify-between">
            <FormLabel>{t(label)}</FormLabel>
            <FormField
              control={control}
              name={checkName}
              render={({ field }) => (
                <FormItem>
                  <FormControl>
                    <Switch
                      {...field}
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    ></Switch>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <FormControl>
            <div className="flex w-full  items-center space-x-2">
              <FormSlider
                {...field}
                disabled={disabled}
                max={max}
                min={min}
                step={step}
              ></FormSlider>
              <Input
                type={'number'}
                className="w-2/5"
                {...field}
                disabled={disabled}
                max={max}
                min={min}
                step={step}
              />
            </div>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

interface LlmSettingFieldItemsProps {
  prefix?: string;
}

export function LlmSettingFieldItems({ prefix }: LlmSettingFieldItemsProps) {
  const form = useFormContext();
  const { t } = useTranslate('chat');
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  const parameterOptions = Object.values(ModelVariableType).map((x) => ({
    label: t(camelCase(x)),
    value: x,
  }));

  const getFieldWithPrefix = useCallback(
    (name: string) => {
      return `${prefix}.${name}`;
    },
    [prefix],
  );

  return (
    <div className="space-y-8">
      <FormField
        control={form.control}
        name={'llm_id'}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('model')}</FormLabel>
            <FormControl>
              <Select onValueChange={field.onChange} {...field}>
                <SelectTrigger value={field.value}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {modelOptions.map((x) => (
                    <SelectGroup key={x.value}>
                      <SelectLabel>{x.label}</SelectLabel>
                      {x.options.map((y) => (
                        <SelectItem
                          value={y.value}
                          key={y.value}
                          disabled={y.disabled}
                        >
                          {y.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  ))}
                </SelectContent>
              </Select>
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
              <Select {...field} onValueChange={field.onChange}>
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
      <SliderWithInputNumberFormField
        name={getFieldWithPrefix('temperature')}
        checkName="temperatureEnabled"
        label="temperature"
        max={1}
        step={0.01}
      ></SliderWithInputNumberFormField>
      <SliderWithInputNumberFormField
        name={getFieldWithPrefix('top_p')}
        checkName="topPEnabled"
        label="topP"
        max={1}
        step={0.01}
      ></SliderWithInputNumberFormField>
      <SliderWithInputNumberFormField
        name={getFieldWithPrefix('presence_penalty')}
        checkName="presencePenaltyEnabled"
        label="presencePenalty"
        max={1}
        step={0.01}
      ></SliderWithInputNumberFormField>
      <SliderWithInputNumberFormField
        name={getFieldWithPrefix('frequency_penalty')}
        checkName="frequencyPenaltyEnabled"
        label="frequencyPenalty"
        max={1}
        step={0.01}
      ></SliderWithInputNumberFormField>
      <SliderWithInputNumberFormField
        name={getFieldWithPrefix('max_tokens')}
        checkName="maxTokensEnabled"
        label="maxTokens"
        max={128000}
      ></SliderWithInputNumberFormField>
    </div>
  );
}
