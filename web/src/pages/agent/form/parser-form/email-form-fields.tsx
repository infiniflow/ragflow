import { RAGFlowFormItem } from '@/components/ragflow-form';
import { MultiSelect } from '@/components/ui/multi-select';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';
import { ParserFields } from '../../constant/pipeline';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

const options = buildOptions(ParserFields);

export function EmailFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  return (
    <>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix(`fields`, prefix)}
        label={t('flow.fields')}
      >
        {(field) => (
          <MultiSelect
            options={options}
            onValueChange={field.onChange}
            defaultValue={field.value}
            variant="inverted"
          ></MultiSelect>
        )}
      </RAGFlowFormItem>
    </>
  );
}
