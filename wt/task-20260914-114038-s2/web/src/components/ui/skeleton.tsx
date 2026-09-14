import { cn } from '@/lib/utils';

function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn('animate-pulse rounded-md bg-bg-card', className)}
      {...props}
    />
  );
}

function ParagraphSkeleton() {
  return (
    <div className="flex items-center space-x-4">
      <Skeleton className="h-12 w-12 rounded-full" />
      <div className="space-y-2">
        <Skeleton className="h-4 w-[250px]" />
        <Skeleton className="h-4 w-[200px]" />
      </div>
    </div>
  );
}

function CardSkeleton() {
  return (
    <div className="w-64">
      <Skeleton className="mb-3 h-28 rounded-xl" />
      <Skeleton className="mb-2 h-4" />
      <Skeleton className="h-4 w-4/5" />
    </div>
  );
}

export { CardSkeleton, ParagraphSkeleton, Skeleton };
