import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import { TreeView } from '@/components/ui/tree-view';
import { GenerateStatus } from '@/constants/knowledge';
import { ITraceInfo, useGenerateStatus } from '@/hooks/use-dataset-generate';
import {
  DatasetNavList,
  DatasetNavNode,
} from '@/interfaces/database/dataset-nav';
import { IStructureGraphTemplate } from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import { useIsGoBackend } from '@/utils/backend-variant';
import { CircleX, FileText, Folder, Loader2, Trash2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { UpdateLogSheet } from './update-log-sheet';
import { buildNavTreeData, NavEntityClickHandler } from './utils/nav-tree';

// TreeView only computes expandedItemIds when initialSelectedItemId is truthy;
// combined with expandAll, any truthy id makes every branch mount open. A
// sentinel that matches no real node forces expand-all without highlighting any
// row as selected (same trick as skills-left-panel).
const NavExpandAllSentinelId = '__nav-tree-expand-all-sentinel__';

type NavNodeDeleteActionProps = {
  name: string;
  parentName: string | null;
  deleteLoading: boolean;
  onDelete: (name: string, parentName: string | null) => void;
};

function NavNodeDeleteAction({
  name,
  parentName,
  deleteLoading,
  onDelete,
}: NavNodeDeleteActionProps) {
  const { t } = useTranslation();
  const isGo = useIsGoBackend();

  const handleTriggerClick = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      // TreeView does not guard action clicks: without this the row would
      // also get selected and a branch row would toggle its accordion.
      e.stopPropagation();
    },
    [],
  );

  const handleConfirmDelete = useCallback(() => {
    onDelete(name, parentName);
  }, [name, parentName, onDelete]);

  // The Go backend does not support deleting nav nodes; don't mount the action.
  if (isGo) return null;

  return (
    <ConfirmDeleteDialog
      title={t('knowledgeCompilation.navDeleteNodeTitle')}
      content={{ title: t('knowledgeCompilation.navDeleteNodeDescription') }}
      onOk={handleConfirmDelete}
    >
      <Button
        variant="ghost"
        size="icon-sm"
        disabled={deleteLoading}
        onClick={handleTriggerClick}
        // TreeActions keeps actions always visible on the selected row;
        // hide again so the button only appears while hovering the row.
        // `hidden` (not opacity-0) so no invisible click target remains.
        className="hidden group-hover:inline-flex"
      >
        <Trash2 />
      </Button>
    </ConfirmDeleteDialog>
  );
}

type NavTreeLeftPanelProps = {
  navList: DatasetNavList | null;
  navLoading: boolean;
  navError?: boolean;
  keywords: string;
  // The debounced filter applied to the nav/children/graph requests. Used as
  // the TreeView key so a filter change remounts the tree: expansion state is
  // uncontrolled per node and onExpand only fires on opening, so without a
  // remount an already-open node whose cached children were dropped would sit
  // on the loading placeholder forever.
  activeKeywords: string;
  childrenMap: Record<string, DatasetNavNode[]>;
  childrenErrorParents?: Record<string, boolean>;
  structureMap: Record<string, IStructureGraphTemplate[]>;
  deleteNavLoading: boolean;
  deleteNodeLoading: boolean;
  traceData?: ITraceInfo;
  onKeywordsChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onNodeClick: (node: DatasetNavNode, parentName: string | null) => void;
  onNodeExpand: (node: DatasetNavNode) => void;
  onEntityClick: NavEntityClickHandler;
  onDeleteAll: () => void;
  onDeleteNode: (name: string, parentName: string | null) => void;
};

