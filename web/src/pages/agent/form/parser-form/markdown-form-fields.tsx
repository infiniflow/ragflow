import { RmdirFormField } from './common-form-fields';
import { CommonProps } from './interface';

/** Renders Markdown parsing controls shared by canvas and dataset configuration. */
export function MarkdownFormFields({ prefix }: CommonProps) {
  return <RmdirFormField prefix={prefix} />;
}
