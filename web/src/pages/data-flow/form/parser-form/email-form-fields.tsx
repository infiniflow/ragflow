import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';
import { FileType } from '../../constant';
import { OutputFormatFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

const options = buildOptions([
  'from',
  'to',
  'cc',
  'bcc',
  'date',
  'subject',
  'body',
  'attachments',
]);

export function EmailFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  return (
    <>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix(`fields`, prefix)}
        label={t('dataflow.fields')}
      >
        <SelectWithSearch options={options}></SelectWithSearch>
      </RAGFlowFormItem>
      <OutputFormatFormField
        prefix={prefix}
        fileType={FileType.Email}
      ></OutputFormatFormField>
    </>
  );
}
