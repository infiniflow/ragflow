import { DelimiterInput } from '@/components/delimiter-form-field';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { BlockButton, Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Trash2 } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialSplitterValues } from '../../constant/pipeline';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

const outputList = buildOutputList(initialSplitterValues.outputs);

export const FormSchema = z.object({
  chunk_token_size: z.number(),
  delimiters: z.array(
    z.object({
      value: z.string().optional(),
    }),
  ),
  overlapped_percent: z.number(), // 0.0 - 0.3 , 0% - 30%
});

export type SplitterFormSchemaType = z.infer<typeof FormSchema>;

const SplitterForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialSplitterValues, node);
  const { t } = useTranslation();

  const form = useForm<SplitterFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });
  const name = 'delimiters';

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <SliderInputFormField
          name="chunk_token_size"
          max={2048}
          label={t('knowledgeConfiguration.chunkTokenNumber')}
        ></SliderInputFormField>
        <SliderInputFormField
          name="overlapped_percent"
          max={30}
          min={0}
          label={t('flow.overlappedPercent')}
        ></SliderInputFormField>
        <section>
          <span className="mb-2 inline-block">{t('flow.delimiters')}</span>
          <div className="space-y-4">
            {fields.map((field, index) => (
              <div key={field.id} className="flex items-center gap-2">
                <div className="space-y-2 flex-1">
                  <RAGFlowFormItem
                    name={`${name}.${index}.value`}
                    label="delimiter"
                    labelClassName="!hidden"
                  >
                    <DelimiterInput className="!m-0"></DelimiterInput>
                  </RAGFlowFormItem>
                </div>
                <Button
                  type="button"
                  variant={'ghost'}
                  onClick={() => remove(index)}
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
          </div>
        </section>
        <BlockButton onClick={() => append({ value: '\n' })}>
          {t('common.add')}
        </BlockButton>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(SplitterForm);
