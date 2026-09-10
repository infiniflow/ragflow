import { cn } from '@/lib/utils';

/**
 * Page scroll container. Full bleed, so the themed page canvas reaches the
 * window edges; the readable column is provided by `PageContent`.
 */
export function PageContainer({
  className,
  ...props
}: React.PropsWithChildren<React.HTMLAttributes<HTMLDivElement>>) {
  return (
    <div
      className={cn('size-full overflow-auto px-6 py-8 md:px-12', className)}
      {...props}
    />
  );
}

/**
 * Centered content column: caps the reading width on wide screens instead of
 * letting the layout stretch edge to edge.
 */
export function PageContent({
  className,
  ...props
}: React.PropsWithChildren<React.HTMLAttributes<HTMLDivElement>>) {
  return (
    <div
      className={cn('mx-auto w-full max-w-[1280px]', className)}
      {...props}
    />
  );
}
