import type { IArtifact, IWikiCommit } from '@/interfaces/database/dataset';
import { Download } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';

import { VersionHistorySheet } from './version-history-sheet';

type WikiDetailToolbarProps = {
  isDirty: boolean;
  selectedArtifact: IArtifact | null;
  selectedVersion: IWikiCommit | null;
  onCancelEdit: () => void;
  onCommitClick: () => void;
  onExport: () => void;
  onSelectVersion: (version: IWikiCommit | null) => void;
};

export function WikiDetailToolbar({
  isDirty,
  selectedArtifact,
  selectedVersion,
  onCancelEdit,
  onCommitClick,
  onExport,
  onSelectVersion,
}: WikiDetailToolbarProps) {
  const { t } = useTranslation();

  if (isDirty) {
    return (
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onCancelEdit}
        >
          {t('common.cancel')}
        </Button>
        <Button type="button" size="sm" onClick={onCommitClick}>
          {t('knowledgeDetails.commit')}
        </Button>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            onClick={onExport}
          >
            <Download className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('knowledgeDetails.export')}</TooltipContent>
      </Tooltip>
      <VersionHistorySheet
        selectedArtifact={selectedArtifact}
        selectedVersion={selectedVersion}
        onSelectVersion={onSelectVersion}
      />
    </div>
  );
}
