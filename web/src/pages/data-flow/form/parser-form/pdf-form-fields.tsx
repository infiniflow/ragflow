import {
  LanguageFormField,
  LargeModelFormField,
  OutputFormatFormField,
  ParserMethodFormField,
} from './common-form-fields';
import { CommonProps } from './interface';

export function PdfFormFields({ prefix }: CommonProps) {
  return (
    <>
      <ParserMethodFormField prefix={prefix}></ParserMethodFormField>
      {/* Multimodal Model */}
      <LargeModelFormField prefix={prefix}></LargeModelFormField>
      <LanguageFormField prefix={prefix}></LanguageFormField>
      <OutputFormatFormField prefix={prefix}></OutputFormatFormField>
    </>
  );
}
