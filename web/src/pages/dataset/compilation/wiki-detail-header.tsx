import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import type { IArtifact, IWikiCommit } from '@/interfaces/database/dataset';
import { MoveLeft } from 'lucide-react';

import { Button } from '@/components/ui/button';

type WikiDetailHeaderProps = {
  title: string | undefined;
  displayedArtifact: IArtifact | null | undefined;
  commitDetail: IWikiCommit | null | undefined;
  isVersionView: boolean;
  toolbar: ReactNode;
  canGoBack: boolean;
  previousEntryTitle: string | undefined;
  linkNavLoading: boolean;
  onBack: () => void;
};

export function WikiDetailHeader({
  title,
  displayedArtifact,
  commitDetail,
  isVersionView,
  toolbar,
  canGoBack,
  previousEntryTitle,
  linkNavLoading,
  onBack,
}: WikiDetailHeaderProps) {
  const { t } = useTranslation();

  return (
    <header className="shrink-0 px-5 pt-2 pb-4">
      {canGoBack && (
        <div className="shrink-0 pt-2 pb-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={onBack}
            disabled={linkNavLoading}
            className="p-0 bg-transparent hover:bg-transparent text-text-secondary"
          >
            <MoveLeft className="size-4 mr-1" />
            {previousEntryTitle ?? t('common.back')}
          </Button>
        </div>
      )}
      <div className="flex items-start justify-between">
        <div className="flex flex-col gap-2">
          <h1 className="text-3xl font-semibold text-text-primary">
            {title ?? displayedArtifact?.title}
          </h1>
          <div className="flex items-center gap-2">
            {isVersionView && commitDetail && (
              <span className="text-sm text-accent-primary bg-accent-primary-5 px-2 py-0.5 rounded">
                {commitDetail.title}
              </span>
            )}
          </div>
        </div>

        {toolbar}
      </div>
    </header>
  );
}
