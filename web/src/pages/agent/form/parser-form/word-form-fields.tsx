import {
  OutputFormatFormFieldProps,
  RemoveHeaderFooterFormField,
  RmdirFormField,
} from './common-form-fields';

export function WordFormFields({ prefix }: OutputFormatFormFieldProps) {
  return (
    <>
      <RmdirFormField prefix={prefix} />
      <RemoveHeaderFooterFormField prefix={prefix} />
    </>
  );
}
