import { cn } from '@/lib/utils';

export function Container({
  children,
  className,
  ...props
}: React.PropsWithChildren<React.HTMLAttributes<HTMLDivElement>>) {
  return (
    <div
      className={cn(
        'px-2 py-1 bg-colors-background-inverse-standard  inline-flex items-center rounded-sm gap-2',
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}
