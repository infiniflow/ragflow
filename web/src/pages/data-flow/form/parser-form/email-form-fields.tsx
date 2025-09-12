import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { buildOptions } from '@/utils/form';
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
  return (
    <>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix(`fields`, prefix)}
        label="fields"
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
