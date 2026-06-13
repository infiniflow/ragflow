import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { BellRing } from 'lucide-react';

export function BellButton({ className }: { className?: string }) {
  return (
    <Button
      asLink
      to={`${Routes.UserSetting}${Routes.Team}`}
      variant="ghost"
      size="icon"
      className={cn('group size-10 shrink-0 lg:size-8', className)}
      dot
    >
      <BellRing className="size-5 animate-bell-shake group-hover:animate-none lg:size-4" />
    </Button>
  );
}
