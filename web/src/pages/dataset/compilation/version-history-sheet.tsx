import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { ClipboardClock } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

type HistoryVersionItem = {
  id: string;
  name: string;
  description: string;
};

type VersionHistoryItemProps = {
  version: HistoryVersionItem;
  isSelected: boolean;
  onSelect: (id: string) => void;
};

function VersionHistoryItem({
  version,
  isSelected,
  onSelect,
}: VersionHistoryItemProps) {
  const handleClick = () => {
    onSelect(version.id);
  };

  return (
    <button
      onClick={handleClick}
      className={cn(
        'w-full text-left px-6 py-4 border-b border-border-button transition-colors',
        isSelected ? 'bg-bg-card' : 'hover:bg-bg-card/50',
      )}
    >
      <div className="flex items-center gap-2 mb-1">
        <span className="font-medium text-text-primary">{version.name}</span>
        <span
          className={cn(
            'size-2 rounded-full',
            isSelected ? 'bg-accent-primary' : 'bg-transparent',
          )}
        />
      </div>
      <p className="pl-4 text-sm text-text-secondary">{version.description}</p>
    </button>
  );
}

const mockHistoryVersions: HistoryVersionItem[] = [
  {
    id: '1',
    name: 'test0415_2025_04_15_15_03_22',
    description: '本版本改了什么///类似日志',
  },
  {
    id: '2',
    name: 'test0415_2025_04_15_15_03_22',
    description: '本版本改了什么///类似日志',
  },
  {
    id: '3',
    name: 'test0415_2025_04_15_15_03',
    description: '本版本改了什么///类似日志',
  },
  {
    id: '4',
    name: 'test0415_2025_04_15_15_03_22',
    description: '本版本改了什么///类似日志',
  },
];

export function VersionHistorySheet() {
  const { t } = useTranslation();
  const [selectedVersionId, setSelectedVersionId] = useState<string>('');
  const [isVersionHistoryOpen, setIsVersionHistoryOpen] = useState(false);

  return (
    <Sheet
      open={isVersionHistoryOpen}
      onOpenChange={setIsVersionHistoryOpen}
      modal={false}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <ClipboardClock className="size-4" />
            </Button>
          </SheetTrigger>
        </TooltipTrigger>
        <TooltipContent>{t('knowledgeDetails.version')}</TooltipContent>
      </Tooltip>
      <SheetContent className="flex flex-col">
        <SheetHeader>
          <SheetTitle>{t('knowledgeDetails.versionHistory')}</SheetTitle>
        </SheetHeader>
        <div className="flex-1 -mx-6 overflow-y-auto">
          {mockHistoryVersions.map((version) => {
            const isSelected = selectedVersionId === version.id;
            return (
              <VersionHistoryItem
                key={version.id}
                version={version}
                isSelected={isSelected}
                onSelect={setSelectedVersionId}
              />
            );
          })}
        </div>
      </SheetContent>
    </Sheet>
  );
}
