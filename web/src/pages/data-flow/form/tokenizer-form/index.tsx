import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { Form } from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  initialTokenizerValues,
  TokenizerFields,
  TokenizerSearchMethod,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

const outputList = buildOutputList(initialTokenizerValues.outputs);

export const FormSchema = z.object({
  search_method: z.array(z.string()).min(1),
  filename_embd_weight: z.number(),
  fields: z.string(),
});

const SearchMethodOptions = buildOptions(TokenizerSearchMethod);

const TokenizerForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();
  const defaultValues = useFormValues(initialTokenizerValues, node);

  const FieldsOptions = buildOptions(
    TokenizerFields,
    t,
    'dataflow.tokenizerFieldsOptions',
  );

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <RAGFlowFormItem
          name="search_method"
          label={t('dataflow.searchMethod')}
        >
          {(field) => (
            <MultiSelect
              options={SearchMethodOptions}
              onValueChange={field.onChange}
              defaultValue={field.value}
              variant="inverted"
            />
          )}
        </RAGFlowFormItem>
        <SliderInputFormField
          name="filename_embd_weight"
          label={t('dataflow.filenameEmbdWeight')}
          max={0.5}
          step={0.01}
        ></SliderInputFormField>
        <RAGFlowFormItem name="fields" label={t('dataflow.fields')}>
          {(field) => <SelectWithSearch options={FieldsOptions} {...field} />}
        </RAGFlowFormItem>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(TokenizerForm);
