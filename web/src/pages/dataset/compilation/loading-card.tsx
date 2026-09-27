import { SkeletonCard } from '@/components/skeleton-card';
import { Card } from '@/components/ui/card';

export function CompilationLoadingCard() {
  return (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col p-8">
      <SkeletonCard className="flex-1" />
    </Card>
  );
}
