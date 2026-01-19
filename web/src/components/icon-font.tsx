import { FileIconMap } from '@/constants/file';
import { cn } from '@/lib/utils';
import { getExtension } from '@/utils/document-util';
import { CSSProperties } from 'react';

type IconFontType = {
  name: string;
  className?: string;
  style?: CSSProperties;
};

export const IconFont = ({ name, className, style }: IconFontType) => (
  <svg className={cn('size-4', className)} style={style}>
    <use xlinkHref={`#icon-${name}`} />
  </svg>
);

export function IconFontFill({
  name,
  className,
  isFill = true,
}: IconFontType & { isFill?: boolean }) {
  return (
    <span className={cn('size-4', className)}>
      <svg
        className={cn('size-4', className)}
        style={{ fill: isFill ? 'currentColor' : '' }}
      >
        <use xlinkHref={`#icon-${name}`} />
      </svg>
    </span>
  );
}

export function FileIcon({
  name,
  className,
  type,
}: IconFontType & { type?: string }) {
  const isFolder = type === 'folder';
  return (
    <span className={cn('size-4', className)}>
      <IconFont
        name={isFolder ? 'file-sub' : FileIconMap[getExtension(name)]}
      ></IconFont>
    </span>
  );
}
