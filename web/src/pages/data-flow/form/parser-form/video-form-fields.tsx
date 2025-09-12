import {
  LargeModelFormField,
  OutputFormatFormField,
} from './common-form-fields';
import { CommonProps } from './interface';

export function VideoFormFields({ prefix }: CommonProps) {
  return (
    <>
      {/* Multimodal Model */}
      <LargeModelFormField prefix={prefix}></LargeModelFormField>
      <OutputFormatFormField prefix={prefix}></OutputFormatFormField>
    </>
  );
}
