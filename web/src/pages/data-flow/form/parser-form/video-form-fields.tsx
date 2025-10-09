import {
  LargeModelFormField,
  OutputFormatFormFieldProps,
} from './common-form-fields';

export function VideoFormFields({ prefix }: OutputFormatFormFieldProps) {
  return (
    <>
      {/* Multimodal Model */}
      <LargeModelFormField prefix={prefix}></LargeModelFormField>
    </>
  );
}
