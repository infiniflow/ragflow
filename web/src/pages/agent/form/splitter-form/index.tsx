import { DelimiterInput } from '@/components/delimiter-form-field';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { BlockButton, Button } from '@/components/ui/button';
import { Form, FormControl, FormField, FormItem } from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
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
  image_table_context_window: z.number(),
  delimiters: z.array(
    z.object({
      value: z.string().optional(),
    }),
  ),
  enable_children: z.boolean(),
  children_delimiters: z.array(
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

  const childrenDelimiters = useFieldArray({
    name: 'children_delimiters',
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
        <SliderInputFormField
          name="image_table_context_window"
          max={256}
          min={0}
          label={t('knowledgeConfiguration.imageTableContextWindow')}
          tooltip={t('knowledgeConfiguration.imageTableContextWindowTip')}
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

        <fieldset>
          <div className="mb-2 flex justify-between items-center gap-1">
            <span>{t('flow.enableChildrenDelimiters')}</span>

            <FormField
              control={form.control}
              name="enable_children"
              render={({ field: { value, onChange, ...restProps } }) => (
                <FormItem>
                  <FormControl>
                    <Switch
                      checked={value}
                      onCheckedChange={onChange}
                      {...restProps}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>

          {form.getValues('enable_children') && (
            <div className="space-y-4">
              {childrenDelimiters.fields.map((field, index) => (
                <div key={field.id} className="flex items-center gap-2">
                  <RAGFlowFormItem
                    name={`children_delimiters.${index}.value`}
                    label="children_delimiter"
                    labelClassName="!hidden"
                    className="flex-auto space-y-0"
                  >
                    <DelimiterInput className="!m-0"></DelimiterInput>
                  </RAGFlowFormItem>

                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => childrenDelimiters.remove(index)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}

              <BlockButton
                onClick={() => childrenDelimiters.append({ value: '\n' })}
              >
                {t('common.add')}
              </BlockButton>
            </div>
          )}
        </fieldset>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(SplitterForm);
