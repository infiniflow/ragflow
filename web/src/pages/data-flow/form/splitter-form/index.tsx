import { FormContainer } from '@/components/form-container';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { BlockButton, Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialChunkerValues, initialSplitterValues } from '../../constant';
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
  overlapped_percent: z.number(), // 0.0 - 0.3
});

const SplitterForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialChunkerValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
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
        <FormContainer>
          <SliderInputFormField
            name="chunk_token_size"
            max={2048}
            label="chunk_token_size"
          ></SliderInputFormField>
          <SliderInputFormField
            name="overlapped_percent"
            max={0.3}
            min={0.1}
            step={0.01}
            label="overlapped_percent"
          ></SliderInputFormField>
          <span>delimiters</span>
          {fields.map((field, index) => (
            <div key={field.id} className="flex items-center gap-2">
              <div className="space-y-2 flex-1">
                <RAGFlowFormItem
                  name={`${name}.${index}.value`}
                  label="delimiter"
                  labelClassName="!hidden"
                >
                  <Input className="!m-0"></Input>
                </RAGFlowFormItem>
              </div>
              <Button
                type="button"
                variant={'ghost'}
                onClick={() => remove(index)}
              >
                <X />
              </Button>
            </div>
          ))}
          <BlockButton onClick={() => append({ value: '' })}>Add</BlockButton>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(SplitterForm);
