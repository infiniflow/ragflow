import { MoreButton } from '@/components/more-button';
import { Card, CardContent } from '@/components/ui/card';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { SessionDropdown } from './session-dropdown';

interface SessionCardProps {
  session: IAgentLogResponse & { is_new?: boolean };
  selected?: boolean;
  onClick: () => void;
  removeTemporarySession?: (sessionId: string) => void;
}

export function SessionCard({
  session,
  selected,
  onClick,
  removeTemporarySession,
}: SessionCardProps) {
  const { t } = useTranslation();
  const isNewSession = session.is_new;

  const displayName = isNewSession ? t('explore.newSession') : session.name;

  return (
    <Card
      onClick={onClick}
      className={cn(
        'cursor-pointer hover:shadow-md transition-shadow',
        selected && 'bg-bg-card',
      )}
    >
      <CardContent className="p-3 flex justify-between items-center gap-2 group">
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium truncate">{displayName}</div>
        </div>
        <SessionDropdown
          session={session}
          removeTemporarySession={removeTemporarySession}
        >
          <MoreButton />
        </SessionDropdown>
      </CardContent>
    </Card>
  );
}
