import {
  LargeModelFormField,
  OutputFormatFormField,
  ParserMethodFormField,
} from './common-form-fields';
import { CommonProps } from './interface';

export function ImageFormFields({ prefix }: CommonProps) {
  return (
    <>
      <ParserMethodFormField prefix={prefix}></ParserMethodFormField>
      {/* Multimodal Model */}
      <LargeModelFormField prefix={prefix}></LargeModelFormField>
      <OutputFormatFormField prefix={prefix}></OutputFormatFormField>
    </>
  );
}
