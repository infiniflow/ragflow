import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { GenerateStatus, GenerateType } from '@/constants/knowledge';
import { ITraceInfo, useGenerateStatus } from '@/hooks/use-dataset-generate';
import { IArtifact } from '@/interfaces/database/dataset';
import { Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { LeftPanelTab } from '../constants';
import { CompilationUpdateButton } from '../update-button';
import { UpdateLogSheet } from '../update-log-sheet';
import { useWikiClear } from './hooks/use-wiki-clear';
import { useWikiUpdate } from './hooks/use-wiki-update';
import { WikiGraphPanel } from './wiki-graph-panel';
import { WikiNavBar } from './wiki-nav-bar';

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
    changed,
    retryPageCount,
    handleUpdate,
    loading: updateLoading,
  } = useWikiUpdate();

  const { status } = useGenerateStatus(traceData);

  const handleUpdateClick = useCallback(async () => {
    onUpdateSheetOpenChange(true);
    if (status === GenerateStatus.Running) {
      return;
    }
    await handleUpdate();
  }, [status, handleUpdate, onUpdateSheetOpenChange]);

  return (
    <aside className="size-full flex flex-col p-5">
      <div className="flex items-center justify-between pb-5">
        <CompilationUpdateButton
          traceData={traceData}
          generateType={GenerateType.Artifact}
          hasChanges={hasChanges}
          newlyUploaded={newlyUploaded}
          removed={removed}
          changed={changed}
          retryPageCount={retryPageCount}
          loading={updateLoading}
          tooltip={t('knowledgeCompilation.updateTooltip', {
            newlyUploaded,
            removed,
            changed,
            defaultValue:
              '{{newlyUploaded}} new, {{removed}} removed, {{changed}} changed documents found. Click to compile and merge into current Wiki.',
          })}
          onClick={handleUpdateClick}
        />
        <ConfirmDeleteDialog
          open={open}
          onOpenChange={setOpen}
          title={t('knowledgeCompilation.clearWikiTitle')}
          content={{ title: t('knowledgeCompilation.clearWikiDescription') }}
          onOk={handleConfirm}
        >
          <Button
            variant="ghost"
            size="icon-sm"
            className="ml-auto"
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
            {t('knowledgeCompilation.contents')}
          </TabsTrigger>
          <TabsTrigger value={LeftPanelTab.Graph}>
            {t('knowledgeCompilation.graph')}
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

      <UpdateLogSheet
        open={updateSheetOpen}
        onOpenChange={onUpdateSheetOpenChange}
        data={traceData}
        title={t('knowledgeCompilation.updateSheetTitle', {
          defaultValue: 'Update Wiki',
        })}
      />
    </aside>
  );
}
