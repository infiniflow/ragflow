import { RemoveHeaderFooterFormField } from './common-form-fields';
import { CommonProps } from './interface';

export function WordFormFields({ prefix }: CommonProps) {
  return (
    <>
      <RemoveHeaderFooterFormField prefix={prefix} />
    </>
  );
}
