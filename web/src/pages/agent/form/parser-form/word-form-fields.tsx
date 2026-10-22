import { FileType } from '@/constants/file';
import { useWatch } from 'react-hook-form';
import { RemoveHeaderFooterFormField } from './common-form-fields';
import { HyperlinkFormFields } from './hyperlink-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function WordFormFields({ prefix }: CommonProps) {
  // One widget serves .doc and .docx, but the backend rejects the
  // linked-content switches outside the docx family, so they are offered only
  // where they will be saved.
  const fileFormat = useWatch({
    name: buildFieldNameWithPrefix('fileFormat', prefix),
  });

  return (
    <>
      <RemoveHeaderFooterFormField prefix={prefix} />
      {fileFormat === FileType.Docx && <HyperlinkFormFields prefix={prefix} />}
    </>
  );
}
