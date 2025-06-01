import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/llm-hooks';
import { Select as AntSelect, Form, message, Slider } from 'antd';
import { useCallback } from 'react';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { SliderInputFormField } from './slider-input-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from './ui/select';

type FieldType = {
  rerank_id?: string;
  top_k?: number;
};

export const RerankItem = () => {
  const { t } = useTranslate('knowledgeDetails');
  const allOptions = useSelectLlmOptionsByModelType();
  const [messageApi, contextHolder] = message.useMessage();

  const handleChange = useCallback(
    (val: string) => {
      if (val) {
        messageApi.open({
          type: 'warning',
          content: t('reRankModelWaring'),
        });
      }
    },
    [messageApi, t],
  );

  return (
    <>
      {contextHolder}
      <Form.Item
        label={t('rerankModel')}
        name={'rerank_id'}
        tooltip={t('rerankTip')}
      >
        <AntSelect
          options={allOptions[LlmModelType.Rerank]}
          allowClear
          placeholder={t('rerankPlaceholder')}
          onChange={handleChange}
        />
      </Form.Item>
    </>
  );
};

export const topKSchema = {
  top_k: z.number().optional(),
};

export const initialTopKValue = {
  top_k: 1024,
};

const Rerank = () => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <>
      <RerankItem></RerankItem>
      <Form.Item noStyle dependencies={['rerank_id']}>
        {({ getFieldValue }) => {
          const rerankId = getFieldValue('rerank_id');
          return (
            rerankId && (
              <Form.Item<FieldType>
                label={t('topK')}
                name={'top_k'}
                initialValue={1024}
                tooltip={t('topKTip')}
              >
                <Slider max={2048} min={1} />
              </Form.Item>
            )
          );
        }}
      </Form.Item>
    </>
  );
};

export default Rerank;

const RerankId = 'rerank_id';

function RerankFormField() {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');
  const allOptions = useSelectLlmOptionsByModelType();
  const options = allOptions[LlmModelType.Rerank];

  return (
    <FormField
      control={form.control}
      name={RerankId}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('rerankTip')}>{t('rerankModel')}</FormLabel>
          <FormControl>
            <Select onValueChange={field.onChange} {...field}>
              <SelectTrigger
                value={field.value}
                onReset={() => {
                  form.resetField(RerankId);
                }}
              >
                <SelectValue placeholder={t('rerankPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {options.map((x) => (
                  <SelectGroup key={x.label}>
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
  );
}

export function RerankFormFields() {
  const { watch } = useFormContext();
  const { t } = useTranslate('knowledgeDetails');
  const rerankId = watch(RerankId);

  return (
    <>
      <RerankFormField></RerankFormField>
      {rerankId && (
        <SliderInputFormField
          name={'top_k'}
          label={t('topK')}
          max={2048}
          min={1}
          tooltip={t('topKTip')}
        ></SliderInputFormField>
      )}
    </>
  );
}
