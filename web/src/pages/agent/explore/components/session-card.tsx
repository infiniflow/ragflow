import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface SessionCardProps {
  session: IAgentLogResponse & { is_new?: boolean };
  selected?: boolean;
  onClick: () => void;
  onRemove?: () => void;
}

export function SessionCard({
  session,
  selected,
  onClick,
  onRemove,
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
      <CardContent className="p-3 flex justify-between items-center gap-2">
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium truncate">{displayName}</div>
        </div>
        {onRemove && (
          <Button
            variant="ghost"
            size="icon"
            className="size-6 flex-shrink-0"
            onClick={(e) => {
              e.stopPropagation();
              onRemove();
            }}
          >
            <X className="h-4 w-4" />
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
