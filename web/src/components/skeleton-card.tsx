import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

type SkeletonCardProps = {
  className?: string;
};
export function SkeletonCard(props: SkeletonCardProps) {
  const { className } = props;
  return (
    <div className={cn('space-y-4', className)}>
      <Skeleton className="h-8 w-full bg-bg-card rounded-lg" />
      <Skeleton className="h-8 w-4/5 bg-bg-card rounded-lg" />
      <Skeleton className="h-8 w-3/5 bg-bg-card rounded-lg" />
    </div>
  );
}
