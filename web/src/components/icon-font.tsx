import { cn } from '@/lib/utils';

type IconFontType = {
  name: string;
  className?: string;
};

export const IconFont = ({ name, className }: IconFontType) => (
  <svg className={cn('fill-current size-4', className)}>
    <use xlinkHref={`#icon-${name}`} />
  </svg>
);
