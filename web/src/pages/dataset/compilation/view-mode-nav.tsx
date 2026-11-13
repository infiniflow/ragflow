import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { cn } from '@/lib/utils';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { ViewMode, ViewModeLabelKeyMap, VisibleViewModes } from './constants';

type ViewModeNavProps = {
  viewMode: ViewMode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onViewModeChange: (mode: ViewMode) => void;
};

type ViewModeItemProps = {
  mode: ViewMode;
  selected: boolean;
  onSelect: (mode: ViewMode) => void;
};

function ViewModeItem({ mode, selected, onSelect }: ViewModeItemProps) {
  const { t } = useTranslation();
  const handleClick = useCallback(() => {
    onSelect(mode);
  }, [mode, onSelect]);

  return (
    <li>
      <Button
        variant="ghost"
        size="auto"
        className={cn(
          'w-full justify-start px-3 py-2 font-normal',
          selected
            ? 'bg-bg-card text-text-primary hover:bg-bg-card'
            : 'text-text-secondary hover:bg-bg-base hover:text-text-primary',
        )}
        aria-pressed={selected}
        onClick={handleClick}
      >
        <span className="truncate">{t(ViewModeLabelKeyMap[mode])}</span>
      </Button>
    </li>
  );
}

export function ViewModeNav({
  viewMode,
  open,
  onOpenChange,
  onViewModeChange,
}: ViewModeNavProps) {
  const { t } = useTranslation();

  return (
    <Collapsible
      open={open}
      onOpenChange={onOpenChange}
      className="shrink-0 p-5 pb-3"
    >
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          size="auto"
          className="w-full justify-between px-3 py-2 font-medium text-text-primary hover:bg-bg-base"
        >
          <span className="truncate">
            {t('knowledgeCompilation.artifacts')}
          </span>
          {open ? <ChevronDown /> : <ChevronRight />}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <ul className="mt-1 space-y-1">
          {VisibleViewModes.map((mode) => (
            <ViewModeItem
              key={mode}
              mode={mode}
              selected={viewMode === mode}
              onSelect={onViewModeChange}
            />
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  );
}
