import { RemoveHeaderFooterFormField } from './common-form-fields';
import { CommonProps } from './interface';

export function HtmlFormFields({ prefix }: CommonProps) {
  return (
    <>
      <RemoveHeaderFooterFormField prefix={prefix} />
    </>
  );
}