export function NavTreeLeftPanel({
  navList,
  navLoading,
  navError = false,
  keywords,
  activeKeywords,
  childrenMap,
  childrenErrorParents = {},
  structureMap,
  deleteNavLoading,
  deleteNodeLoading,
  traceData,
  onKeywordsChange,
  onNodeClick,
  onNodeExpand,
  onEntityClick,
  onDeleteAll,
  onDeleteNode,
}: NavTreeLeftPanelProps) {
  const { t } = useTranslation();
  const isGo = useIsGoBackend();

  const { status: compileStatus } = useGenerateStatus(traceData);
  const [logSheetOpen, setLogSheetOpen] = useState(false);
  // Go: an incremental compile is running while a tree is already on screen —
  // surface it as a log entry point in the header (the full-view placeholder
  // covers the first compile, when no tree exists).
  const compiling =
    isGo &&
    (compileStatus === GenerateStatus.Running ||
      compileStatus === GenerateStatus.Failed);
  const compileFailed = compiling && compileStatus === GenerateStatus.Failed;

  const handleOpenLogSheet = useCallback(() => {
    setLogSheetOpen(true);
  }, []);

  const renderNavActions = useCallback(
    (node: DatasetNavNode, parentName: string | null) => (
      <NavNodeDeleteAction
        name={node.name}
        parentName={parentName}
        deleteLoading={deleteNodeLoading}
        onDelete={onDeleteNode}
      />
    ),
    [deleteNodeLoading, onDeleteNode],
  );

  const treeData = useMemo(
    () =>
      buildNavTreeData(navList?.items, {
        childrenMap,
        childrenErrorParents,
        structureMap,
        // A search response is a pruned forest (hits + the cluster path above
        // them), so it is nested from the payload instead of being fetched
        // branch by branch.
        searchMode: !!activeKeywords,
        getActions: renderNavActions,
        onNodeClick,
        onNodeExpand,
        onEntityClick,
        loadingPlaceholder: t('knowledgeCompilation.navLoading'),
        errorPlaceholder: t('knowledgeCompilation.navChildLoadFailed'),
      }),
    [
      navList?.items,
      activeKeywords,
      childrenMap,
      childrenErrorParents,
      structureMap,
      renderNavActions,
      onNodeClick,
      onNodeExpand,
      onEntityClick,
      t,
    ],
  );

  return (
    <aside className="size-full flex flex-col">
      <section className="flex items-center justify-between px-3 pt-3">
        <span className="text-sm font-medium text-text-primary">
          {t('knowledgeCompilation.navTitle')} ({navList?.total ?? 0})
        </span>
        {!isGo && treeData.length > 0 && (
          <ConfirmDeleteDialog
            title={t('knowledgeCompilation.navDeleteAllTitle')}
            content={{
              title: t('knowledgeCompilation.navDeleteAllDescription'),
            }}
            onOk={onDeleteAll}
          >
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={deleteNavLoading}
              data-testid="nav-tree-clear-trigger"
            >
              <Trash2 />
            </Button>
          </ConfirmDeleteDialog>
        )}
        {compiling && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleOpenLogSheet}
            data-testid="nav-compile-log-trigger"
            className={cn({ 'text-state-error': compileFailed })}
          >
            {compileFailed ? <CircleX /> : <Loader2 className="animate-spin" />}
            <span
              className="max-w-56 truncate"
              title={compileFailed ? traceData?.compilationError : undefined}
            >
              {compileFailed
                ? traceData?.compilationError || t('message.operated')
                : t('knowledgeCompilation.compiling')}
            </span>
          </Button>
        )}
      </section>

      <div className="px-3 pt-2">
        <SearchInput value={keywords} onChange={onKeywordsChange} />
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto px-1 pt-2 pb-3">
        {navLoading && treeData.length === 0 ? (
          <div className="py-8 flex justify-center">
            <Spin size="small" />
          </div>
        ) : treeData.length === 0 ? (
          <div className="py-8 text-center text-sm text-text-secondary">
            {t(
              navError
                ? 'knowledgeCompilation.navLoadFailed'
                : 'knowledgeCompilation.navEmpty',
            )}
          </div>
        ) : (
          <>
            {navError ? (
              <div className="px-2 pb-2 text-center text-sm text-text-secondary">
                {t('knowledgeCompilation.navLoadFailed')}
              </div>
            ) : null}
            <TreeView
              key={activeKeywords}
              data={treeData}
              // Search: mount the matched branches open (sentinel trick above).
              expandAll={!!activeKeywords}
              initialSelectedItemId={
                activeKeywords ? NavExpandAllSentinelId : undefined
              }
              expandOnRowClick={false}
              defaultNodeIcon={Folder}
              defaultLeafIcon={FileText}
            />
          </>
        )}
      </div>

      <UpdateLogSheet
        open={logSheetOpen}
        onOpenChange={setLogSheetOpen}
        data={traceData}
        title={t('knowledgeCompilation.navLogTitle')}
      />
    </aside>
  );
}
