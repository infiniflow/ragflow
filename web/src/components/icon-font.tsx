import { FileIconMap } from '@/constants/file';
import { cn } from '@/lib/utils';
import { getExtension } from '@/utils/document-util';

type IconFontType = {
  name: string;
  className?: string;
};

export const IconFont = ({ name, className }: IconFontType) => (
  <svg className={cn('size-4', className)}>
    <use xlinkHref={`#icon-${name}`} />
  </svg>
);

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
