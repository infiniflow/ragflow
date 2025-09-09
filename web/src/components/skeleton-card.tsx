import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

type SkeletonCardProps = {
  className?: string;
};
export function SkeletonCard(props: SkeletonCardProps) {
  const { className } = props;
  return (
    <div className={cn('space-y-2', className)}>
      <Skeleton className="h-4 w-full bg-bg-card" />
      <Skeleton className="h-4 w-full bg-bg-card" />
      <Skeleton className="h-4 w-2/3 bg-bg-card" />
    </div>
  );
}
