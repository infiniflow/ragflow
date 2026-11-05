import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { BellRing } from 'lucide-react';
import { TeamInvitationReminderDialog } from '@/ee/layouts/components/team-invitation-reminder-dialog';

export function BellButton({ className }: { className?: string }) {
  return (
    <TeamInvitationReminderDialog>
      <Button
        variant="ghost"
        size="icon"
        className={cn('group size-10 shrink-0 lg:size-8', className)}
        dot
      >
        <BellRing className="size-5 animate-bell-shake group-hover:animate-none lg:size-4" />
      </Button>
    </TeamInvitationReminderDialog>
  );
}
