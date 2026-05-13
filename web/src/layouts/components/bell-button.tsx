import { Button } from '@/components/ui/button';
import { Routes } from '@/routes';
import { BellRing } from 'lucide-react';

export function BellButton() {
  return (
    <Button
      asLink
      to={`${Routes.UserSetting}${Routes.Team}`}
      variant="ghost"
      size="icon"
      className="group"
      dot
    >
      <BellRing className="size-4 animate-bell-shake group-hover:animate-none" />
    </Button>
  );
}
