import { HyperlinkFormFields } from './hyperlink-form-fields';
import { CommonProps } from './interface';

export function MarkdownFormFields({ prefix }: CommonProps) {
  return <HyperlinkFormFields prefix={prefix} showMarkdownLinks />;
}
