/**
 * ContentEditable wrapper — thin wrapper around Lexical's contenteditable.
 */

import { ContentEditable } from '@lexical/react/LexicalContentEditable';
import type { JSX } from 'react';

interface Props {
  className?: string;
  placeholderClassName?: string;
  placeholder: string;
}

export default function LexicalContentEditable({
  className,
  placeholder,
  placeholderClassName,
}: Props): JSX.Element {
  return (
    <ContentEditable
      className={className ?? 'nim-content-editable'}
      aria-placeholder={placeholder}
      placeholder={
        <div className={placeholderClassName ?? 'nim-placeholder'}>
          {placeholder}
        </div>
      }
    />
  );
}
