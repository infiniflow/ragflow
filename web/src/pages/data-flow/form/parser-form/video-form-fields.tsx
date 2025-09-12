import {
  LargeModelFormField,
  OutputFormatFormField,
  OutputFormatFormFieldProps,
} from './common-form-fields';

export function VideoFormFields({
  prefix,
  fileType,
}: OutputFormatFormFieldProps) {
  return (
    <>
      {/* Multimodal Model */}
      <LargeModelFormField prefix={prefix}></LargeModelFormField>
      <OutputFormatFormField
        prefix={prefix}
        fileType={fileType}
      ></OutputFormatFormField>
    </>
  );
}
