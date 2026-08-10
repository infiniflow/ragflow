import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { Spin } from '@/components/ui/spin';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useFetchWikiCommits } from '@/hooks/use-knowledge-request';
import { IArtifact, IWikiCommit } from '@/interfaces/database/dataset';
import { ClipboardClock } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { VersionHistoryItem } from './version-history-item';

type VersionHistorySheetProps = {
  selectedArtifact: IArtifact | null;
  selectedVersion: IWikiCommit | null;
  onSelectVersion: (version: IWikiCommit | null) => void;
};

export function VersionHistorySheet({
  selectedArtifact,
  selectedVersion,
  onSelectVersion,
}: VersionHistorySheetProps) {
  const { t } = useTranslation();
  const [isVersionHistoryOpen, setIsVersionHistoryOpen] = useState(false);
  const { commits, loading } = useFetchWikiCommits(
    selectedArtifact,
    isVersionHistoryOpen,
  );

  const handleSelect = (version: IWikiCommit) => {
    onSelectVersion(version);
    setIsVersionHistoryOpen(false);
  };

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
        <div className="flex-1 overflow-y-auto">
          {loading && commits.length === 0 && (
            <div className="py-8 flex justify-center">
              <Spin size="large" />
            </div>
          )}
          {commits.map((version) => {
            const isSelected = selectedVersion?.id === version.id;
            return (
              <VersionHistoryItem
                key={version.id}
                version={version}
                isSelected={isSelected}
                onSelect={handleSelect}
              />
            );
          })}
        </div>
      </SheetContent>
    </Sheet>
  );
}
