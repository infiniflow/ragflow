import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/llm-hooks';
import { Select as AntSelect, Form, Slider } from 'antd';
import { useFormContext } from 'react-hook-form';
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
import { FormSlider } from './ui/slider';

type FieldType = {
  rerank_id?: string;
  top_k?: number;
};

export const RerankItem = () => {
  const { t } = useTranslate('knowledgeDetails');
  const allOptions = useSelectLlmOptionsByModelType();

  return (
    <Form.Item
      label={t('rerankModel')}
      name={'rerank_id'}
      tooltip={t('rerankTip')}
    >
      <AntSelect
        options={allOptions[LlmModelType.Rerank]}
        allowClear
        placeholder={t('rerankPlaceholder')}
      />
    </Form.Item>
  );
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
          <FormLabel>{t('rerankModel')}</FormLabel>
          <FormControl>
            <Select onValueChange={field.onChange} {...field}>
              <SelectTrigger
                className="w-[280px]"
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
  const { control, watch } = useFormContext();
  const { t } = useTranslate('knowledgeDetails');
  const rerankId = watch(RerankId);

  return (
    <>
      <RerankFormField></RerankFormField>
      {rerankId && (
        <FormField
          control={control}
          name={'top_k'}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('topK')}</FormLabel>
              <FormControl>
                <FormSlider {...field} max={2048} min={1}></FormSlider>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </>
  );
}
