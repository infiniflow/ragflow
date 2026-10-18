import {
  OutputFormatFormFieldProps,
  RemoveHeaderFooterFormField,
} from './common-form-fields';

export function WordFormFields({ prefix }: OutputFormatFormFieldProps) {
  return (
    <>
      <RemoveHeaderFooterFormField prefix={prefix} />
    </>
  );
}
