import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { IArtifact } from '@/interfaces/database/dataset';
import { ITraceInfo } from '@/pages/dataset/dataset/generate-button/hook';
import { Trash2, WandSparkles } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { LeftPanelTab } from '../constants';
import { useWikiClear } from './hooks/use-wiki-clear';
import { useWikiUpdate } from './hooks/use-wiki-update';
import { WikiGraphPanel } from './wiki-graph-panel';
import { WikiNavBar } from './wiki-nav-bar';
import { WikiUpdateSheet } from './wiki-update-sheet';

type WikiLeftPanelProps = {
  tab: LeftPanelTab;
  onTabChange: (value: string) => void;
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
  onClearArtifact: () => void;
  onClearWiki?: () => void;
  updateSheetOpen: boolean;
  onUpdateSheetOpenChange: (open: boolean) => void;
  traceData?: ITraceInfo;
};

export function WikiLeftPanel({
  tab,
  onTabChange,
  selectedArtifact,
  onSelectArtifact,
  onClearArtifact,
  onClearWiki,
  updateSheetOpen,
  onUpdateSheetOpenChange,
  traceData,
}: WikiLeftPanelProps) {
  const { t } = useTranslation();

  const { open, setOpen, handleConfirm, loading } = useWikiClear({
    onClearWiki,
  });

  const {
    hasChanges,
    newlyUploaded,
    removed,
    handleUpdate,
    loading: updateLoading,
  } = useWikiUpdate();

  const handleUpdateClick = useCallback(async () => {
    onUpdateSheetOpenChange(true);
    await handleUpdate();
  }, [handleUpdate, onUpdateSheetOpenChange]);

  return (
    <aside className="size-full flex flex-col p-5">
      <div className="flex items-center justify-between pb-5">
        {hasChanges && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant={'outline'}
                  onClick={handleUpdateClick}
                  disabled={updateLoading}
                >
                  {t('knowledgeDetails.update', { defaultValue: 'Update' })}
                  {newlyUploaded > 0 && (
                    <Badge variant="success" className="ml-1">
                      {newlyUploaded}
                    </Badge>
                  )}
                  {removed > 0 && (
                    <Badge variant="destructive" className="ml-1">
                      {removed}
                    </Badge>
                  )}
                  <WandSparkles />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {t('knowledgeDetails.updateTooltip', {
                  newlyUploaded,
                  removed,
                  defaultValue:
                    '{{newlyUploaded}} new, {{removed}} removed documents found. Click to compile and merge into current Wiki.',
                })}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
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
      </div>
      <Tabs value={tab} onValueChange={onTabChange} className="pb-5">
        <TabsList className="grid grid-cols-2 w-80">
          <TabsTrigger value={LeftPanelTab.Contents}>
            {t('knowledgeDetails.contents')}
          </TabsTrigger>
          <TabsTrigger value={LeftPanelTab.Graph}>
            {t('knowledgeDetails.graph')}
          </TabsTrigger>
        </TabsList>
      </Tabs>

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

      <WikiUpdateSheet
        open={updateSheetOpen}
        onOpenChange={onUpdateSheetOpenChange}
        data={traceData}
      />
    </aside>
  );
}
