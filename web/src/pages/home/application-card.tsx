import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/date';
import { t } from 'i18next';
import { ChevronRight } from 'lucide-react';

/**
 * Card shell shared by the home page application tiles (chat / search / agent /
 * memory). Sizing is fluid so the tiles can live in the responsive grid.
 */
const applicationCardClass = cn(
  'group h-full w-full rounded-xl px-4 py-3',
  'border border-cable-border bg-cable-surface shadow-cable-surface',
  // Elevation plus border highlight only: a translate would be clipped by the
  // overflow-auto grid these tiles are rendered in.
  'transition-[box-shadow,border-color] duration-200 ease-out',
  'hover:border-cable-border-hover hover:shadow-cable-surface-hover',
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cable-accent',
);

type ApplicationCardProps = {
  app: {
    avatar?: string;
    title: string;
    update_time: number;
  };
  onClick?(): void;
  moreDropdown: React.ReactNode;
};

export function ApplicationCard({
  app,
  onClick,
  moreDropdown,
}: ApplicationCardProps) {
  return (
    <Card className={applicationCardClass} onClick={onClick} as="article">
      <CardContent className="flex w-full items-center justify-between gap-3 p-0">
        <RAGFlowAvatar
          className="size-12 shrink-0 rounded-xl"
          avatar={app.avatar}
          name={app.title || 'CN'}
          aria-hidden="true"
        />

        <div className="min-w-0 flex-1">
          <h3 className="mb-1 truncate text-sm font-medium text-text-primary">
            {app.title}
          </h3>
          <p className="truncate text-xs text-cable-muted">
            {formatDate(app.update_time)}
          </p>
        </div>

        {moreDropdown}
      </CardContent>
    </Card>
  );
}

export type SeeAllAppCardProps = {
  click(): void;
};

export function SeeAllAppCard({ click }: SeeAllAppCardProps) {
  return (
    <Card
      className={cn(
        applicationCardClass,
        'flex min-h-[76px] cursor-pointer items-center justify-center',
      )}
      onClick={click}
      tabIndex={0}
    >
      <CardContent className="flex w-full items-center justify-center gap-1.5 p-0 text-cable-muted transition-colors group-hover:text-cable-brand">
        {t('common.seeAll')} <ChevronRight className="size-4" />
      </CardContent>
    </Card>
  );
}
