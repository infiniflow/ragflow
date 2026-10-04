import {
  RemoveHeaderFooterFormField,
  RmdirFormField,
} from './common-form-fields';
import { CommonProps } from './interface';

export function TextMarkdownFormFields({ prefix }: CommonProps) {
  return (
    <>
      <RmdirFormField prefix={prefix} />
    </>
  );
}

export function HtmlFormFields({ prefix }: CommonProps) {
  return (
    <>
      <RmdirFormField prefix={prefix} />
      <RemoveHeaderFooterFormField prefix={prefix} />
    </>
  );
}
