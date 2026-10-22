import {
  OutputFormatFormFieldProps,
  RemoveHeaderFooterFormField,
} from './common-form-fields';
import { HyperlinkFormFields } from './hyperlink-form-fields';
import { FileType } from '@/constants/file';

export function WordFormFields({
  prefix,
  fileType,
}: OutputFormatFormFieldProps) {
  return (
    <>
      <RemoveHeaderFooterFormField prefix={prefix} />
      {fileType === FileType.Docx && <HyperlinkFormFields prefix={prefix} />}
    </>
  );
}
