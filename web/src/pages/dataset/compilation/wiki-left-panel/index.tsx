import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { IArtifact } from '@/interfaces/database/dataset';
import { Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { LeftPanelTab } from '../constants';
import { useWikiClear } from './hooks/use-wiki-clear';
import { WikiGraphPanel } from './wiki-graph-panel';
import { WikiNavBar } from './wiki-nav-bar';

type WikiLeftPanelProps = {
  tab: LeftPanelTab;
  onTabChange: (value: string) => void;
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
  onClearArtifact: () => void;
  onClearWiki?: () => void;
};

export function WikiLeftPanel({
  tab,
  onTabChange,
  selectedArtifact,
  onSelectArtifact,
  onClearArtifact,
  onClearWiki,
}: WikiLeftPanelProps) {
  const { t } = useTranslation();

  const { open, setOpen, handleConfirm, loading } = useWikiClear({
    onClearWiki,
  });

  return (
    <aside className="size-full flex flex-col p-5">
      <section className="flex items-center justify-between pb-5">
        <Tabs value={tab} onValueChange={onTabChange}>
          <TabsList className="grid grid-cols-2 w-80">
            <TabsTrigger value={LeftPanelTab.Contents}>
              {t('knowledgeDetails.contents')}
            </TabsTrigger>
            <TabsTrigger value={LeftPanelTab.Graph}>
              {t('knowledgeDetails.graph')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <ConfirmDeleteDialog
          open={open}
          onOpenChange={setOpen}
          title={t('knowledgeDetails.clearWikiTitle')}
          content={{ title: t('knowledgeDetails.clearWikiDescription') }}
          onOk={handleConfirm}
        >
          <Button
            variant="ghost"
            size="icon-sm"
            disabled={loading}
            data-testid="wiki-clear-trigger"
          >
            <Trash2 className="size-[1em]" />
          </Button>
        </ConfirmDeleteDialog>
      </section>

      <div className="flex-1 min-h-0 relative">
        {tab === LeftPanelTab.Contents && (
          <WikiNavBar
            selectedArtifact={selectedArtifact}
            onSelectArtifact={onSelectArtifact}
          />
        )}
        {tab === LeftPanelTab.Graph && (
          <WikiGraphPanel
            selectedArtifact={selectedArtifact}
            onSelectArtifact={onSelectArtifact}
            onClearArtifact={onClearArtifact}
          />
        )}
      </div>
    </aside>
  );
}
