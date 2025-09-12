import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { FileType } from '../../constant';
import {
  LargeModelFormField,
  OutputFormatFormField,
  ParserMethodFormField,
} from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function PdfFormFields({ prefix }: CommonProps) {
  return (
    <>
      <ParserMethodFormField prefix={prefix}></ParserMethodFormField>
      {/* Multimodal Model */}
      <LargeModelFormField prefix={prefix}></LargeModelFormField>
      <CrossLanguageFormField
        name={buildFieldNameWithPrefix(`lang`, prefix)}
        label="lang"
      ></CrossLanguageFormField>
      <OutputFormatFormField
        prefix={prefix}
        fileType={FileType.Image}
      ></OutputFormatFormField>
    </>
  );
}
