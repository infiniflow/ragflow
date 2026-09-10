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
    <div className={cn('size-full overflow-auto py-8', className)} {...props} />
  );
}

/**
 * Centered content column sharing the `page-gutter` grid with the header and the
 * list pages. `fill` turns it into the flex parent of page-owned scroll areas
 * instead of a plain block.
 */
export function PageContent({
  fill = false,
  className,
  ...props
}: React.PropsWithChildren<
  React.HTMLAttributes<HTMLDivElement> & { fill?: boolean }
>) {
  return (
    <div
      className={cn(
        'page-gutter',
        fill && 'flex min-h-0 flex-1 flex-col',
        className,
      )}
      {...props}
    />
  );
}

